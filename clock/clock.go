package clock

import (
	"time"
)

// Clock 接口定义了时钟相关的操作，用于获取时间、创建定时器和执行延时操作
type Clock interface {
	// Now 返回当前时间
	Now() time.Time

	// Since 返回从指定时间到现在的持续时间
	Since(time.Time) time.Duration

	// After 返回一个通道，在指定持续时间后会发送当前时间
	After(time.Duration) <-chan time.Time

	// Tick 返回一个通道，每隔指定持续时间发送当前时间
	// 注意：这个ticker不能被回收，容易造成资源泄漏，建议使用 NewTicker 替代
	Tick(time.Duration) <-chan time.Time

	// Ticker 创建并返回一个新的 ticker
	Ticker(time.Duration) *time.Ticker

	// AfterFunc 在指定持续时间后执行函数f，并返回timer
	AfterFunc(time.Duration, func()) *time.Timer

	// NewTimer 创建并返回一个新的 timer
	NewTimer(time.Duration) *time.Timer

	// NewTicker 创建并返回一个新的 ticker
	NewTicker(time.Duration) *time.Ticker

	// Sleep 使当前goroutine暂停指定的持续时间
	Sleep(time.Duration)

	// Epoch 返回当前时间的Unix时间戳(秒)
	Epoch() int64
}

// SystemClock 是 Clock 接口的默认实现，使用系统时间
type SystemClock struct{}

// Now 返回系统当前时间
func (SystemClock) Now() time.Time {
	return time.Now()
}

// NewTicker 创建并返回一个新的 ticker
func (SystemClock) NewTicker(duration time.Duration) *time.Ticker {
	return time.NewTicker(duration)
}

// Since 返回从指定时间到现在的持续时间
func (SystemClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// After 返回一个通道，在指定持续时间后会发送当前时间
func (SystemClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// Tick 返回一个通道，每隔指定持续时间发送当前时间
// 注意：这个ticker不能被回收，容易造成资源泄漏，建议使用 NewTicker 替代
func (SystemClock) Tick(d time.Duration) <-chan time.Time {
	return time.Tick(d)
}

// Ticker 创建并返回一个新的 ticker（与 NewTicker 功能相同）
func (SystemClock) Ticker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

// AfterFunc 在指定持续时间后执行函数f，并返回timer
func (SystemClock) AfterFunc(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}

// NewTimer 创建并返回一个新的 timer
func (SystemClock) NewTimer(d time.Duration) *time.Timer {
	return time.NewTimer(d)
}

// Sleep 使当前goroutine暂停指定的持续时间
func (SystemClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// Epoch 返回当前时间的Unix时间戳(秒)
func (SystemClock) Epoch() int64 {
	return time.Now().Unix()
}
