package run

type actor struct {
	execute   func() error
	interrupt func(error)
}

// Group collects actors (functions) and runs them concurrently.
// When one actor returns an error, all actors are interrupted and the error is.
// The zero value of a Group is useful.
type Group struct {
	actors []actor
}

// Add adds an actor to the group. Each actor must be pre-emptable by an
// interrupt function. That is, if interrupt is invoked, execute should return.
// Also, it must be safe to call interrupt even after execute has returned.
//
// The first actor to return interrupts all running actors.
// The error os passed to the interrupt functions, and is returned by Run.
func (g *Group) Add(execute func() error, interrupt func(error)) {
	g.actors = append(g.actors, actor{execute, interrupt})
}

// Run runs all actors concurrently.
// When the first actor returns, all actors are interrupted.
// Run only returns when all actors have exited.
// Run returns the error returned by the first exiting actor.
func (g *Group) Run() error {
	if len(g.actors) == 0 {
		return nil
	}

	// Run each actor
	errors := make(chan error, len(g.actors))
	for _, a := range g.actors {
		go func(a actor) {
			errors <- a.execute()
		}(a)
	}

	// wait for the first actor to stop
	err := <-errors

	// Signal all actors to stop
	for _, a := range g.actors {
		a.interrupt(err)
	}

	// wait for all actors to stop
	// 这里使用cap, 避免在启动协程过程中出现错误导致这里len != cap
	// 从1 开始，第一个错误已经处理了
	for i := 1; i < cap(errors); i++ {
		<-errors
	}

	// Return the original error.
	return err
}
