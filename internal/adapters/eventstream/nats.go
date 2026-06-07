package eventstream

import (
	"context"
	"os"
	"strings"

	"github.com/nats-io/nats.go"
)

type NATSBus struct {
	conn *nats.Conn
}

func NewNATSBusFromEnv() (*NATSBus, error) {
	url := strings.TrimSpace(os.Getenv("NATS_URL"))
	if url == "" {
		url = nats.DefaultURL
	}
	name := strings.TrimSpace(os.Getenv("NATS_CLIENT_NAME"))
	if name == "" {
		name = "exchange-eventstream"
	}
	conn, err := nats.Connect(url, nats.Name(name))
	if err != nil {
		return nil, err
	}
	return &NATSBus{conn: conn}, nil
}

func (b *NATSBus) Publish(ctx context.Context, msg Message) error {
	topic := normalizeTopic(msg.Topic)
	if topic == "" || len(msg.Payload) == 0 {
		return nil
	}
	if err := b.conn.Publish(topic, msg.Payload); err != nil {
		return err
	}
	return b.conn.FlushWithContext(ctx)
}

func (b *NATSBus) Subscribe(ctx context.Context, topics []string, handler Handler) error {
	if handler == nil {
		return nil
	}
	subs := make([]*nats.Subscription, 0, len(topics))
	for _, raw := range topics {
		topic := normalizeTopic(raw)
		if topic == "" {
			continue
		}
		sub, err := b.conn.Subscribe(topic, func(msg *nats.Msg) {
			_ = handler(ctx, Message{Topic: msg.Subject, Payload: msg.Data})
		})
		if err != nil {
			return err
		}
		subs = append(subs, sub)
	}
	if err := b.conn.FlushWithContext(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	return ctx.Err()
}

func (b *NATSBus) Close() error {
	b.conn.Close()
	return nil
}
