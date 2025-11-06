package mutex

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/andrewbytecoder/gokit/sys/goid"
)

// RecursiveMutex 包装一个Mutex，实现可重入
type RecursiveMutex struct {
	sync.Mutex       // 嵌入Mutex以提供基础互斥功能
	owner      int64 // 当前持有锁的goroutine的id
	recursion  int32 // 当前goroutine的重入次数
}

// Lock 获取锁，支持同goroutine多次获取（重入）
func (m *RecursiveMutex) Lock() {
	g := goid.GetGoroutineId() // 获取当前goroutine ID

	// 如果当前锁的拥有者是当前goroutine，说明是重入
	if atomic.LoadInt64(&m.owner) == int64(g) {
		m.recursion++ // 重入计数加一
		return
	}

	// 首次获取锁，调用底层Mutex.Lock()
	m.Mutex.Lock()

	// 记录锁的拥有者为当前goroutine，并初始化重入计数为1
	atomic.StoreInt64(&m.owner, int64(g))
	m.recursion = 1
}

// Unlock 释放锁，只有当递归计数归零时才真正释放
func (m *RecursiveMutex) Unlock() {
	g := goid.GetGoroutineId() // 获取当前goroutine ID

	// 检查解锁操作是否由锁的拥有者执行
	if atomic.LoadInt64(&m.owner) != int64(g) {
		panic(fmt.Sprintf("wrong the owner(%d): %d!", m.owner, g))
	}

	// 减少重入计数
	m.recursion--

	// 只有当重入计数归零时才真正释放锁
	if m.recursion != 0 {
		return
	}

	// 清除锁拥有者并调用底层Mutex.Unlock()
	atomic.StoreInt64(&m.owner, -1)
	m.Mutex.Unlock()
}
