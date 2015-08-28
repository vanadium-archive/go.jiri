// Package retry provides a facility for retrying function
// invocations.
package retry

import (
	"fmt"
	"time"

	"v.io/jiri/internal/tool"
)

type RetryOpt interface {
	retryOpt()
}

type AttemptsOpt int

func (a AttemptsOpt) retryOpt() {}

type IntervalOpt time.Duration

func (i IntervalOpt) retryOpt() {}

const (
	defaultAttempts = 3
	defaultInterval = 10 * time.Second
)

// Function retries the given function for the given number of
// attempts at the given interval.
func Function(ctx *tool.Context, fn func() error, opts ...RetryOpt) error {
	attempts, interval := defaultAttempts, defaultInterval
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case AttemptsOpt:
			attempts = int(typedOpt)
		case IntervalOpt:
			interval = time.Duration(typedOpt)
		}
	}

	var err error
	for i := 1; i <= attempts; i++ {
		if i > 1 {
			fmt.Fprintf(ctx.Stdout(), "Attempt %d/%d:\n", i, attempts)
		}
		if err = fn(); err == nil {
			return nil
		}
		fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		if i < attempts {
			fmt.Fprintf(ctx.Stdout(), "Wait for %v before next attempt...\n", interval)
			time.Sleep(interval)
		}
	}
	return fmt.Errorf("Failed %d times in a row. Last error:\n%v", attempts, err)
}
