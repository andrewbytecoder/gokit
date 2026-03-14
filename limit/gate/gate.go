package gate

import "context"

// A Gate controls the maximum number of concurrent running and waiting queries.
type Gate struct {
	sem chan struct{}
}

// New returns a query gate that limits the number of queries being concurrently executed.
func New(maxConcurrentQueries int) *Gate {
	return &Gate{
		sem: make(chan struct{}, maxConcurrentQueries),
	}
}

// Start blocks until the gate has a free spot or the context is done
func (g *Gate) Start(ctx context.Context) error {
	select {
	case g.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Done releases a single spot int the gate.
func (g *Gate) Done() {
	<-g.sem
}
