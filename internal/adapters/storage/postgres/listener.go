package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type NotificationHandler func(ctx context.Context, payload []byte) error

func Listen(ctx context.Context, conninfo string, channel string, handle NotificationHandler) error {
	if conninfo == "" {
		return fmt.Errorf("postgres listener requires connection string")
	}
	if channel == "" {
		return fmt.Errorf("postgres listener requires channel")
	}

	listener := pq.NewListener(conninfo, 10*time.Second, time.Minute, nil)
	defer func() {
		_ = listener.Close()
	}()

	if err := listener.Listen(channel); err != nil {
		return fmt.Errorf("listen %s: %w", channel, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case notification := <-listener.Notify:
			if notification == nil {
				continue
			}
			if handle != nil {
				if err := handle(ctx, []byte(notification.Extra)); err != nil {
					return err
				}
			}
		}
	}
}
