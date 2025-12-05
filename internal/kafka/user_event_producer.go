package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kegazani/metachat-event-sourcing/events"
	"github.com/IBM/sarama"
)

type UserEventProducer interface {
	PublishUserEvent(ctx context.Context, event *events.Event) error
	Close() error
}

type userEventProducer struct {
	producer sarama.SyncProducer
	topic    string
}

func NewUserEventProducer(bootstrapServers, topic string) (UserEventProducer, error) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Timeout = 10 * time.Second

	brokers := strings.Split(bootstrapServers, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &userEventProducer{
		producer: producer,
		topic:    topic,
	}, nil
}

func (p *userEventProducer) PublishUserEvent(ctx context.Context, event *events.Event) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	message := &sarama.ProducerMessage{
		Topic: p.topic,
		Key:   sarama.StringEncoder(event.AggregateID),
		Value: sarama.ByteEncoder(eventJSON),
		Headers: []sarama.RecordHeader{
			{Key: []byte("event_type"), Value: []byte(event.Type)},
			{Key: []byte("aggregate_id"), Value: []byte(event.AggregateID)},
			{Key: []byte("timestamp"), Value: []byte(event.Timestamp.Format(time.RFC3339))},
		},
	}

	partition, offset, err := p.producer.SendMessage(message)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	_ = partition
	_ = offset

	return nil
}

func (p *userEventProducer) Close() error {
	return p.producer.Close()
}
