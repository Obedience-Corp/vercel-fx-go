package fx

import (
	"context"
	"math/rand"
	"time"
)

// RetryPolicy retries retryable failures on top of the retries fx performs
// internally. It is disabled by default because fx already retries ten times.
type RetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       bool
}

// DefaultRetryPolicy is a conservative policy for callers that opt in.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{MaxAttempts: 3, InitialDelay: 2 * time.Second, MaxDelay: 30 * time.Second, Multiplier: 2}
}

func (p *RetryPolicy) attempts() int {
	if p == nil || p.MaxAttempts < 1 {
		return 1
	}
	return p.MaxAttempts
}

func (p *RetryPolicy) delayFor(attempt int, err *Error) time.Duration {
	if p == nil {
		return 0
	}
	if hinted := err.RetryDelay(); hinted > 0 {
		return p.capped(hinted)
	}
	delay := p.InitialDelay
	if delay <= 0 {
		delay = time.Second
	}
	multiplier := p.Multiplier
	if multiplier < 1 {
		multiplier = 2
	}
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * multiplier)
	}
	if p.Jitter && delay > 0 {
		delay = time.Duration(float64(delay) * (0.5 + rand.Float64()/2))
	}
	return p.capped(delay)
}

func (p *RetryPolicy) capped(d time.Duration) time.Duration {
	if p.MaxDelay > 0 && d > p.MaxDelay {
		return p.MaxDelay
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
