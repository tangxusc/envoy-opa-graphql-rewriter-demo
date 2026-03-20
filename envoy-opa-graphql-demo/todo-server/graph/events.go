package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	defaultKafkaTopic = "employee.todo.events"
)

type TodoEvent struct {
	Typename   string `json:"__typename,omitempty"`
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	OccurredAt string `json:"occurred_at"`
	ID         string `json:"id"`
	TodoID     string `json:"todo_id"`
	EmployeeID string `json:"employee_id"`
	Content    string `json:"content"`
	Deleted    bool   `json:"deleted"`
}

type EventPublisher interface {
	PublishTodoEvent(ctx context.Context, event TodoEvent) error
	Close() error
}

type NoopPublisher struct{}

func (NoopPublisher) PublishTodoEvent(context.Context, TodoEvent) error { return nil }
func (NoopPublisher) Close() error                                      { return nil }

type KafkaPublisher struct {
	writer *kafka.Writer
}

func NewKafkaPublisherFromEnv() (EventPublisher, error) {
	brokersRaw := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	if brokersRaw == "" {
		return NoopPublisher{}, nil
	}

	topic := strings.TrimSpace(os.Getenv("KAFKA_TOPIC"))
	if topic == "" {
		topic = defaultKafkaTopic
	}

	brokers := strings.Split(brokersRaw, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
	return &KafkaPublisher{writer: writer}, nil
}

func (p *KafkaPublisher) PublishTodoEvent(ctx context.Context, event TodoEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal todo event: %w", err)
	}

	message := kafka.Message{
		Key:   []byte(event.EmployeeID),
		Value: payload,
		Time:  time.Now().UTC(),
	}
	if err := p.writer.WriteMessages(ctx, message); err != nil {
		return fmt.Errorf("write kafka message: %w", err)
	}
	return nil
}

func (p *KafkaPublisher) Close() error {
	if p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
