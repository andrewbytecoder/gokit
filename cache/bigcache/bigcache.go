package bigcache

import (
	"context"
	"errors"
	"time"

	"github.com/andrewbytecoder/gokit/clock"
	"github.com/andrewbytecoder/gokit/hash"
	"github.com/andrewbytecoder/gokit/math"
)

const (
	minimumEntriesInShard = 10 // Minimum number of entries in single shard
)

// BigCache 是一个快速、并发、可淘汰的缓存，用于存储大量条目而不会影响性能
// 它将条目保存在堆上但避免了GC对它们的影响。为了实现这一点，操作在字节数组上进行，
// 因此在大多数使用场景下，缓存前需要对条目进行(反)序列化。
type BigCache struct {
	shards     []*cacheShard // 缓存分片数组
	lifeWindow uint64        // 条目生存时间窗口（秒）
	clock      clock.Clock   // 时钟接口，用于获取时间
	hash       hash.Hasher   // 哈希函数接口
	config     Config        // 缓存配置
	shardMask  uint64        // 分片掩码，用于快速计算分片索引
	close      chan struct{} // 关闭信号通道
}

// New 初始化 BigCache 的新实例
// 参数:
//
//	ctx: 上下文，用于控制清理goroutine的生命周期
//	config: 缓存配置
//
// 返回值:
//
//	*BigCache: BigCache实例指针
//	error: 错误信息
func New(ctx context.Context, config Config) (*BigCache, error) {
	return newBigCache(ctx, config, clock.SystemClock{})
}

// NewBigCache 创建 BigCache 实例的旧版本接口（向后兼容）
// 参数:
//
//	config: 缓存配置
//
// 返回值:
//
//	*BigCache: BigCache实例指针
//	error: 错误信息
func NewBigCache(config Config) (*BigCache, error) {
	return newBigCache(context.Background(), config, clock.SystemClock{})
}

// newBigCache BigCache 的核心构造函数
// 参数:
//
//	ctx: 上下文
//	config: 缓存配置
//	clock: 时钟接口
//
// 返回值:
//
//	*BigCache: BigCache实例指针
//	error: 错误信息
func newBigCache(ctx context.Context, config Config, clock clock.Clock) (*BigCache, error) {
	if !math.IsPowerOfTwo(config.Shards) {
		return nil, errors.New("shards number must be power of two")
	}
	if config.MaxEntrySize < 0 {
		return nil, errors.New("MaxEntrySize must be >= 0")
	}
	if config.MaxEntriesInWindow < 0 {
		return nil, errors.New("MaxEntriesInWindow must be >= 0")
	}
	if config.HardMaxCacheSize < 0 {
		return nil, errors.New("HardMaxCacheSize must be >= 0")
	}

	lifeWindowSeconds := uint64(config.LifeWindow.Seconds())
	if config.CleanWindow > 0 && lifeWindowSeconds == 0 {
		return nil, errors.New("LifeWindow must be > 0 when CleanWindow is set")
	}

	if config.Hasher == nil {
		config.Hasher = hash.NewFnv64()
	}

	cache := &BigCache{
		shards:     make([]*cacheShard, config.Shards),
		lifeWindow: lifeWindowSeconds,
		clock:      clock,
		hash:       config.Hasher,
		config:     config,
		shardMask:  uint64(config.Shards - 1),
		close:      make(chan struct{}),
	}

	var onRemove func(wrappedEntry []byte, reason RemoveReason)
	if config.OnRemoveWithMetadata != nil {
		onRemove = cache.provideOnRemoveWithMetadata
	} else if config.OnRemove != nil {
		onRemove = cache.providedOnRemove
	} else if config.OnRemoveWithReason != nil {
		onRemove = cache.providedOnRemoveWithReason
	} else {
		onRemove = cache.notProvideOnRemove
	}
	for i := 0; i < config.Shards; i++ {
		cache.shards[i] = initNewShard(config, onRemove, clock)
	}

	if config.CleanWindow > 0 {
		go func() {
			ticker := time.NewTicker(config.CleanWindow)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-ticker.C: // <-ticker.C 发送当前时间点 time.Time类型数据
					cache.cleanUp(uint64(t.Unix()))
				case <-cache.close:
					return
				}
			}
		}()
	}

	return cache, nil
}

// Close 用于在使用完缓存后发出关闭信号
// 这允许清理goroutine退出，并确保不保留对缓存的引用，从而允许GC回收条目缓存
// 返回值:
//
//	error: 错误信息
func (c *BigCache) Close() error {
	close(c.close)
	return nil
}

// Get 根据键读取条目
// 当给定键不存在条目时返回 ErrEntryNotFound 错误
// 参数:
//
//	key: 要查找的键
//
// 返回值:
//
//	[]byte: 条目数据
//	error: 错误信息
func (c *BigCache) Get(key string) ([]byte, error) {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.get(key, hashedKey)
}

// GetWithInfo 根据键读取条目并返回响应信息
// 当给定键不存在条目时返回 ErrEntryNotFound 错误
// 参数:
//
//	key: 要查找的键
//
// 返回值:
//
//	[]byte: 条目数据
//	Response: 响应信息
//	error: 错误信息
func (c *BigCache) GetWithInfo(key string) ([]byte, Response, error) {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.getWithInfo(key, hashedKey)
}

// Set 在键下保存条目
// 参数:
//
//	key: 键
//	entry: 要保存的条目数据
//
// 返回值:
//
//	error: 错误信息
func (c *BigCache) Set(key string, entry []byte) error {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.set(key, hashedKey, entry)
}

// Append 如果键存在则在键下追加条目，否则行为与 Set() 相同
// 使用 Append() 可以以锁优化的方式在同一个键下连接多个条目
// 参数:
//
//	key: 键
//	entry: 要追加的条目数据
//
// 返回值:
//
//	error: 错误信息
func (c *BigCache) Append(key string, entry []byte) error {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.append(key, hashedKey, entry)
}

// Delete 删除指定键
// 参数:
//
//	key: 要删除的键
//
// 返回值:
//
//	error: 错误信息
func (c *BigCache) Delete(key string) error {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.del(hashedKey)
}

// Reset 清空所有缓存分片
// 返回值:
//
//	error: 错误信息
func (c *BigCache) Reset() error {
	for _, shard := range c.shards {
		shard.reset(c.config)
	}
	return nil
}

// ResetStats 重置缓存统计信息
// 返回值:
//
//	error: 错误信息
func (c *BigCache) ResetStats() error {
	for _, shard := range c.shards {
		shard.resetStats()
	}
	return nil
}

// Len 计算缓存中的条目数量
// 返回值:
//
//	int: 缓存中条目的总数
func (c *BigCache) Len() int {
	var cacheLen int
	for _, shard := range c.shards {
		cacheLen += shard.len()
	}
	return cacheLen
}

// Capacity 返回缓存中存储的字节数
// 返回值:
//
//	int: 缓存中存储的字节总数
func (c *BigCache) Capacity() int {
	var cacheLen int
	for _, shard := range c.shards {
		cacheLen += shard.capacity()
	}
	return cacheLen
}

// Stats 返回缓存的统计信息
// 返回值:
//
//	Stats: 缓存统计信息
func (c *BigCache) Stats() Stats {
	var s Stats
	for _, shard := range c.shards {
		tmp := shard.getStats()
		s.Hits += tmp.Hits
		s.Misses += tmp.Misses
		s.DelHits += tmp.DelHits
		s.DelMisses += tmp.DelMisses
		s.Collisions += tmp.Collisions
	}
	return s
}

// KeyMetadata 返回缓存资源被请求的次数
// 参数:
//
//	key: 要查询的键
//
// 返回值:
//
//	Metadata: 键的元数据信息
func (c *BigCache) KeyMetadata(key string) Metadata {
	hashedKey := c.hash.Sum64(key)
	shard := c.getShard(hashedKey)
	return shard.getKeyMetadataWithLock(hashedKey)
}

// Iterator 返回迭代器函数，用于遍历整个缓存中的 EntryInfo
// 返回值:
//
//	*EntryInfoIterator: 条目信息迭代器
func (c *BigCache) Iterator() *EntryInfoIterator {
	return newIterator(c)
}

// onEvict 检查条目是否过期并执行淘汰操作
// 参数:
//
//	oldestEntry: 最旧的条目
//	currentTimestamp: 当前时间戳
//	evict: 淘汰函数
//
// 返回值:
//
//	bool: 如果条目已过期并被成功淘汰则返回true，否则返回false
func (c *BigCache) onEvict(oldestEntry []byte, currentTimestamp uint64, evict func(reason RemoveReason) error) bool {
	oldestTimestamp := readTimestampFromEntry(oldestEntry)
	if currentTimestamp < oldestTimestamp {
		return false
	}
	if currentTimestamp-oldestTimestamp > c.lifeWindow {
		err := evict(Expired)
		if err != nil {
			return false
		}
		return true
	}
	return false
}

// cleanUp 清理过期条目
// 参数:
//
//	currentTimestamp: 当前时间戳
func (c *BigCache) cleanUp(currentTimestamp uint64) {
	for _, shard := range c.shards {
		shard.cleanUp(currentTimestamp)
	}
}

// getShard 根据哈希键获取对应的分片
// 参数:
//
//	hashedKey: 哈希键
//
// 返回值:
//
//	*cacheShard: 对应的缓存分片
func (c *BigCache) getShard(hashedKey uint64) (shard *cacheShard) {
	return c.shards[hashedKey&c.shardMask]
}

// providedOnRemove 处理条目移除的回调函数（基础版本）
// 参数:
//
//	wrappedEntry: 包装的条目
//	reason: 移除原因
func (c *BigCache) providedOnRemove(wrappedEntry []byte, reason RemoveReason) {
	c.config.OnRemove(readKeyFromEntry(wrappedEntry), readEntry(wrappedEntry))
}

// providedOnRemoveWithReason 处理条目移除的回调函数（带原因版本）
// 参数:
//
//	wrappedEntry: 包装的条目
//	reason: 移除原因
func (c *BigCache) providedOnRemoveWithReason(wrappedEntry []byte, reason RemoveReason) {
	if c.config.onRemoveFilter == 0 || (1<<uint(reason))&c.config.onRemoveFilter > 0 {
		c.config.OnRemoveWithReason(readKeyFromEntry(wrappedEntry), readEntry(wrappedEntry), reason)
	}
}

// notProvideOnRemove 空的条目移除回调函数
// 参数:
//
//	wrappedEntry: 包装的条目
//	reason: 移除原因
func (c *BigCache) notProvideOnRemove(wrappedEntry []byte, reason RemoveReason) {}

// provideOnRemoveWithMetadata 处理条目移除的回调函数（带元数据版本）
// 参数:
//
//	wrappedEntry: 包装的条目
//	reason: 移除原因
func (c *BigCache) provideOnRemoveWithMetadata(wrappedEntry []byte, reason RemoveReason) {
	key := readKeyFromEntry(wrappedEntry)

	hashKey := c.hash.Sum64(key)
	shard := c.getShard(hashKey)
	c.config.OnRemoveWithMetadata(key, readEntry(wrappedEntry), shard.getKeyMetadata(hashKey))
}
