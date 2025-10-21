package clock

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSystemClock_Now(t *testing.T) {
	clock := SystemClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Error("Expected non-zero time from Now()")
	}
}

func TestSystemClock_Since(t *testing.T) {
	clock := SystemClock{}
	startTime := time.Now().Add(-1 * time.Second)
	duration := clock.Since(startTime)

	if duration < 1*time.Second {
		t.Errorf("Expected duration >= 1 second, got %v", duration)
	}
}

func TestSystemClock_After(t *testing.T) {
	clock := SystemClock{}
	ch := clock.After(10 * time.Millisecond)

	select {
	case <-ch:
		// 正常情况，按时收到信号
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for After() channel")
	}
}

func TestSystemClock_Tick(t *testing.T) {
	clock := SystemClock{}
	ch := clock.Tick(10 * time.Millisecond)

	// 测试接收两个tick
	ticksReceived := 0
	timeout := time.After(100 * time.Millisecond)

	for ticksReceived < 2 {
		select {
		case <-ch:
			ticksReceived++
		case <-timeout:
			t.Fatal("Timeout waiting for Tick() channel")
		}
	}
}

func TestSystemClock_Ticker(t *testing.T) {
	clock := SystemClock{}
	ticker := clock.Ticker(10 * time.Millisecond)
	defer ticker.Stop()

	// 测试接收两个tick
	ticksReceived := 0
	timeout := time.After(100 * time.Millisecond)

	for ticksReceived < 2 {
		select {
		case <-ticker.C:
			ticksReceived++
		case <-timeout:
			t.Fatal("Timeout waiting for Ticker channel")
		}
	}
}

func TestSystemClock_NewTicker(t *testing.T) {
	clock := SystemClock{}
	ticker := clock.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	// 测试接收两个tick
	ticksReceived := 0
	timeout := time.After(100 * time.Millisecond)

	for ticksReceived < 2 {
		select {
		case <-ticker.C:
			ticksReceived++
		case <-timeout:
			t.Fatal("Timeout waiting for NewTicker channel")
		}
	}
}

func TestSystemClock_AfterFunc(t *testing.T) {
	clock := SystemClock{}
	var executed int32

	timer := clock.AfterFunc(10*time.Millisecond, func() {
		atomic.AddInt32(&executed, 1)
	})
	defer timer.Stop()

	// 等待函数执行
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 1 {
		t.Error("Expected AfterFunc callback to be executed once")
	}
}

func TestSystemClock_NewTimer(t *testing.T) {
	clock := SystemClock{}
	timer := clock.NewTimer(10 * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
		// 正常情况，按时收到信号
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for NewTimer channel")
	}
}

func TestSystemClock_Sleep(t *testing.T) {
	clock := SystemClock{}
	start := time.Now()
	clock.Sleep(10 * time.Millisecond)
	duration := time.Since(start)

	if duration < 10*time.Millisecond {
		t.Errorf("Expected sleep duration >= 10ms, got %v", duration)
	}
}

func TestSystemClock_Epoch(t *testing.T) {
	clock := SystemClock{}
	epoch := clock.Epoch()
	now := time.Now().Unix()

	// 允许1秒的误差
	if epoch < now-1 || epoch > now+1 {
		t.Errorf("Expected epoch close to current time, got %d, now %d", epoch, now)
	}
}
