package channel

import (
	"testing"
	"time"
)

//go:generate go test

func TestChannel(t *testing.T) {
	sig := func(after time.Duration) <-chan interface{} {
		ch := make(chan interface{})
		go func() {
			defer close(ch)
			time.Sleep(after)
		}()
		return ch
	}
	start := time.Now()
	<-Or(
		sig(2*time.Hour),
		sig(5*time.Minute),
		sig(1*time.Second),
		sig(1*time.Hour),
		sig(1*time.Minute),
	)
	duration := time.Since(start)
	if duration < 1*time.Second {
		t.Fatalf("should take about 1s, but actually %s", duration)
	}
}

func TestPipeline(t *testing.T) {
	done := make(chan interface{})
	intStream := Generator(done, 1, 2, 3, 4, 5)
	defer close(done)
	pipeline := Multiply(done, Add(done, Multiply(done, intStream, 2), 1), 2)
	for v := range pipeline {
		t.Log(v)
	}
}
