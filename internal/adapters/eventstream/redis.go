package eventstream

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type RedisBus struct {
	client *redis.Client
}

func NewRedisBusFromEnv() (*RedisBus, error) {
	var opts *redis.Options
	if url := strings.TrimSpace(os.Getenv("REDIS_URL")); url != "" {
		parsed, err := redis.ParseURL(url)
		if err != nil {
			return nil, err
		}
		opts = parsed
	} else {
		opts = &redis.Options{
			Addr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
			Username: strings.TrimSpace(os.Getenv("REDIS_USERNAME")),
			Password: os.Getenv("REDIS_PASSWORD"),
		}
		if raw := strings.TrimSpace(os.Getenv("REDIS_DB")); raw != "" {
			db, err := strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid REDIS_DB %q: %w", raw, err)
			}
			opts.DB = db
		}
	}
	return &RedisBus{client: redis.NewClient(opts)}, nil
}

func (b *RedisBus) Publish(ctx context.Context, msg Message) error {
	topic := normalizeTopic(msg.Topic)
	if topic == "" || len(msg.Payload) == 0 {
		return nil
	}
	return b.client.Publish(ctx, topic, msg.Payload).Err()
}

func (b *RedisBus) Subscribe(ctx context.Context, topics []string, handler Handler) error {
	if handler == nil {
		return nil
	}
	channels := make([]string, 0, len(topics))
	for _, raw := range topics {
		if topic := normalizeTopic(raw); topic != "" {
			channels = append(channels, topic)
		}
	}
	if len(channels) == 0 {
		return nil
	}

	sub := b.client.Subscribe(ctx, channels...)
	defer func() {
		_ = sub.Close()
	}()
	if _, err := sub.Receive(ctx); err != nil {
		return err
	}
	for {
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if err := handler(ctx, Message{Topic: msg.Channel, Payload: []byte(msg.Payload)}); err != nil {
			return err
		}
	}
}

func (b *RedisBus) Close() error {
	return b.client.Close()
}

func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
