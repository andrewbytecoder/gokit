package clock

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// counter is an atomic uint32 that can be incremented easily.  It's
// useful for asserting things have happened in tests.
type counter struct {
	count uint32
}

func (c *counter) incr() {
	atomic.AddUint32(&c.count, 1)
}

func (c *counter) get() uint32 {
	return atomic.LoadUint32(&c.count)
}

// Ensure that the clock's After channel sends at the correct time.
func TestClock_After(t *testing.T) {
	start := time.Now()
	<-New().After(20 * time.Millisecond)
	dur := time.Since(start)

	if dur < 20*time.Millisecond || dur > 40*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}
}

// Ensure that the clock's AfterFunc executes at the correct time.
func TestClock_AfterFunc(t *testing.T) {
	var ok bool
	var wg sync.WaitGroup

	wg.Add(1)
	start := time.Now()
	New().AfterFunc(20*time.Millisecond, func() {
		ok = true
		wg.Done()
	})
	wg.Wait()
	dur := time.Since(start)

	if dur < 20*time.Millisecond || dur > 40*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}
	if !ok {
		t.Fatal("Function did not run")
	}
}

// Ensure that the clock's time matches the standary library.
func TestClock_Now(t *testing.T) {
	a := time.Now().Round(time.Second)
	b := New().Now().Round(time.Second)
	if !a.Equal(b) {
		t.Errorf("not equal: %s != %s", a, b)
	}
}

// Ensure that the clock sleeps for the appropriate amount of time.
func TestClock_Sleep(t *testing.T) {
	start := time.Now()
	New().Sleep(20 * time.Millisecond)
	dur := time.Since(start)

	if dur < 20*time.Millisecond || dur > 40*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}
}

// Ensure that the clock ticks correctly.
func TestClock_Tick(t *testing.T) {
	start := time.Now()
	c := New().Tick(20 * time.Millisecond)
	<-c
	<-c
	dur := time.Since(start)

	if dur < 20*time.Millisecond || dur > 50*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}
}

// Ensure that the clock's ticker ticks correctly.
func TestClock_Ticker(t *testing.T) {
	start := time.Now()
	ticker := New().Ticker(50 * time.Millisecond)
	<-ticker.C
	<-ticker.C
	dur := time.Since(start)

	if dur < 100*time.Millisecond || dur > 200*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}
}

// Ensure that the clock's ticker can stop correctly.
func TestClock_Ticker_Stp(t *testing.T) {
	ticker := New().Ticker(20 * time.Millisecond)
	<-ticker.C
	ticker.Stop()
	select {
	case <-ticker.C:
		t.Fatal("unexpected send")
	case <-time.After(30 * time.Millisecond):
	}
}

// Ensure that the clock's ticker can reset correctly.
func TestClock_Ticker_Rst(t *testing.T) {
	start := time.Now()
	ticker := New().Ticker(20 * time.Millisecond)
	<-ticker.C
	ticker.Reset(5 * time.Millisecond)
	<-ticker.C
	dur := time.Since(start)
	if dur >= 30*time.Millisecond {
		t.Fatal("took more than 30ms")
	}
	ticker.Stop()
}

// Ensure that the clock's timer waits correctly.
func TestClock_Timer(t *testing.T) {
	start := time.Now()
	timer := New().Timer(20 * time.Millisecond)
	<-timer.C
	dur := time.Since(start)

	if dur < 20*time.Millisecond || dur > 40*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}

	if timer.Stop() {
		t.Fatal("timer still running")
	}
}

// Ensure that the clock's timer can be stopped.
func TestClock_Timer_Stop(t *testing.T) {
	timer := New().Timer(20 * time.Millisecond)
	if !timer.Stop() {
		t.Fatal("timer not running")
	}
	if timer.Stop() {
		t.Fatal("timer wasn't cancelled")
	}
	select {
	case <-timer.C:
		t.Fatal("unexpected send")
	case <-time.After(30 * time.Millisecond):
	}
}

// Ensure that the clock's timer can be reset.
func TestClock_Timer_Reset(t *testing.T) {
	start := time.Now()
	timer := New().Timer(10 * time.Millisecond)
	if !timer.Reset(20 * time.Millisecond) {
		t.Fatal("timer not running")
	}
	<-timer.C
	dur := time.Since(start)

	if dur < 20*time.Millisecond || dur > 40*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}
}

// Ensure reset can be called immediately after reading channel
func TestClock_Timer_Reset_Unlock(t *testing.T) {
	clock := NewMock()
	timer := clock.Timer(1 * time.Second)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		select {
		case <-timer.C:
			timer.Reset(1 * time.Second)
		}

		select {
		case <-timer.C:
		}
	}()

	clock.Add(2 * time.Second)
	wg.Wait()
}

// Ensure that the mock's After channel sends at the correct time.
func TestMock_After(t *testing.T) {
	var ok int32
	clock := NewMock()

	// Create a channel to execute after 10 mock seconds.
	ch := clock.After(10 * time.Second)
	go func(ch <-chan time.Time) {
		<-ch
		atomic.StoreInt32(&ok, 1)
	}(ch)

	// Move clock forward to just before the time.
	clock.Add(9 * time.Second)
	if atomic.LoadInt32(&ok) == 1 {
		t.Fatal("too early")
	}

	// Move clock forward to the after channel's time.
	clock.Add(1 * time.Second)
	if atomic.LoadInt32(&ok) == 0 {
		t.Fatal("too late")
	}
}

// Ensure that the mock's After channel doesn't block on write.
func TestMock_UnusedAfter(t *testing.T) {
	mock := NewMock()
	mock.After(1 * time.Millisecond)

	done := make(chan bool, 1)
	go func() {
		mock.Add(1 * time.Second)
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("mock.Add hung")
	}
}

// Ensure that the mock's AfterFunc executes at the correct time.
func TestMock_AfterFunc(t *testing.T) {
	var ok int32
	clock := NewMock()

	// Execute function after duration.
	clock.AfterFunc(10*time.Second, func() {
		atomic.StoreInt32(&ok, 1)
	})

	// Move clock forward to just before the time.
	clock.Add(9 * time.Second)
	if atomic.LoadInt32(&ok) == 1 {
		t.Fatal("too early")
	}

	// Move clock forward to the after channel's time.
	clock.Add(1 * time.Second)
	if atomic.LoadInt32(&ok) == 0 {
		t.Fatal("too late")
	}
}

// Ensure that the mock's AfterFunc doesn't execute if stopped.
func TestMock_AfterFunc_Stop(t *testing.T) {
	// Execute function after duration.
	clock := NewMock()
	timer := clock.AfterFunc(10*time.Second, func() {
		t.Fatal("unexpected function execution")
	})
	gosched()

	// Stop timer & move clock forward.
	timer.Stop()
	clock.Add(10 * time.Second)
	gosched()
}

// Ensure that the mock's current time can be changed.
func TestMock_Now(t *testing.T) {
	clock := NewMock()
	if now := clock.Now(); !now.Equal(time.Unix(0, 0)) {
		t.Fatalf("expected epoch, got: %v", now)
	}

	// Add 10 seconds and check the time.
	clock.Add(10 * time.Second)
	if now := clock.Now(); !now.Equal(time.Unix(10, 0)) {
		t.Fatalf("expected epoch, got: %v", now)
	}
}

func TestMock_Since(t *testing.T) {
	clock := NewMock()

	beginning := clock.Now()
	clock.Add(500 * time.Second)
	if since := clock.Since(beginning); since.Seconds() != 500 {
		t.Fatalf("expected 500 since beginning, actually: %v", since.Seconds())
	}
}

func TestMock_Until(t *testing.T) {
	clock := NewMock()

	end := clock.Now().Add(500 * time.Second)
	if dur := clock.Until(end); dur.Seconds() != 500 {
		t.Fatalf("expected 500s duration between `clock` and `end`, actually: %v", dur.Seconds())
	}
	clock.Add(100 * time.Second)
	if dur := clock.Until(end); dur.Seconds() != 400 {
		t.Fatalf("expected 400s duration between `clock` and `end`, actually: %v", dur.Seconds())
	}
}

// Ensure that the mock can sleep for the correct time.
func TestMock_Sleep(t *testing.T) {
	var ok int32
	clock := NewMock()

	// Create a channel to execute after 10 mock seconds.
	go func() {
		clock.Sleep(10 * time.Second)
		atomic.StoreInt32(&ok, 1)
	}()
	gosched()

	// Move clock forward to just before the sleep duration.
	clock.Add(9 * time.Second)
	if atomic.LoadInt32(&ok) == 1 {
		t.Fatal("too early")
	}

	// Move clock forward to after the sleep duration.
	clock.Add(1 * time.Second)
	if atomic.LoadInt32(&ok) == 0 {
		t.Fatal("too late")
	}
}

// Ensure that the mock's Tick channel sends at the correct time.
func TestMock_Tick(t *testing.T) {
	var n int32
	clock := NewMock()

	// Create a channel to increment every 10 seconds.
	go func() {
		tick := clock.Tick(10 * time.Second)
		for {
			<-tick
			atomic.AddInt32(&n, 1)
		}
	}()
	gosched()

	// Move clock forward to just before the first tick.
	clock.Add(9 * time.Second)
	if atomic.LoadInt32(&n) != 0 {
		t.Fatalf("expected 0, got %d", n)
	}

	// Move clock forward to the start of the first tick.
	clock.Add(1 * time.Second)
	if atomic.LoadInt32(&n) != 1 {
		t.Fatalf("expected 1, got %d", n)
	}

	// Move clock forward over several ticks.
	clock.Add(30 * time.Second)
	if atomic.LoadInt32(&n) != 4 {
		t.Fatalf("expected 4, got %d", n)
	}
}

// Ensure that the mock's Ticker channel sends at the correct time.
func TestMock_Ticker(t *testing.T) {
	var n int32
	clock := NewMock()

	// Create a channel to increment every microsecond.
	go func() {
		ticker := clock.Ticker(1 * time.Microsecond)
		for {
			<-ticker.C
			atomic.AddInt32(&n, 1)
		}
	}()
	gosched()

	// Move clock forward.
	clock.Add(10 * time.Microsecond)
	if atomic.LoadInt32(&n) != 10 {
		t.Fatalf("unexpected: %d", n)
	}
}

// Ensure that the mock's Ticker channel won't block if not read from.
func TestMock_Ticker_Overflow(t *testing.T) {
	clock := NewMock()
	ticker := clock.Ticker(1 * time.Microsecond)
	clock.Add(10 * time.Microsecond)
	ticker.Stop()
}

// Ensure that the mock's Ticker can be stopped.
func TestMock_Ticker_Stop(t *testing.T) {
	var n int32
	clock := NewMock()

	// Create a channel to increment every second.
	ticker := clock.Ticker(1 * time.Second)
	go func() {
		for {
			<-ticker.C
			atomic.AddInt32(&n, 1)
		}
	}()
	gosched()

	// Move clock forward.
	clock.Add(5 * time.Second)
	if atomic.LoadInt32(&n) != 5 {
		t.Fatalf("expected 5, got: %d", n)
	}

	ticker.Stop()

	// Move clock forward again.
	clock.Add(5 * time.Second)
	if atomic.LoadInt32(&n) != 5 {
		t.Fatalf("still expected 5, got: %d", n)
	}
}

func TestMock_Ticker_Reset(t *testing.T) {
	var n int32
	clock := NewMock()

	ticker := clock.Ticker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			<-ticker.C
			atomic.AddInt32(&n, 1)
		}
	}()
	gosched()

	// Move clock forward.
	clock.Add(10 * time.Second)
	if atomic.LoadInt32(&n) != 2 {
		t.Fatalf("expected 2, got: %d", n)
	}

	clock.Add(4 * time.Second)
	ticker.Reset(5 * time.Second)

	// Advance the remaining second
	clock.Add(1 * time.Second)

	if atomic.LoadInt32(&n) != 2 {
		t.Fatalf("expected 2, got: %d", n)
	}

	// Advance the remaining 4 seconds from the previous tick
	clock.Add(4 * time.Second)

	if atomic.LoadInt32(&n) != 3 {
		t.Fatalf("expected 3, got: %d", n)
	}
}

// Ensure that multiple tickers can be used together.
func TestMock_Ticker_Multi(t *testing.T) {
	var n int32
	clock := NewMock()

	go func() {
		a := clock.Ticker(1 * time.Microsecond)
		b := clock.Ticker(3 * time.Microsecond)

		for {
			select {
			case <-a.C:
				atomic.AddInt32(&n, 1)
			case <-b.C:
				atomic.AddInt32(&n, 100)
			}
		}
	}()
	gosched()

	// Move clock forward.
	clock.Add(10 * time.Microsecond)
	gosched()
	if atomic.LoadInt32(&n) != 310 {
		t.Fatalf("unexpected: %d", n)
	}
}

func ExampleMock_After() {
	// Create a new mock clock.
	clock := NewMock()
	var count counter

	ready := make(chan struct{})
	// Create a channel to execute after 10 mock seconds.
	go func() {
		ch := clock.After(10 * time.Second)
		close(ready)
		<-ch
		count.incr()
	}()
	<-ready

	// Print the starting value.
	fmt.Printf("%s: %d\n", clock.Now().UTC(), count.get())

	// Move the clock forward 5 seconds and print the value again.
	clock.Add(5 * time.Second)
	fmt.Printf("%s: %d\n", clock.Now().UTC(), count.get())

	// Move the clock forward 5 seconds to the tick time and check the value.
	clock.Add(5 * time.Second)
	fmt.Printf("%s: %d\n", clock.Now().UTC(), count.get())

	// Output:
	// 1970-01-01 00:00:00 +0000 UTC: 0
	// 1970-01-01 00:00:05 +0000 UTC: 0
	// 1970-01-01 00:00:10 +0000 UTC: 1
}

func ExampleMock_AfterFunc() {
	// Create a new mock clock.
	clock := NewMock()
	count := 0

	// Execute a function after 10 mock seconds.
	clock.AfterFunc(10*time.Second, func() {
		count = 100
	})
	gosched()

	// Print the starting value.
	fmt.Printf("%s: %d\n", clock.Now().UTC(), count)

	// Move the clock forward 10 seconds and print the new value.
	clock.Add(10 * time.Second)
	fmt.Printf("%s: %d\n", clock.Now().UTC(), count)

	// Output:
	// 1970-01-01 00:00:00 +0000 UTC: 0
	// 1970-01-01 00:00:10 +0000 UTC: 100
}

func ExampleMock_Sleep() {
	// Create a new mock clock.
	clock := NewMock()
	var count counter

	// Execute a function after 10 mock seconds.
	go func() {
		clock.Sleep(10 * time.Second)
		count.incr()
	}()
	gosched()

	// Print the starting value.
	fmt.Printf("%s: %d\n", clock.Now().UTC(), count.get())

	// Move the clock forward 10 seconds and print the new value.
	clock.Add(10 * time.Second)
	fmt.Printf("%s: %d\n", clock.Now().UTC(), count.get())

	// Output:
	// 1970-01-01 00:00:00 +0000 UTC: 0
	// 1970-01-01 00:00:10 +0000 UTC: 1
}

func ExampleMock_Ticker() {
	// Create a new mock clock.
	clock := NewMock()
	var count counter

	ready := make(chan struct{})
	// Increment count every mock second.
	go func() {
		ticker := clock.Ticker(1 * time.Second)
		close(ready)
		for {
			<-ticker.C
			count.incr()
		}
	}()
	<-ready

	// Move the clock forward 10 seconds and print the new value.
	clock.Add(10 * time.Second)
	fmt.Printf("Count is %d after 10 seconds\n", count.get())

	// Move the clock forward 5 more seconds and print the new value.
	clock.Add(5 * time.Second)
	fmt.Printf("Count is %d after 15 seconds\n", count.get())

	// Output:
	// Count is 10 after 10 seconds
	// Count is 15 after 15 seconds
}

func ExampleMock_Timer() {
	// Create a new mock clock.
	clock := NewMock()
	var count counter

	ready := make(chan struct{})
	// Increment count after a mock second.
	go func() {
		timer := clock.Timer(1 * time.Second)
		close(ready)
		<-timer.C
		count.incr()
	}()
	<-ready

	// Move the clock forward 10 seconds and print the new value.
	clock.Add(10 * time.Second)
	fmt.Printf("Count is %d after 10 seconds\n", count.get())

	// Output:
	// Count is 1 after 10 seconds
}

func TestMock_ReentrantDeadlock(t *testing.T) {
	mockedClock := NewMock()
	timer20 := mockedClock.Timer(20 * time.Second)
	go func() {
		v := <-timer20.C
		panic(fmt.Sprintf("timer should not have ticked: %v", v))
	}()
	mockedClock.AfterFunc(10*time.Second, func() {
		timer20.Stop()
	})

	mockedClock.Add(15 * time.Second)
	mockedClock.Add(15 * time.Second)
}

func warn(v ...interface{})              { fmt.Fprintln(os.Stderr, v...) }
func warnf(msg string, v ...interface{}) { fmt.Fprintf(os.Stderr, msg+"\n", v...) }

// ==================== Additional Tests ====================

// Ensure that the clock's Epoch returns a valid Unix timestamp.
func TestClock_Epoch(t *testing.T) {
	c := New()
	epoch := c.Epoch()
	if epoch <= 0 {
		t.Fatalf("expected positive epoch, got: %d", epoch)
	}
	// Should be close to time.Now().Unix()
	expected := time.Now().Unix()
	diff := expected - epoch
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		t.Fatalf("epoch too far from expected: %d vs %d", epoch, expected)
	}
}

// Ensure that the clock's Since works correctly.
func TestClock_Since(t *testing.T) {
	c := New()
	start := c.Now()
	time.Sleep(10 * time.Millisecond)
	dur := c.Since(start)
	if dur < 10*time.Millisecond || dur > 30*time.Millisecond {
		t.Fatalf("Bad duration: %s", dur)
	}
}

// Ensure that the clock's Until works correctly.
func TestClock_Until(t *testing.T) {
	c := New()
	future := c.Now().Add(20 * time.Millisecond)
	dur := c.Until(future)
	if dur > 20*time.Millisecond || dur < 0 {
		t.Fatalf("Bad duration: %s", dur)
	}
}

// Ensure that Mock.Set correctly sets time and triggers timers.
func TestMock_Set(t *testing.T) {
	var ok int32
	clock := NewMock()

	clock.AfterFunc(5*time.Second, func() {
		atomic.StoreInt32(&ok, 1)
	})

	clock.Set(time.Unix(10, 0))
	if atomic.LoadInt32(&ok) != 1 {
		t.Fatal("AfterFunc not triggered by Set")
	}

	if !clock.Now().Equal(time.Unix(10, 0)) {
		t.Fatalf("expected time.Unix(10,0), got %v", clock.Now())
	}
}

// Ensure that Mock.Set backward in time doesn't trigger future timers.
func TestMock_Set_Backward(t *testing.T) {
	clock := NewMock()
	clock.Set(time.Unix(100, 0))
	clock.Set(time.Unix(50, 0))

	if !clock.Now().Equal(time.Unix(50, 0)) {
		t.Fatalf("expected time.Unix(50,0), got %v", clock.Now())
	}
}

// Ensure that Mock.Epoch returns correct value.
func TestMock_Epoch(t *testing.T) {
	clock := NewMock()
	if clock.Epoch() != 0 {
		t.Fatalf("expected epoch 0, got: %d", clock.Epoch())
	}
	clock.Add(10 * time.Second)
	if clock.Epoch() != 10 {
		t.Fatalf("expected epoch 10, got: %d", clock.Epoch())
	}
}

// Ensure that mock timer can be stopped.
func TestMock_Timer_Stop(t *testing.T) {
	clock := NewMock()
	timer := clock.Timer(10 * time.Second)

	if !timer.Stop() {
		t.Fatal("expected timer to be running")
	}
	if timer.Stop() {
		t.Fatal("expected timer to be stopped")
	}

	clock.Add(10 * time.Second)
	select {
	case <-timer.C:
		t.Fatal("unexpected send on stopped timer")
	default:
	}
}

// Ensure that mock timer can be reset.
func TestMock_Timer_Reset(t *testing.T) {
	clock := NewMock()
	timer := clock.Timer(10 * time.Second)

	if !timer.Reset(20 * time.Second) {
		t.Fatal("expected timer to be running")
	}
	clock.Add(10 * time.Second)
	select {
	case <-timer.C:
		t.Fatal("unexpected send too early")
	default:
	}
	clock.Add(10 * time.Second)
	select {
	case <-timer.C:
	default:
		t.Fatal("expected timer to fire after 20s")
	}
}

// Ensure that a stopped mock timer can be reset.
func TestMock_Timer_Reset_Stopped(t *testing.T) {
	clock := NewMock()
	timer := clock.Timer(10 * time.Second)
	timer.Stop()
	timer.Reset(5 * time.Second)

	clock.Add(5 * time.Second)
	select {
	case <-timer.C:
	default:
		t.Fatal("expected timer to fire after reset")
	}
}

// Ensure that New() returns a Clock interface implementation.
func TestNew_Returns_Clock(t *testing.T) {
	var c Clock = New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

// Ensure that NewMock() returns a *Mock.
func TestNewMock_NotNil(t *testing.T) {
	m := NewMock()
	if m == nil {
		t.Fatal("NewMock() returned nil")
	}
}

// ==================== Benchmark Tests ====================

// BenchmarkClock_Now benchmarks the real clock's Now().
func BenchmarkClock_Now(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Now()
	}
}

// BenchmarkClock_Epoch benchmarks the real clock's Epoch().
func BenchmarkClock_Epoch(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Epoch()
	}
}

// BenchmarkClock_Since benchmarks the real clock's Since().
func BenchmarkClock_Since(b *testing.B) {
	c := New()
	start := c.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Since(start)
	}
}

// BenchmarkClock_Until benchmarks the real clock's Until().
func BenchmarkClock_Until(b *testing.B) {
	c := New()
	future := c.Now().Add(time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Until(future)
	}
}

// BenchmarkClock_Timer_Create benchmarks creating Timers on the real clock.
func BenchmarkClock_Timer_Create(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := c.Timer(time.Millisecond)
		timer.Stop()
	}
}

// BenchmarkClock_Timer_Stop benchmarks stopping Timers on the real clock.
func BenchmarkClock_Timer_Stop(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := c.Timer(time.Hour)
		timer.Stop()
	}
}

// BenchmarkClock_Timer_Reset benchmarks resetting Timers on the real clock.
func BenchmarkClock_Timer_Reset(b *testing.B) {
	c := New()
	timer := c.Timer(time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer.Reset(time.Hour)
	}
	timer.Stop()
}

// BenchmarkClock_Ticker_Create benchmarks creating Tickers on the real clock.
func BenchmarkClock_Ticker_Create(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticker := c.Ticker(time.Hour)
		ticker.Stop()
	}
}

// BenchmarkClock_Ticker_Stop benchmarks stopping Tickers on the real clock.
func BenchmarkClock_Ticker_Stop(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticker := c.Ticker(time.Hour)
		ticker.Stop()
	}
}

// BenchmarkClock_AfterFunc benchmarks AfterFunc on the real clock.
func BenchmarkClock_AfterFunc(b *testing.B) {
	c := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := c.AfterFunc(time.Hour, func() {})
		timer.Stop()
	}
}

// BenchmarkMock_Now benchmarks the mock clock's Now().
func BenchmarkMock_Now(b *testing.B) {
	m := NewMock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Now()
	}
}

// BenchmarkMock_Epoch benchmarks the mock clock's Epoch().
func BenchmarkMock_Epoch(b *testing.B) {
	m := NewMock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Epoch()
	}
}

// BenchmarkMock_Since benchmarks the mock clock's Since().
func BenchmarkMock_Since(b *testing.B) {
	m := NewMock()
	start := m.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Since(start)
	}
}

// BenchmarkMock_Until benchmarks the mock clock's Until().
func BenchmarkMock_Until(b *testing.B) {
	m := NewMock()
	future := m.Now().Add(time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Until(future)
	}
}

// BenchmarkMock_Timer_Create benchmarks creating Timers on the mock clock.
func BenchmarkMock_Timer_Create(b *testing.B) {
	m := NewMock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Timer(time.Second)
	}
}

// BenchmarkMock_Timer_Stop benchmarks stopping Timers on the mock clock.
func BenchmarkMock_Timer_Stop(b *testing.B) {
	m := NewMock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := m.Timer(time.Second)
		timer.Stop()
	}
}

// BenchmarkMock_Timer_Reset benchmarks resetting Timers on the mock clock.
func BenchmarkMock_Timer_Reset(b *testing.B) {
	m := NewMock()
	timer := m.Timer(time.Second)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer.Reset(time.Second)
	}
}

// BenchmarkMock_Ticker_Create benchmarks creating Tickers on the mock clock.
func BenchmarkMock_Ticker_Create(b *testing.B) {
	m := NewMock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Ticker(time.Second)
	}
}

// BenchmarkMock_Ticker_Stop benchmarks stopping Tickers on the mock clock.
func BenchmarkMock_Ticker_Stop(b *testing.B) {
	m := NewMock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticker := m.Ticker(time.Second)
		ticker.Stop()
	}
}

// BenchmarkMock_Ticker_Reset benchmarks resetting Tickers on the mock clock.
func BenchmarkMock_Ticker_Reset(b *testing.B) {
	m := NewMock()
	ticker := m.Ticker(time.Second)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticker.Reset(time.Second)
	}
}

// BenchmarkMock_AfterFunc benchmarks AfterFunc on the mock clock.
func BenchmarkMock_AfterFunc(b *testing.B) {
	m := NewMock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timer := m.AfterFunc(time.Second, func() {})
		timer.Stop()
	}
}

// BenchmarkMock_Add benchmarks Add on the mock clock.
func BenchmarkMock_Add(b *testing.B) {
	m := NewMock()
	m.Timer(time.Microsecond) // register one timer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Add(time.Microsecond)
	}
}

// BenchmarkMock_Set benchmarks Set on the mock clock.
func BenchmarkMock_Set(b *testing.B) {
	m := NewMock()
	future := time.Unix(1000, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Set(future)
	}
}

// BenchmarkMock_Add_ManyTimers benchmarks Add with many registered timers.
func BenchmarkMock_Add_ManyTimers(b *testing.B) {
	m := NewMock()
	for i := 0; i < 100; i++ {
		m.Timer(time.Duration(i+1) * time.Second)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Add(time.Microsecond)
	}
}

// BenchmarkMock_Timer_Sort benchmarks the timer list sorting cost.
func BenchmarkMock_Timer_Sort(b *testing.B) {
	m := NewMock()
	for i := 0; i < b.N; i++ {
		if i < 10000 {
			m.Timer(time.Duration(i+1) * time.Second)
		}
	}
	// Note: Add invokes sort; this measures the Add throughput with many timers.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Add(time.Microsecond)
	}
}

// BenchmarkStd_Time_Now benchmarks the standard library time.Now for comparison.
func BenchmarkStd_Time_Now(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = time.Now()
	}
}

// BenchmarkStd_Time_Since benchmarks the standard library time.Since for comparison.
func BenchmarkStd_Time_Since(b *testing.B) {
	start := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = time.Since(start)
	}
}
