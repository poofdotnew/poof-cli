package poll

import (
	"context"
	"fmt"
	"time"
)

// Config controls polling behavior.
type Config struct {
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffFactor     float64
	Timeout           time.Duration
	MaxConsecutiveErr int
}

// DefaultConfig returns sensible defaults for polling.
func DefaultConfig() Config {
	return Config{
		InitialDelay:      5 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffFactor:     1.5,
		Timeout:           10 * time.Minute,
		MaxConsecutiveErr: 5,
	}
}

// CheckFunc is called on each poll iteration.
// Returns (done, error). done=true means polling succeeded.
type CheckFunc func(ctx context.Context) (bool, error)

// Poll repeatedly calls check until it returns done=true or the timeout expires.
func Poll(ctx context.Context, cfg Config, check CheckFunc) error {
	delay := cfg.InitialDelay
	deadline := time.Now().Add(cfg.Timeout)
	consecutiveErrs := 0
	var lastErr error

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", cfg.Timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		done, err := check(ctx)
		if err != nil {
			consecutiveErrs++
			lastErr = err
			if cfg.MaxConsecutiveErr > 0 && consecutiveErrs >= cfg.MaxConsecutiveErr {
				return fmt.Errorf("polling failed after %d consecutive errors, last: %w", consecutiveErrs, lastErr)
			}
			delay = min(time.Duration(float64(delay)*2), cfg.MaxDelay)
			continue
		}
		consecutiveErrs = 0
		if done {
			return nil
		}

		delay = min(time.Duration(float64(delay)*cfg.BackoffFactor), cfg.MaxDelay)
	}
}
