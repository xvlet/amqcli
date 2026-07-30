package activemq

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/xvlet/amqcli/config"
	"github.com/xvlet/amqcli/domain"

	"github.com/go-stomp/stomp/v3"
	"github.com/go-stomp/stomp/v3/frame"
)

var msgSeqNum int64

func generateActiveMQMessageID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}
	seq := atomic.AddInt64(&msgSeqNum, 1)
	// ActiveMQ default format: ID:<hostname>-<port>-<timestamp>-<connection_id>:<session_id>:<producer_id>:<sequence_number>
	return fmt.Sprintf("ID:%s-12345-%d-1:1:1:%d", hostname, time.Now().UnixMilli(), seq)
}

type StompClient struct {
	url      string
	username string
	password string
}

func NewStompClient(cfg config.ActiveMQConfig) *StompClient {
	return &StompClient{
		url:      cfg.StompURL,
		username: cfg.Username,
		password: cfg.Password,
	}
}

func (s *StompClient) connect() (*stomp.Conn, error) {
	opts := []func(*stomp.Conn) error{
		stomp.ConnOpt.Login(s.username, s.password),
		stomp.ConnOpt.AcceptVersion(stomp.V12),
		stomp.ConnOpt.Host(s.url),
	}
	return stomp.Dial("tcp", s.url, opts...)
}

func (s *StompClient) SendMessage(queueName string, correlationID string, ttl time.Duration, body string) error {
	conn, err := s.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Disconnect() }()

	expiresAt := time.Now().Add(ttl).UnixMilli()
	msgID := generateActiveMQMessageID()

	opts := []func(*frame.Frame) error{
		stomp.SendOpt.Header("JMSCorrelationID", correlationID),
		stomp.SendOpt.Header("correlation-id", correlationID),
		stomp.SendOpt.Header("message-id", msgID),
		stomp.SendOpt.Header("expires", strconv.FormatInt(expiresAt, 10)),
		stomp.SendOpt.Header("persistent", "true"),
	}

	return conn.Send("/queue/"+queueName, "text/plain", []byte(body), opts...)
}

func (s *StompClient) BrowseQueue(queueName string) ([]domain.Message, error) {
	return s.BrowseQueueWithPagination(queueName, 1000, "")
}

func (s *StompClient) BrowseQueueWithPagination(queueName string, limit int, selector string) ([]domain.Message, error) {
	conn, err := s.connect()
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Disconnect() }()

	// ActiveMQ extension: browser:true allows browsing without consuming
	headers := []func(*frame.Frame) error{
		stomp.SubscribeOpt.Header("browser", "true"),
		stomp.SubscribeOpt.Header("activemq.prefetchSize", "10000"),
	}
	if selector != "" {
		headers = append(headers, stomp.SubscribeOpt.Header("selector", selector))
	}

	sub, err := conn.Subscribe("/queue/"+queueName, stomp.AckClientIndividual, headers...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sub.Unsubscribe() }()

	var messages []domain.Message
	timeout := time.After(2 * time.Second)

	count := 0
Loop:
	for limit <= 0 || count < limit {
		select {
		case msg := <-sub.C:
			if msg == nil || msg.Err != nil {
				break Loop
			}

			m := domain.Message{
				MessageID:     msg.Header.Get("message-id"),
				CorrelationID: msg.Header.Get("correlation-id"),
				Persistence:   msg.Header.Get("persistent"),
				Body:          string(msg.Body),
				Properties:    make(map[string]string),
			}

			// Priority
			if pStr := msg.Header.Get("priority"); pStr != "" {
				if p, err := strconv.Atoi(pStr); err == nil {
					m.Priority = p
				}
			}

			// Timestamp (ActiveMQ stomp uses 'timestamp' header in ms)
			if tsStr := msg.Header.Get("timestamp"); tsStr != "" {
				if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
					m.Timestamp = time.UnixMilli(ts)
				}
			}

			// Map all headers to properties for metadata view
			for i := 0; i < msg.Header.Len(); i++ {
				k, v := msg.Header.GetAt(i)
				m.Properties[k] = v
			}

			if len(strings.TrimSpace(m.MessageID)) < 5 {
				continue
			}
			messages = append(messages, m)
			count++
			// Reset timeout on each message, wait up to 2s for next message
			timeout = time.After(2 * time.Second)
		case <-timeout:
			break Loop
		}
	}

	return messages, nil
}

func (s *StompClient) BrowseMessagesByCorrelationID(queueName string, correlationID string) ([]domain.Message, error) {
	selector := fmt.Sprintf("JMSCorrelationID = '%s'", correlationID)
	return s.BrowseQueueWithPagination(queueName, 1000, selector)
}

func (s *StompClient) DeleteMessagesByCorrelationID(queueName string, correlationID string) error {
	selector := fmt.Sprintf("JMSCorrelationID='%s'", correlationID)
	return s.DeleteMessagesBySelector(queueName, selector)
}

func (s *StompClient) DeleteMessagesBySelector(queueName string, selector string) error {
	conn, err := s.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Disconnect() }()

	sub, err := conn.Subscribe("/queue/"+queueName, stomp.AckClientIndividual,
		stomp.SubscribeOpt.Header("selector", selector),
	)
	if err != nil {
		return err
	}
	defer func() { _ = sub.Unsubscribe() }()

	for {
		select {
		case msg := <-sub.C:
			if msg != nil && msg.Err == nil {
				_ = conn.Ack(msg)
			} else {
				return nil
			}
		case <-time.After(1 * time.Second):
			return nil
		}
	}
}

func (s *StompClient) ConsumeMessageDestructive(queueName string) (string, error) {
	conn, err := s.connect()
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Disconnect() }()

	sub, err := conn.Subscribe("/queue/"+queueName, stomp.AckClientIndividual)
	if err != nil {
		return "", err
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Wait up to some timeout for the message
	select {
	case msg := <-sub.C:
		if msg != nil && msg.Err == nil {
			// Acknowledge to delete it
			_ = conn.Ack(msg)
			return string(msg.Body), nil
		}
		if msg != nil {
			return "", msg.Err
		}
		return "", fmt.Errorf("subscription channel closed without message")
	case <-time.After(2 * time.Second):
		return "", fmt.Errorf("timeout waiting for message to appear in %s", queueName)
	}
}
