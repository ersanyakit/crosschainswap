package eventbus

import (
	"context"
	"sync"

	"exchange/internal/core/event"
)

type InMemory struct {
	mu       sync.RWMutex
	handlers map[event.Type][]event.Handler
}

func NewInMemory() *InMemory {
	return &InMemory{
		handlers: make(map[event.Type][]event.Handler),
	}
}

func (b *InMemory) Subscribe(eventType event.Type, handler event.Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

func (b *InMemory) Publish(ctx context.Context, evt event.Event) error {
	b.mu.RLock()
	handlers := append([]event.Handler(nil), b.handlers[evt.Type]...)
	b.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, evt); err != nil {
			return err
		}
	}

	return nil
}
