package usecase

import (
	"testing"
	"time"

	"github.com/xvlet/amqcli/domain"
)

type mockQueueRepo struct {
	queues []domain.Queue
	detail *domain.QueueDetail
}

func (m *mockQueueRepo) GetBrokerStats() (domain.BrokerStats, error) {
	return domain.BrokerStats{TotalEnqueueCount: 100}, nil
}
func (m *mockQueueRepo) GetBrokerInfo() (string, error) {
	return "Apache ActiveMQ Artemis 2.33.0", nil
}
func (m *mockQueueRepo) GetJVMStats() (domain.JVMStats, error) {
	return domain.JVMStats{ThreadCount: 10}, nil
}
func (m *mockQueueRepo) GetQueues() ([]domain.Queue, error) {
	return m.queues, nil
}
func (m *mockQueueRepo) GetTopics() ([]domain.Topic, error) {
	return []domain.Topic{{Name: "TOPIC.1"}}, nil
}
func (m *mockQueueRepo) GetQueueDetail(name string) (*domain.QueueDetail, error) {
	return m.detail, nil
}
func (m *mockQueueRepo) GetConnections() ([]domain.Connection, error) {
	return []domain.Connection{{Name: "conn-1"}}, nil
}
func (m *mockQueueRepo) GetAllConsumers() ([]domain.Consumer, error) {
	return []domain.Consumer{{ConsumerID: "c1"}}, nil
}
func (m *mockQueueRepo) CreateQueue(name string) error { return nil }
func (m *mockQueueRepo) DeleteQueue(name string) error { return nil }
func (m *mockQueueRepo) PurgeQueue(name string) error  { return nil }
func (m *mockQueueRepo) RemoveMessage(queueName string, messageID string) error {
	return nil
}
func (m *mockQueueRepo) MoveMessage(queueName string, messageID string, destQueue string) error {
	return nil
}
func (m *mockQueueRepo) CopyMessage(queueName string, messageID string, destQueue string) error {
	return nil
}
func (m *mockQueueRepo) RetryMessage(queueName string, messageID string) error {
	return nil
}

type mockMessageRepo struct{}

func (m *mockMessageRepo) BrowseQueue(queueName string) ([]domain.Message, error) {
	return []domain.Message{{MessageID: "1", Body: "hello"}}, nil
}
func (m *mockMessageRepo) BrowseQueueWithPagination(queueName string, limit int, selector string) ([]domain.Message, error) {
	return []domain.Message{{MessageID: "1", Body: "hello"}}, nil
}
func (m *mockMessageRepo) SendMessage(queueName string, correlationID string, ttl time.Duration, body string) error {
	return nil
}
func (m *mockMessageRepo) BrowseMessagesByCorrelationID(queueName string, correlationID string) ([]domain.Message, error) {
	return []domain.Message{{MessageID: "1", CorrelationID: correlationID}}, nil
}
func (m *mockMessageRepo) DeleteMessagesByCorrelationID(queueName string, correlationID string) error {
	return nil
}
func (m *mockMessageRepo) DeleteMessagesBySelector(queueName string, selector string) error {
	return nil
}
func (m *mockMessageRepo) ConsumeMessageDestructive(queueName string) (string, error) {
	return "test message content", nil
}

func TestActiveMQUseCase_ArtemisAndClassic(t *testing.T) {
	qRepo := &mockQueueRepo{
		queues: []domain.Queue{{Name: "QUEUE.1", Pending: 10}},
		detail: &domain.QueueDetail{Name: "QUEUE.1", QueueSize: 10},
	}
	mRepo := &mockMessageRepo{}

	uc := NewActiveMQUseCase(qRepo, mRepo, "utf-8")

	info, err := uc.GetBrokerInfo()
	if err != nil || info != "Apache ActiveMQ Artemis 2.33.0" {
		t.Fatalf("GetBrokerInfo failed: %v, got %s", err, info)
	}

	queues, err := uc.GetQueues()
	if err != nil || len(queues) != 1 {
		t.Fatalf("GetQueues failed: %v", err)
	}

	fullBody, err := uc.GetFullMessageBody("QUEUE.1", "1")
	if err != nil || fullBody != "test message content" {
		t.Fatalf("GetFullMessageBody failed: %v, got %s", err, fullBody)
	}
}
