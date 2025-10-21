package bigcache

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/andrewbytecoder/gokit/clock"
	"github.com/andrewbytecoder/gokit/container/bytesqyeye"
	"go.uber.org/zap"
)

// RemoveReason 是一个值，用于在 OnRemove 回调中向用户指示特定键被移除的原因。
type RemoveReason uint32

// 定义移除原因的常量
const (
	// Expired 表示条目因过期而被移除
	Expired RemoveReason = iota
	// NoSpace 表示由于空间不足而移除条目
	NoSpace
	// Deleted 表示条目被显式删除
	Deleted
)

// onRemoveCallback 定义了当缓存条目被移除时调用的回调函数类型
type onRemoveCallback func(wrappedEntry []byte, reason RemoveReason)

// Metadata 包含特定缓存条目的信息
type Metadata struct {
	// RequestCount 记录该条目被请求的次数
	RequestCount uint32
}

var (
	// ErrEntryNotFound is an error type struct which is returned when entry was not found for provided key
	ErrEntryNotFound = errors.New("entry not found")
)

// cacheShard 表示缓存的一个分片，用于存储实际的缓存数据
type cacheShard struct {
	// hashmap 存储键的哈希值到条目在队列中位置的映射
	hashmap map[uint64]uint64
	// entries 是存储实际缓存数据的字节队列
	entries bytesqyeye.BytesQueue
	// lock 用于保护分片的并发访问
	lock sync.RWMutex
	// entryBuffer 用于临时存储条目数据的缓冲区
	entryBuffer []byte
	// onRemove 是条目被移除时调用的回调函数
	onRemove onRemoveCallback

	// isVerbose 指示是否启用详细日志记录
	isVerbose bool
	// statsEnabled 指示是否启用统计信息收集
	statsEnabled bool
	// logger 用于记录日志
	logger *zap.Logger
	// clock 用于获取当前时间，便于测试
	clock clock.Clock
	// lifeWindow 定义条目的生存时间窗口（以秒为单位）
	lifeWindow uint64

	// hashmapStats 存储每个哈希值的统计信息
	hashmapStats map[uint64]uint32
	// stats 存储分片的统计信息
	stats *Stats
	// cleanEnabled 指示是否启用自动清理过期条目
	cleanEnabled bool
}

func (s *cacheShard) getWrappedEntry(hashedKey uint64) ([]byte, error) {
	itemIndex := s.hashmap[hashedKey]

	if itemIndex == 0 {
		s.miss()
		return nil, ErrEntryNotFound
	}

	wrappedEntry, err := s.entries.Get(int(itemIndex))
	if err != nil {
		s.miss()
		return nil, err
	}
	return wrappedEntry, nil
}

func (s *cacheShard) getStats() Stats {
	var stats = Stats{
		Hits:       atomic.LoadInt64(&s.stats.Hits),
		Misses:     atomic.LoadInt64(&s.stats.Misses),
		DelHits:    atomic.LoadInt64(&s.stats.DelHits),
		DelMisses:  atomic.LoadInt64(&s.stats.DelMisses),
		Collisions: atomic.LoadInt64(&s.stats.Collisions),
	}
	return stats
}

func (s *cacheShard) hit(key uint64) {
	atomic.AddInt64(&s.stats.Hits, 1)
	if s.statsEnabled {
		s.lock.Lock()
		defer s.lock.Unlock()
		s.hashmapStats[key]++
	}
}

func (s *cacheShard) miss() {
	atomic.AddInt64(&s.stats.Misses, 1)
}

func (s *cacheShard) Delhit() {
	atomic.AddInt64(&s.stats.DelHits, 1)
}

func (s *cacheShard) delmiss() {
	atomic.AddInt64(&s.stats.DelMisses, 1)
}

func (s *cacheShard) collision() {
	atomic.AddInt64(&s.stats.Collisions, 1)
}
