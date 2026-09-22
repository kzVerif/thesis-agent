// Package retry provides cancellation-aware, bounded equal-jitter backoff.
package retry

import (
	"context"
	"math/rand/v2"
	"time"
)

type Backoff struct {
	base, cap, next time.Duration
	random          func() float64
}

func New() *Backoff { return NewWithRandom(time.Second, 30*time.Second, rand.Float64) }

// NewWithRandom accepts a source in [0,1); tests can supply deterministic values.
func NewWithRandom(base, cap time.Duration, random func() float64) *Backoff {
	if base <= 0 || cap < base || random == nil {
		panic("invalid backoff configuration")
	}
	return &Backoff{base: base, cap: cap, next: base, random: random}
}

func (b *Backoff) Next() time.Duration {
	window := b.next
	if b.next >= b.cap/2 {
		b.next = b.cap
	} else {
		b.next *= 2
	}
	r := b.random()
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	return window/2 + time.Duration(float64(window-window/2)*r)
}

func (b *Backoff) Reset() { b.next = b.base }

func Wait(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
