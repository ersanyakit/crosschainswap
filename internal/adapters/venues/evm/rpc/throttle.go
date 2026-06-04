package rpc

import (
	"context"
	"strings"
	"time"
)

type Throttle struct {
	tokens chan struct{}
	delay  time.Duration
}

func NewThrottle(rps int, delay time.Duration) *Throttle {
	if rps <= 0 {
		rps = 1
	}

	t := &Throttle{
		tokens: make(chan struct{}, rps),
		delay:  delay,
	}

	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(rps))
		defer ticker.Stop()

		for range ticker.C {
			select {
			case t.tokens <- struct{}{}:
			default:
			}
		}
	}()

	return t
}

func (t *Throttle) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.tokens:
		if t.delay > 0 {
			timer := time.NewTimer(t.delay)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}

		return nil
	}
}

func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "over rate limit")
}

func Retry(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error

	for i := 0; i < attempts; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if !IsRateLimitError(err) {
			return err
		}

		wait := baseDelay * time.Duration(i+1)

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return lastErr
}
