package swap

import (
	"context"
	"fmt"
	"sync"

	coreswap "exchange/internal/core/swap"
	"exchange/internal/core/venue"
)

type Engine struct {
	mu        sync.RWMutex
	executors map[venue.VenueKind]coreswap.Executor
}

func NewEngine() *Engine {
	return &Engine{
		executors: make(map[venue.VenueKind]coreswap.Executor),
	}
}

func (e *Engine) Register(kind venue.VenueKind, executor coreswap.Executor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executors[kind] = executor
}

func (e *Engine) Quote(ctx context.Context, req coreswap.Request) (*coreswap.Quote, error) {
	executor, err := e.executor(req.VenueKind)
	if err != nil {
		return nil, err
	}
	return executor.Quote(ctx, req)
}

func (e *Engine) BuildTransaction(ctx context.Context, req coreswap.Request, quote coreswap.Quote) (*coreswap.TransactionIntent, error) {
	executor, err := e.executor(req.VenueKind)
	if err != nil {
		return nil, err
	}
	return executor.BuildTransaction(ctx, req, quote)
}

func (e *Engine) executor(kind venue.VenueKind) (coreswap.Executor, error) {
	if kind == "" {
		return nil, fmt.Errorf("venue kind is required")
	}

	e.mu.RLock()
	executor, ok := e.executors[kind]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("swap executor not registered for venue kind %s", kind)
	}
	return executor, nil
}
