package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTriggerSkipsOverlappingRun(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})

	var calls atomic.Int32
	events := make(chan Event, 16)

	service, err := New(Options{
		Jobs: []Job{
			{
				ID:         "overlap",
				Interval:   time.Hour,
				MaxRetries: 0,
				Handler: func(context.Context) (Result, error) {
					if calls.Add(1) == 1 {
						close(entered)
						<-release
					}
					return Result{Processed: 1, Success: 1}, nil
				},
			},
		},
		OnEvent: func(event Event) {
			events <- event
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		service.Stop()
	})

	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if !service.Trigger("overlap") {
		t.Fatalf("Trigger(overlap) = false, want true")
	}
	waitForSignal(t, entered, time.Second)

	// 実行中にもう一度 trigger して重複スキップを確認する。
	if !service.Trigger("overlap") {
		t.Fatalf("second Trigger(overlap) = false, want true")
	}

	waitForEvent(t, events, time.Second, func(event Event) bool {
		return event.Kind == EventKindOverlapSkipped
	})

	close(release)

	waitForEvent(t, events, time.Second, func(event Event) bool {
		return event.Kind == EventKindSucceeded && event.JobID == "overlap"
	})

	if got := calls.Load(); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
}

func TestRetryUsesExponentialBackoff(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	var mu sync.Mutex
	delays := make([]time.Duration, 0, 3)
	events := make(chan Event, 32)

	service, err := New(Options{
		Jobs: []Job{
			{
				ID:           "retry",
				Interval:     time.Hour,
				MaxRetries:   3,
				RetryBackoff: 5 * time.Millisecond,
				Handler: func(context.Context) (Result, error) {
					current := attempts.Add(1)
					if current <= 3 {
						return Result{}, MarkRetryable(errors.New("temporary failure"))
					}
					return Result{Processed: 4, Success: 4}, nil
				},
			},
		},
		Sleep: func(_ context.Context, delay time.Duration) error {
			mu.Lock()
			delays = append(delays, delay)
			mu.Unlock()
			return nil
		},
		OnEvent: func(event Event) {
			events <- event
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		service.Stop()
	})

	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !service.Trigger("retry") {
		t.Fatalf("Trigger(retry) = false, want true")
	}

	waitForEvent(t, events, time.Second, func(event Event) bool {
		return event.Kind == EventKindSucceeded && event.JobID == "retry"
	})

	if got := attempts.Load(); got != 4 {
		t.Fatalf("attempts = %d, want 4", got)
	}

	mu.Lock()
	gotDelays := append([]time.Duration(nil), delays...)
	mu.Unlock()

	want := []time.Duration{
		5 * time.Millisecond,
		10 * time.Millisecond,
		20 * time.Millisecond,
	}
	if len(gotDelays) != len(want) {
		t.Fatalf("len(delays) = %d, want %d", len(gotDelays), len(want))
	}
	for i := range want {
		if gotDelays[i] != want[i] {
			t.Fatalf("delays[%d] = %s, want %s", i, gotDelays[i], want[i])
		}
	}
}

func TestRetryStopsAtMaxRetries(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	events := make(chan Event, 32)

	service, err := New(Options{
		Jobs: []Job{
			{
				ID:           "max-retry",
				Interval:     time.Hour,
				MaxRetries:   2,
				RetryBackoff: time.Millisecond,
				Handler: func(context.Context) (Result, error) {
					attempts.Add(1)
					return Result{}, MarkRetryable(errors.New("still temporary"))
				},
			},
		},
		Sleep: func(context.Context, time.Duration) error {
			return nil
		},
		OnEvent: func(event Event) {
			events <- event
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		service.Stop()
	})

	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !service.Trigger("max-retry") {
		t.Fatalf("Trigger(max-retry) = false, want true")
	}

	waitForEvent(t, events, time.Second, func(event Event) bool {
		return event.Kind == EventKindFailed && event.JobID == "max-retry"
	})

	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for signal (%s)", timeout)
	}
}

func waitForEvent(t *testing.T, events <-chan Event, timeout time.Duration, predicate func(Event) bool) Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event := <-events:
			if predicate(event) {
				return event
			}
		case <-deadline:
			t.Fatalf("timeout waiting for event (%s)", timeout)
		}
	}
}
