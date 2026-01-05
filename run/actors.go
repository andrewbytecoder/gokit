package run

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
)

// SignalHandler returns a function that can be used to handle signals.
// that terminates with SignalError when the process receives one of the provides signals.
// or the parent context is canceled.
func SignalHandler(ctx context.Context, signals ...os.Signal) (execute func() error, interrupt func(error)) {
	ctx, cancel := context.WithCancel(ctx)
	return func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, signals...)
			select {
			case sig := <-c:
				return SignalError{
					Single: sig,
				}
			case <-ctx.Done():
				return ctx.Err()
			}

		}, func(err error) {
			cancel()
			slog.Error("interrupt", "error", err)
		}
}

// SignalError is an error that indicates that the process received a signal.
type SignalError struct {
	Single os.Signal
}

// Error implements the error interface.
func (e SignalError) Error() string {
	return fmt.Sprintf("received signal %s", e.Single)
}
