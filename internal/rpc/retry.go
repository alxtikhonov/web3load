// Package rpc holds transport-level concerns (retry policy today; rate
// limiting is a v0.2 item, see docs/security.md) shared by chain adapters.
// The connection pooling itself comes from ethclient's underlying
// net/http.Transport — this package only adds retry-with-backoff on top.
package rpc

import (
	"context"
	"time"
)

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond}
}

// Do runs fn until it succeeds or MaxAttempts is exhausted, waiting an
// exponentially increasing delay between attempts. It returns the last
// error on exhaustion, or ctx.Err() if the context is cancelled first.
func Do(ctx context.Context, p RetryPolicy, fn func() error) error {
	var err error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == p.MaxAttempts-1 {
			break
		}
		delay := p.BaseDelay * time.Duration(1<<uint(attempt))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}
