package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/metachat/common/event-sourcing/events"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// UserEventProducer defines the interface for publishing user events to Kafka
type UserEventProducer interface {
	// PublishUserEvent publishes a user event to Kafka
	PublishUserEvent(ctx context.Context, event *events.Event) error

	// Close closes the Kafka producer
	Close() error
}

// userEventProducer is the implementation of UserEventProducer
type userEventProducer struct {
	producer *kafka.Producer
	topic    string
}

// NewUserEventProducer creates a new user event producer
func NewUserEventProducer(bootstrapServers, topic string) (UserEventProducer, error) {
	config := &kafka.ConfigMap{
		"bootstrap.servers": bootstrapServers,
		"acks":              "all",
		"retries":           3,
		"retry.backoff.ms":  100,
		"linger.ms":         10,
		"buffer.memory":     33554432, // 32MB
		"compression.type":  "snappy",
	}

	producer, err := kafka.NewProducer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &userEventProducer{
		producer: producer,
		topic:    topic,
	}, nil
}

// PublishUserEvent publishes a user event to Kafka
func (p *userEventProducer) PublishUserEvent(ctx context.Context, event *events.Event) error {
	// Convert event to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create Kafka message
	message := &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &p.topic, Partition: kafka.PartitionAny},
		Key:            []byte(event.AggregateID),
		Value:          eventJSON,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(event.Type)},
			{Key: "aggregate_id", Value: []byte(event.AggregateID)},
			{Key: "timestamp", Value: []byte(event.Timestamp.Format(time.RFC3339))},
		},
	}

	// Produce message
	deliveryChan := make(chan kafka.Event)
	err = p.producer.Produce(message, deliveryChan)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	// Wait for delivery report
	e := <-deliveryChan
	m := e.(*kafka.Message)

	if m.TopicPartition.Error != nil {
		return fmt.Errorf("delivery failed: %w", m.TopicPartition.Error)
	}

	return nil
}

// Close closes the Kafka producer
func (p *userEventProducer) Close() error {
	p.producer.Flush(15 * 1000) // 15 seconds timeout
	p.producer.Close()
	return nil
}
