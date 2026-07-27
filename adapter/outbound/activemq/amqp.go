package activemq

import (
	"context"
	"fmt"
	"time"

	"amqcli/config"
	"amqcli/domain"

	"github.com/Azure/go-amqp"
)

type AmqpClient struct {
	url      string
	username string
	password string
}

func NewAmqpClient(cfg config.ActiveMQConfig) *AmqpClient {
	return &AmqpClient{
		url:      cfg.AmqpURL,
		username: cfg.Username,
		password: cfg.Password,
	}
}

func (a *AmqpClient) connect(ctx context.Context) (*amqp.Conn, error) {
	opts := &amqp.ConnOptions{
		SASLType: amqp.SASLTypePlain(a.username, a.password),
	}
	return amqp.Dial(ctx, a.url, opts)
}

func (a *AmqpClient) SendMessage(queueName string, correlationID string, ttl time.Duration, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := a.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = session.Close(ctx) }()

	sender, err := session.NewSender(ctx, queueName, nil)
	if err != nil {
		return err
	}
	defer func() { _ = sender.Close(ctx) }()

	msg := amqp.NewMessage([]byte(body))
	msg.Properties = &amqp.MessageProperties{
		CorrelationID: correlationID,
	}
	if ttl > 0 {
		msg.Header = &amqp.MessageHeader{
			TTL: time.Duration(ttl),
		}
	}

	return sender.Send(ctx, msg, nil)
}

func (a *AmqpClient) BrowseQueue(queueName string) ([]domain.Message, error) {
	return a.BrowseQueueWithPagination(queueName, 1000, "")
}

func (a *AmqpClient) BrowseQueueWithPagination(queueName string, limit int, selector string) ([]domain.Message, error) {
	// Note: AMQP 1.0 browsing without consumption requires specific node properties
	// (e.g. distribution-mode = copy). For basic ActiveMQ, this might consume or fail
	// if not properly configured. This is a basic consumer implementation.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := a.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = session.Close(ctx) }()

	// In AMQP 1.0, filtering by selector is complex (requires source filter).
	// This is a simplified receiver.
	receiver, err := session.NewReceiver(ctx, queueName, &amqp.ReceiverOptions{
		// SettleMode: amqp.ReceiverSettleModeFirst,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = receiver.Close(ctx) }()

	var messages []domain.Message
	count := 0

	for limit <= 0 || count < limit {
		msgCtx, msgCancel := context.WithTimeout(ctx, 1*time.Second)
		msg, err := receiver.Receive(msgCtx, nil)
		msgCancel()

		if err != nil {
			break // Timeout or error
		}

		// Don't accept() if we are just browsing (ActiveMQ might still lock it though)
		// To properly browse in AMQP 1.0, Source properties need careful configuration.

		m := domain.Message{
			Body:       string(msg.GetData()),
			Properties: make(map[string]string),
		}
		if msg.Properties != nil {
			if id, ok := msg.Properties.MessageID.(string); ok {
				m.MessageID = id
			}
			if cid, ok := msg.Properties.CorrelationID.(string); ok {
				m.CorrelationID = cid
			}
		}

		messages = append(messages, m)
		count++
	}

	return messages, nil
}

func (a *AmqpClient) BrowseMessagesByCorrelationID(queueName string, correlationID string) ([]domain.Message, error) {
	return a.BrowseQueueWithPagination(queueName, 1000, fmt.Sprintf("JMSCorrelationID='%s'", correlationID))
}

func (a *AmqpClient) DeleteMessagesByCorrelationID(queueName string, correlationID string) error {
	return a.DeleteMessagesBySelector(queueName, fmt.Sprintf("JMSCorrelationID='%s'", correlationID))
}

func (a *AmqpClient) DeleteMessagesBySelector(queueName string, selector string) error {
	// This requires consuming and acknowledging messages matching the selector.
	return fmt.Errorf("AMQP DeleteMessagesBySelector not fully implemented")
}

func (a *AmqpClient) ConsumeMessageDestructive(queueName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := a.connect(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	session, err := conn.NewSession(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close(ctx) }()

	receiver, err := session.NewReceiver(ctx, queueName, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = receiver.Close(ctx) }()

	msgCtx, msgCancel := context.WithTimeout(ctx, 2*time.Second)
	defer msgCancel()

	msg, err := receiver.Receive(msgCtx, nil)
	if err != nil {
		return "", err
	}

	_ = receiver.AcceptMessage(context.Background(), msg)
	return string(msg.GetData()), nil
}
