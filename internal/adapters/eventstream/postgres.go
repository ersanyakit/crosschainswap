package eventstream

import (
	"context"
	"fmt"
	"os"
	"sync"

	"exchange/internal/adapters/storage/postgres"

	"gorm.io/gorm"
)

type PostgresBus struct {
	db *gorm.DB
}

func NewPostgresBus(db *gorm.DB) (*PostgresBus, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres event backend requires db")
	}
	return &PostgresBus{db: db}, nil
}

func (b *PostgresBus) Publish(ctx context.Context, msg Message) error {
	topic := normalizeTopic(msg.Topic)
	if topic == "" || len(msg.Payload) == 0 {
		return nil
	}
	return b.db.WithContext(ctx).Exec("SELECT pg_notify(?, ?)", topic, string(msg.Payload)).Error
}

func (b *PostgresBus) Subscribe(ctx context.Context, topics []string, handler Handler) error {
	if handler == nil {
		return nil
	}
	conninfo := os.Getenv("DATABASE_URL")
	if conninfo == "" {
		return fmt.Errorf("DATABASE_URL is required for postgres event subscriptions")
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(topics))
	var wg sync.WaitGroup
	for _, raw := range topics {
		topic := normalizeTopic(raw)
		if topic == "" {
			continue
		}
		wg.Add(1)
		go func(topic string) {
			defer wg.Done()
			err := postgres.Listen(subCtx, conninfo, topic, func(ctx context.Context, payload []byte) error {
				return handler(ctx, Message{Topic: topic, Payload: payload})
			})
			if err != nil && subCtx.Err() == nil {
				errCh <- err
			}
		}(topic)
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

func (b *PostgresBus) Close() error {
	return nil
}
