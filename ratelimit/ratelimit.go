package ratelimit

import (
	"time"

	"github.com/andrewbytecoder/gokit/clock"
)

// Note: This file is inspired by:
// https://github.com/prashantv/go-bench/blob/master/ratelimit

// Limiter is used to rate-limit some process, possibly across goroutines.
// The process is expected to call Take() before every iteration, which
// may block to throttle the goroutine.
type Limiter interface {
	// Take should block to make sure that the RPS is met
	Take() time.Time
}

type Clock interface {
	Now() time.Time
	Sleep(duration time.Duration)
}

// config configures a limiter.
type config struct {
	clock Clock
	slack int
	per   time.Duration
}

// buildConfig combines defaults with options
func buildConfig(opts []Option) config {
	c := config{
		clock: clock.New(),
		slack: 100,
		per:   time.Second,
	}

	for _, opt := range opts {
		opt.apply(&c)
	}
	return c
}

// Option configures a Limiter.
type Option interface {
	apply(*config)
}
