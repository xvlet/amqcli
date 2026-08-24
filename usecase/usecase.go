package usecase

import (
	"bytes"
	"fmt"
	"github.com/xvlet/amqcli/domain"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// ActiveMQUseCase groups all use definitions
type UseCase interface {
	GetBrokerInfo() (string, error)
	GetBrokerStats() (domain.BrokerStats, error)
	GetJVMStats() (domain.JVMStats, error)
	GetQueues() ([]domain.Queue, error)
	GetTopics() ([]domain.Topic, error)
	GetConnections() ([]domain.Connection, error)
	GetAllConsumers() ([]domain.Consumer, error)
	GetQueueDetail(name string) (*domain.QueueDetail, error)
	CreateQueue(name string) error
	DeleteQueue(name string) error
	PurgeQueue(name string) error
	SendToQueue(queueName string, correlationID string, ttl time.Duration, body string) error
	BrowseOldMessages(queueName string, correlationID string) ([]domain.Message, error)
	DeleteOldMessages(queueName string, correlationID string) error
	DeleteMessagesByTime(queueName string, olderThanStr string) error
	BrowseQueue(name string) ([]domain.Message, error)
	BrowseQueueWithPagination(name string, limit int, selector string) ([]domain.Message, error)
	DeleteMessage(queueName string, messageID string) error
	MoveMessage(queueName string, messageID string, destQueue string) error
	RetryMessage(queueName string, messageID string) error
	GetFullMessageBody(queueName string, messageID string) (string, error)
}

type activeMQInteractor struct {
	queueRepo   domain.QueueRepository
	messageRepo domain.MessageRepository
	encoding    string
}

func NewActiveMQUseCase(queueRepo domain.QueueRepository, messageRepo domain.MessageRepository, encoding string) UseCase {
	return &activeMQInteractor{
		queueRepo:   queueRepo,
		messageRepo: messageRepo,
		encoding:    encoding,
	}
}

func (u *activeMQInteractor) GetBrokerInfo() (string, error) {
	return u.queueRepo.GetBrokerInfo()
}

func (u *activeMQInteractor) GetBrokerStats() (domain.BrokerStats, error) {
	return u.queueRepo.GetBrokerStats()
}

func (u *activeMQInteractor) GetJVMStats() (domain.JVMStats, error) {
	return u.queueRepo.GetJVMStats()
}

func (u *activeMQInteractor) GetQueues() ([]domain.Queue, error) {
	return u.queueRepo.GetQueues()
}

func (u *activeMQInteractor) GetTopics() ([]domain.Topic, error) {
	return u.queueRepo.GetTopics()
}

func (u *activeMQInteractor) GetConnections() ([]domain.Connection, error) {
	return u.queueRepo.GetConnections()
}

func (u *activeMQInteractor) GetAllConsumers() ([]domain.Consumer, error) {
	return u.queueRepo.GetAllConsumers()
}

func (u *activeMQInteractor) GetQueueDetail(name string) (*domain.QueueDetail, error) {
	return u.queueRepo.GetQueueDetail(name)
}

func (u *activeMQInteractor) CreateQueue(name string) error {
	return u.queueRepo.CreateQueue(name)
}

func (u *activeMQInteractor) DeleteQueue(name string) error {
	return u.queueRepo.DeleteQueue(name)
}

func (u *activeMQInteractor) PurgeQueue(name string) error {
	return u.queueRepo.PurgeQueue(name)
}

func (u *activeMQInteractor) SendToQueue(queueName string, correlationID string, ttl time.Duration, body string) error {
	return u.messageRepo.SendMessage(queueName, correlationID, ttl, body)
}

func (u *activeMQInteractor) BrowseOldMessages(queueName string, correlationID string) ([]domain.Message, error) {
	return u.messageRepo.BrowseMessagesByCorrelationID(queueName, correlationID)
}

func (u *activeMQInteractor) DeleteOldMessages(queueName string, correlationID string) error {
	return u.messageRepo.DeleteMessagesByCorrelationID(queueName, correlationID)
}

func (u *activeMQInteractor) DeleteMessagesByTime(queueName string, olderThanStr string) error {
	var duration time.Duration
	if strings.HasSuffix(olderThanStr, "d") || strings.HasSuffix(olderThanStr, "D") {
		daysStr := olderThanStr[:len(olderThanStr)-1]
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return fmt.Errorf("invalid days format: %s", olderThanStr)
		}
		duration = time.Duration(days) * 24 * time.Hour
	} else if strings.HasSuffix(olderThanStr, "m") || strings.HasSuffix(olderThanStr, "h") {
		parsed, err := time.ParseDuration(olderThanStr)
		if err != nil {
			return fmt.Errorf("invalid time format: %s", olderThanStr)
		}
		duration = parsed
	} else {
		return fmt.Errorf("invalid format. Use '3d', '12h', '30m'")
	}

	cutoff := time.Now().Add(-duration).UnixMilli()
	selector := fmt.Sprintf("JMSTimestamp < %d", cutoff)
	return u.messageRepo.DeleteMessagesBySelector(queueName, selector)
}

func (u *activeMQInteractor) BrowseQueue(name string) ([]domain.Message, error) {
	return u.messageRepo.BrowseQueue(name)
}

func (u *activeMQInteractor) BrowseQueueWithPagination(name string, limit int, selector string) ([]domain.Message, error) {
	return u.messageRepo.BrowseQueueWithPagination(name, limit, selector)
}

func (u *activeMQInteractor) DeleteMessage(queueName string, messageID string) error {
	return u.queueRepo.RemoveMessage(queueName, messageID)
}

func (u *activeMQInteractor) MoveMessage(queueName string, messageID string, destQueue string) error {
	return u.queueRepo.MoveMessage(queueName, messageID, destQueue)
}

func (u *activeMQInteractor) RetryMessage(queueName string, messageID string) error {
	return u.queueRepo.RetryMessage(queueName, messageID)
}

func (u *activeMQInteractor) GetFullMessageBody(queueName string, messageID string) (body string, retErr error) {
	// Use a unique timestamp-based name to avoid conflicts
	tempQueue := fmt.Sprintf("CLI.TEMP.%d", time.Now().UnixNano())

	// 1. Copy the message to the temp queue via JMX (leaves original message completely untouched)
	err := u.queueRepo.CopyMessage(queueName, messageID, tempQueue)
	if err != nil {
		return "", fmt.Errorf("failed to copy message to temp queue: %v", err)
	}

	// Make sure we delete the temp queue afterward to avoid leaks
	defer func() {
		if delErr := u.queueRepo.DeleteQueue(tempQueue); delErr != nil {
			if retErr == nil {
				retErr = fmt.Errorf("message read succeeded, but failed to delete temp queue %s: %v", tempQueue, delErr)
			}
		}
	}()

	// 2. Consume the message via STOMP (fetches full payload regardless of size)
	body, err = u.messageRepo.ConsumeMessageDestructive(tempQueue)
	if err != nil {
		return "", fmt.Errorf("failed to read full message via STOMP: %v", err)
	}

	// Decode based on configured encoding
	if strings.ToLower(u.encoding) == "euc-kr" || strings.ToLower(u.encoding) == "cp949" {
		reader := transform.NewReader(bytes.NewReader([]byte(body)), korean.EUCKR.NewDecoder())
		if decodedBody, err := io.ReadAll(reader); err == nil {
			return string(decodedBody), nil
		}
	} else {
		// utf-8 or unsupported encoding passes through directly
		return body, nil
	}

	return body, nil
}
