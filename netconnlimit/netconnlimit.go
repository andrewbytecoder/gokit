// Package netconnlimit provides network utility functions for limiting
// simultaneous connections across multiple listeners.
package netconnlimit

import (
	"net"
	"sync"
)

// NewSharedSemaphore creates and returns a new semaphore channel that can be used
// to limit the number of simultaneous connections across multiple listeners.
// 使用空结构体作为信号，避免资源浪费
func NewSharedSemaphore(n int) chan struct{} {
	return make(chan struct{}, n)
}

// SharedLimitListener returns a listener that accepts at most n simultaneous
// connections across multiple listeners using the provided shared semaphore.
func SharedLimitListener(l net.Listener, sem chan struct{}) net.Listener {
	return &sharedLimitListener{
		Listener: l,
		sem:      sem,
		done:     make(chan struct{}),
	}
}

type sharedLimitListener struct {
	net.Listener
	sem       chan struct{}
	closeOnce sync.Once     // Ensures the done chan is only closed once.
	done      chan struct{} // No values sent; closed when Close is called.
}

// Acquire acquires the shared semaphore. Returns true if successfully
// acquired, false if the listener is closed and the semaphore is not
// acquired.
func (l *sharedLimitListener) acquire() bool {
	select {
	case <-l.done:
		return false
	case l.sem <- struct{}{}:
		return true
	}
}

func (l *sharedLimitListener) release() { <-l.sem }

func (l *sharedLimitListener) Accept() (net.Conn, error) {
	if !l.acquire() {
		for {
			c, err := l.Listener.Accept()
			if err != nil {
				return nil, err
			}
			c.Close()
		}
	}

	c, err := l.Listener.Accept()
	if err != nil {
		l.release()
		return nil, err
	}
	return &sharedLimitListenerConn{Conn: c, release: l.release}, nil
}

func (l *sharedLimitListener) Close() error {
	err := l.Listener.Close()
	l.closeOnce.Do(func() { close(l.done) })
	return err
}

type sharedLimitListenerConn struct {
	net.Conn
	releaseOnce sync.Once
	release     func()
}

func (l *sharedLimitListenerConn) Close() error {
	err := l.Conn.Close()
	l.releaseOnce.Do(l.release)
	return err
}
