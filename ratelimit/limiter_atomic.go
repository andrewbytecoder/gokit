package ratelimit

import (
	"sync/atomic"
	"time"
	"unsafe"
)

type state struct {
	last     time.Time
	sleepFor time.Duration
}

type atomicLimiter struct {
	state   unsafe.Pointer
	padding [56]byte // cache line size - state pointer size = 64 - 8; created to avoid false sharing.

	perRequest time.Duration
	maxSlack   time.Duration
	clock      Clock
}

// newAtomicBased returns a new atomic based limiter.
func newAtomicBased(rate int, opts ...Option) *atomicLimiter {
	// independent code
	c := buildConfig(opts)
	preRequest := c.per / time.Duration(rate)
	l := &atomicLimiter{
		perRequest: preRequest,
		maxSlack:   time.Duration(c.slack) * preRequest,
		clock:      c.clock,
	}

	initialState := state{
		last:     time.Time{},
		sleepFor: 0,
	}
	atomic.StorePointer(&l.state, unsafe.Pointer(&initialState))
	return l
}

// Take blocks to ensure that the time spent between multiple
// Take calls is on average per/rate
func (t *atomicLimiter) Take() time.Time {
	var (
		newState state
		taken    bool
		interval time.Duration
	)
	for !taken {
		now := t.clock.Now()

		previousStatePointer := atomic.LoadPointer(&t.state)
		oldState := (*state)(previousStatePointer)

		newState = state{
			last:     now,
			sleepFor: oldState.sleepFor,
		}

		// If this is our first request, then we allow it.
		if oldState.last.IsZero() {
			taken = atomic.CompareAndSwapPointer(&t.state, previousStatePointer, unsafe.Pointer(&newState))
			continue
		}

	}
	return newState.last
}
