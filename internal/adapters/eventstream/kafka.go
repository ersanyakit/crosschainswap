package eventstream

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaBus struct {
	brokers []string
	groupID string
	writer  *kafka.Writer
}

func NewKafkaBusFromEnv() (*KafkaBus, error) {
	brokers := splitCSV(os.Getenv("KAFKA_BROKERS"))
	if len(brokers) == 0 {
		brokers = []string{"localhost:9092"}
	}
	groupID := strings.TrimSpace(os.Getenv("KAFKA_CONSUMER_GROUP"))
	if groupID == "" {
		groupID = defaultConsumerGroup("exchange-api")
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}
	return &KafkaBus{brokers: brokers, groupID: groupID, writer: writer}, nil
}

func (b *KafkaBus) Publish(ctx context.Context, msg Message) error {
	topic := normalizeTopic(msg.Topic)
	if topic == "" || len(msg.Payload) == 0 {
		return nil
	}
	return b.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(msg.Key),
		Value: msg.Payload,
		Time:  time.Now(),
	})
}

func (b *KafkaBus) Subscribe(ctx context.Context, topics []string, handler Handler) error {
	if handler == nil {
		return nil
	}
	topics = nonEmptyTopics(topics)
	if len(topics) == 0 {
		return nil
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(topics))
	var wg sync.WaitGroup
	for _, topic := range topics {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:  b.brokers,
			Topic:    topic,
			GroupID:  b.groupID + "-" + topic,
			MinBytes: 1,
			MaxBytes: 10e6,
		})
		wg.Add(1)
		go func(topic string, reader *kafka.Reader) {
			defer wg.Done()
			defer func() {
				_ = reader.Close()
			}()
			for {
				msg, err := reader.ReadMessage(subCtx)
				if err != nil {
					if subCtx.Err() != nil {
						return
					}
					errCh <- err
					return
				}
				if err := handler(subCtx, Message{Topic: msg.Topic, Key: string(msg.Key), Payload: msg.Value}); err != nil {
					errCh <- err
					return
				}
			}
		}(topic, reader)
	}
	go func() {
		wg.Wait()
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	case err, ok := <-errCh:
		cancel()
		if !ok {
			return nil
		}
		return err
	}
}

func (b *KafkaBus) Close() error {
	return b.writer.Close()
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func nonEmptyTopics(topics []string) []string {
	out := make([]string, 0, len(topics))
	for _, raw := range topics {
		if topic := normalizeTopic(raw); topic != "" {
			out = append(out, topic)
		}
	}
	return out
}

func defaultConsumerGroup(prefix string) string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s-%s-%d", prefix, host, os.Getpid())
}
