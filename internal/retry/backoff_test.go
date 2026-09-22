package retry

import (
	"context"
	"testing"
	"time"
)

func TestGrowthCapJitterReset(t *testing.T) {
	for _, random := range []float64{0, 0.25, 0.999999, 1} {
		b := NewWithRandom(time.Second, 30*time.Second, func() float64 { return random })
		for _, ceiling := range []time.Duration{1, 2, 4, 8, 16, 30, 30, 30} {
			ceiling *= time.Second
			got := b.Next()
			if got < ceiling/2 || got > ceiling {
				t.Fatalf("jitter=%v delay=%v window=%v", random, got, ceiling)
			}
		}
		b.Reset()
		if got := b.Next(); got < 500*time.Millisecond || got > time.Second {
			t.Fatalf("reset: %v", got)
		}
	}
}

func TestWaitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Wait(ctx, 30*time.Second) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("backoff ignored cancellation")
	}
}
