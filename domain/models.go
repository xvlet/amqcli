package domain

import "time"

// Queue represents an ActiveMQ Queue and its statistics
type Queue struct {
	Name               string
	Pending            int64 // Number Of Pending Messages
	Consumers          int64 // Number Of Consumers
	Enqueued           int64 // Messages Enqueued
	Dequeued           int64 // Messages Dequeued
	MemoryPercentUsage int   // Queue Memory Usage Percentage
	MemoryUsageBytes   int64 // Actual memory bytes used
	MemoryLimit        int64 // Queue memory limit
	StoreMessageSize   int64 // Actual disk bytes used by persistent messages
}

// Message represents an ActiveMQ Message retrieved via STOMP or Jolokia
type Message struct {
	MessageID     string
	CorrelationID string
	Persistence   string
	Priority      int
	Redelivered   bool
	ReplyTo       string
	Timestamp     time.Time
	Type          string
	Headers       map[string]string
	Properties    map[string]string
	Body          string
}

// QueueDetail represents extended statistics and consumers of a Queue
type QueueDetail struct {
	Name               string
	QueueSize          int64
	ConsumerCount      int64
	EnqueueCount       int64
	DequeueCount       int64
	InFlightCount      int64
	ExpiredCount       int64
	DispatchCount      int64
	ProducerCount      int64
	MemoryUsageBytes   int64
	MemoryPercentUsage int
	StoreMessageSize   int64
	AverageBlockedTime float64
	Consumers          []Consumer
}

// Consumer represents a client connected to a Queue
type Consumer struct {
	ConsumerID    string
	ClientID      string
	ConnectionID  string
	RemoteAddress string
	PID           string // Extracted from ClientID or ConnectionID
	Uptime        string // Calculated from timestamp in ClientID
	Enqueues      int64
	Dequeues      int64
	PrefetchSize  int
	Exclusive     bool
	Retroactive   bool
}

// Connection represents a client connection to the broker
type Connection struct {
	Name          string
	RemoteAddress string
	Active        bool
	Slow          bool
}

type BrokerStats struct {
	MemoryPercentUsage int
	StorePercentUsage  int
	TempPercentUsage   int
	StoreLimit         int64
	CPUUsage           float64
}

// QueueRepository defines operations for managing Queues and their messages
type QueueRepository interface {
	GetBrokerStats() (BrokerStats, error)
	GetBrokerInfo() (string, error)
	GetQueues() ([]Queue, error)
	GetQueueDetail(name string) (*QueueDetail, error)
	GetConnections() ([]Connection, error)
	CreateQueue(name string) error
	DeleteQueue(name string) error
	PurgeQueue(name string) error
	RemoveMessage(queueName string, messageID string) error
	MoveMessage(queueName string, messageID string, destQueue string) error
	CopyMessage(queueName string, messageID string, destQueue string) error
	RetryMessage(queueName string, messageID string) error
}

// MessageRepository defines operations for sending and manipulating Messages
type MessageRepository interface {
	BrowseQueue(queueName string) ([]Message, error)
	BrowseQueueWithPagination(queueName string, limit int, selector string) ([]Message, error)
	SendMessage(queueName string, correlationID string, ttl time.Duration, body string) error
	BrowseMessagesByCorrelationID(queueName string, correlationID string) ([]Message, error)
	DeleteMessagesByCorrelationID(queueName string, correlationID string) error
	DeleteMessagesBySelector(queueName string, selector string) error
	ConsumeMessageDestructive(queueName string) (string, error)
}
