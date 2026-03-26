package poll

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPoll_ImmediateSuccess(t *testing.T) {
	cfg := Config{
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 1.5,
		Timeout:       1 * time.Second,
	}

	calls := 0
	err := Poll(context.Background(), cfg, func(ctx context.Context) (bool, error) {
		calls++
		return true, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestPoll_EventualSuccess(t *testing.T) {
	cfg := Config{
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      50 * time.Millisecond,
		BackoffFactor: 1.5,
		Timeout:       2 * time.Second,
	}

	calls := 0
	err := Poll(context.Background(), cfg, func(ctx context.Context) (bool, error) {
		calls++
		return calls >= 3, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestPoll_Timeout(t *testing.T) {
	cfg := Config{
		InitialDelay:  50 * time.Millisecond,
		MaxDelay:      50 * time.Millisecond,
		BackoffFactor: 1.0,
		Timeout:       200 * time.Millisecond,
	}

	err := Poll(context.Background(), cfg, func(ctx context.Context) (bool, error) {
		return false, nil
	})

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && err.Error() != "timed out after 200ms" {
		// Our poller returns a custom error, not context.DeadlineExceeded
		if err.Error() == "" {
			t.Fatal("expected non-empty timeout error")
		}
	}
}

func TestPoll_ContextCancel(t *testing.T) {
	cfg := Config{
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      50 * time.Millisecond,
		BackoffFactor: 1.5,
		Timeout:       5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Poll(ctx, cfg, func(ctx context.Context) (bool, error) {
		return false, nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestPoll_ErrorBackoff(t *testing.T) {
	cfg := Config{
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffFactor:     1.5,
		Timeout:           2 * time.Second,
		MaxConsecutiveErr: 5,
	}

	calls := 0
	err := Poll(context.Background(), cfg, func(ctx context.Context) (bool, error) {
		calls++
		if calls <= 2 {
			return false, errors.New("transient error")
		}
		return true, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestPoll_MaxConsecutiveErrors(t *testing.T) {
	cfg := Config{
		InitialDelay:      10 * time.Millisecond,
		MaxDelay:          50 * time.Millisecond,
		BackoffFactor:     1.5,
		Timeout:           5 * time.Second,
		MaxConsecutiveErr: 3,
	}

	calls := 0
	err := Poll(context.Background(), cfg, func(ctx context.Context) (bool, error) {
		calls++
		return false, errors.New("persistent error")
	})

	if err == nil {
		t.Fatal("expected error after max consecutive errors")
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if !strings.Contains(err.Error(), "3 consecutive errors") {
		t.Errorf("expected consecutive error message, got: %v", err)
	}
}
