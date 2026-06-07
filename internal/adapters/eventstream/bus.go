package eventstream

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
)

type Message struct {
	Topic   string
	Key     string
	Payload []byte
}

type Handler func(context.Context, Message) error

type Bus interface {
	Publish(context.Context, Message) error
	Subscribe(context.Context, []string, Handler) error
	Close() error
}

func NewFromEnv(db *gorm.DB) (Bus, error) {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("EVENT_BACKEND")))
	if backend == "" {
		backend = strings.ToLower(strings.TrimSpace(os.Getenv("OUTBOX_DELIVERY_BACKEND")))
	}
	if backend == "" {
		backend = "postgres"
	}

	switch backend {
	case "postgres", "pg", "notify", "postgres_notify":
		return NewPostgresBus(db)
	case "redis":
		return NewRedisBusFromEnv()
	case "nats":
		return NewNATSBusFromEnv()
	case "kafka":
		return NewKafkaBusFromEnv()
	default:
		return nil, fmt.Errorf("unsupported EVENT_BACKEND %q", backend)
	}
}

func normalizeTopic(topic string) string {
	return strings.TrimSpace(topic)
}
