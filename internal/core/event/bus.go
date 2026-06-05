package event

import (
	"context"

	"exchange/internal/core/venue"
)

type Type string

const PoolBatchScanned Type = "venue.pool_batch_scanned"

type Event struct {
	Type    Type
	Payload any
}

type Handler func(ctx context.Context, event Event) error

type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventType Type, handler Handler)
}

type PoolBatchScannedPayload struct {
	VenueKey venue.VenueKey
	Pools    []venue.Pool
}
