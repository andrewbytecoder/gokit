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
	Expired = RemoveReason(1)
	// NoSpace 表示由于空间不足而移除条目
	NoSpace = RemoveReason(2)
	// Deleted 表示条目被显式删除
	Deleted = RemoveReason(3)
)

// onRemoveCallback 定义了当缓存条目被移除时调用的回调函数类型
type onRemoveCallback func(wrappedEntry []byte, reason RemoveReason)

// Metadata 包含特定缓存条目的信息
type Metadata struct {
	// RequestCount 记录该条目被请求的次数
	RequestCount uint32
}

// Response will contain metadata about the entry for witch GetWithInfo(key) was called
type Response struct {
	EntryStatus RemoveReason
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
	stats Stats
	// cleanEnabled 指示是否启用自动清理过期条目
	cleanEnabled bool
}

// getWithInfo 根据键和哈希值获取缓存条目，并返回条目的额外信息
// 参数:
//
//	key: 要查找的键字符串
//	hashedKey: 键的哈希值
//
// 返回值:
//
//	entry: 找到的条目数据
//	resp: 条目的响应信息，包含条目状态等
//	err: 错误信息，如果查找失败则返回相应错误
func (s *cacheShard) getWithInfo(key string, hashedKey uint64) (entry []byte, resp Response, err error) {
	currentTime := uint64(s.clock.Epoch())            // 获取当前时间戳（秒）
	s.lock.RLock()                                    // 获取读锁以保证并发安全
	wrappedEntry, err := s.getWrappedEntry(hashedKey) // 根据哈希值获取包装的条目
	if err != nil {                                   // 如果获取条目失败
		s.lock.RUnlock()      // 释放读锁
		return nil, resp, err // 返回错误
	}

	if entryKey := readKeyFromEntry(wrappedEntry); key != entryKey { // 比较键是否匹配（处理哈希冲突）
		s.lock.RUnlock() // 释放读锁
		s.collision()    // 记录哈希冲突统计
		if s.isVerbose { // 如果启用了详细日志
			s.logger.Info("Collision detected", zap.String("key", key), zap.Uint64("hashedKey", hashedKey),
				zap.String("entryKey", entryKey)) // 记录哈希冲突日志
		}
		return nil, resp, ErrEntryNotFound // 返回条目未找到错误
	}

	entry = readEntry(wrappedEntry)             // 从包装条目中提取实际数据
	if s.isExpired(wrappedEntry, currentTime) { // 检查条目是否已过期
		resp.EntryStatus = Expired // 设置条目状态为已过期
	}

	s.lock.RUnlock()        // 释放读锁
	s.hit(hashedKey)        // 记录命中统计
	return entry, resp, nil // 返回条目数据、响应信息和nil错误
}

// get 根据键和哈希值获取缓存条目
// 参数:
//
//	key: 要查找的键字符串
//	hashedKey: 键的哈希值
//
// 返回值:
//
//	[]byte: 找到的条目数据
//	error: 错误信息，如果查找失败则返回相应错误
func (s *cacheShard) get(key string, hashedKey uint64) ([]byte, error) {
	s.lock.RLock()                                    // 获取读锁以保证并发安全
	wrappedEntry, err := s.getWrappedEntry(hashedKey) // 根据哈希值获取包装的条目
	if err != nil {                                   // 如果获取条目失败
		s.lock.RUnlock() // 释放读锁
		return nil, err  // 返回错误
	}

	if entryKey := readKeyFromEntry(wrappedEntry); key != entryKey { // 比较键是否匹配（处理哈希冲突）
		s.lock.RUnlock() // 释放读锁
		s.collision()    // 记录哈希冲突统计
		if s.isVerbose { // 如果启用了详细日志
			s.logger.Info("Collision detected", zap.String("key", key), zap.Uint64("hashedKey", hashedKey),
				zap.String("entryKey", entryKey)) // 记录哈希冲突日志
		}
		return nil, ErrEntryNotFound // 返回条目未找到错误
	}
	entry := readEntry(wrappedEntry) // 从包装条目中提取实际数据
	s.lock.RUnlock()                 // 释放读锁
	s.hit(hashedKey)                 // 记录命中统计
	return entry, nil                // 返回条目数据和nil错误
}

// getWrappedEntry 根据哈希键获取包装的条目数据
// 参数:
//
//	hashedKey: 键的哈希值
//
// 返回值:
//
//	[]byte: 包装的条目数据
//	error: 错误信息，如果找不到条目或获取失败则返回相应错误
func (s *cacheShard) getWrappedEntry(hashedKey uint64) ([]byte, error) {
	itemIndex := s.hashmap[hashedKey] // 从hashmap中获取条目在队列中的索引

	if itemIndex == 0 { // 如果索引为0，表示条目不存在
		s.miss()                     // 记录未命中统计
		return nil, ErrEntryNotFound // 返回条目未找到错误
	}

	wrappedEntry, err := s.entries.Get(int(itemIndex)) // 从字节队列中获取包装的条目数据
	if err != nil {                                    // 如果获取失败
		s.miss()        // 记录未命中统计
		return nil, err // 返回错误
	}
	return wrappedEntry, nil // 返回找到的包装条目数据
}

// getValidWrapEntry 获取有效的包装条目，验证键是否匹配
// 参数:
//
//	key: 要验证的键字符串
//	hashedKey: 键的哈希值
//
// 返回值:
//
//	[]byte: 包装的条目数据
//	error: 错误信息，如果条目不存在、键不匹配或发生错误则返回相应错误
func (s *cacheShard) getValidWrapEntry(key string, hashedKey uint64) ([]byte, error) {
	wrappedEntry, err := s.getWrappedEntry(hashedKey) // 获取包装的条目
	if err != nil {                                   // 如果获取失败
		return nil, err // 返回错误
	}

	if !compareKeyFromEntry(wrappedEntry, key) { // 比较条目中的键与提供的键是否匹配
		s.collision()    // 记录哈希冲突统计
		if s.isVerbose { // 如果启用了详细日志
			// hash 冲突
			s.logger.Info("Collision detected", zap.String("key", key),
				zap.String("wrappedKey", readKeyFromEntry(wrappedEntry))) // 记录哈希冲突日志
		}

		return nil, ErrEntryNotFound // 返回条目未找到错误
	}
	s.hitWithoutLock(hashedKey) // 记录命中统计（不使用锁）
	return wrappedEntry, nil    // 返回验证通过的包装条目
}

// set 在缓存中设置键值对
// 参数:
//
//	key: 要设置的键
//	hashedKey: 键的哈希值
//	entry: 要存储的值
//
// 返回值:
//
//	error: 错误信息，如果设置失败则返回相应错误
func (s *cacheShard) set(key string, hashedKey uint64, entry []byte) error {
	currentTimestamp := uint64(s.clock.Epoch()) // 获取当前时间戳

	s.lock.Lock() // 获取写锁以保证并发安全

	if previousIndex := s.hashmap[hashedKey]; previousIndex != 0 { // 如果已存在相同哈希键的条目
		if previousEntry, err := s.entries.Get(int(previousIndex)); err == nil { // 获取旧条目
			resetHashFromEntry(previousEntry) // 重置旧条目的哈希值
			// remove hashkey
			delete(s.hashmap, hashedKey) // 从hashmap中删除旧条目索引
		}
	}

	if !s.cleanEnabled { // 如果未启用自动清理
		if oldestEntry, err := s.entries.Peek(); err == nil { // 查看最旧的条目
			s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry) // 尝试淘汰过期条目
		}
	}

	w := wrapEntry(currentTimestamp, hashedKey, key, entry, &s.entryBuffer) // 包装条目数据

	for {
		if index, err := s.entries.Push(w); err == nil { // 尝试将包装条目推入队列
			s.hashmap[hashedKey] = uint64(index) // 更新hashmap中的索引
			s.lock.Unlock()                      // 释放写锁
			return nil                           // 返回成功
		}
		// 上面push失败，这里还pop不了，只能是容量不够
		if s.removeOldestEntry(NoSpace) != nil { // 尝试删除最旧条目以腾出空间
			s.lock.Unlock()                                          // 释放写锁
			return errors.New("entry is bigger than max shard size") // 返回条目过大错误
		}
	}
}

// addNewWithoutLock 在不持有写锁的情况下添加新的缓存条目
// 参数:
//
//	key: 要添加的键
//	hashedKey: 键的哈希值
//	entry: 要存储的值
//
// 返回值:
//
//	error: 错误信息，如果添加失败则返回相应错误
//
// 注意: 调用此函数前必须已经持有写锁
func (s *cacheShard) addNewWithoutLock(key string, hashedKey uint64, entry []byte) error {
	currentTimestamp := uint64(s.clock.Epoch()) // 获取当前时间戳

	if !s.cleanEnabled { // 如果未启用自动清理
		if oldestEntry, err := s.entries.Peek(); err == nil { // 查看最旧的条目
			s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry) // 尝试淘汰过期条目
		}
	}

	w := wrapEntry(currentTimestamp, hashedKey, key, entry, &s.entryBuffer) // 包装条目数据

	for {
		if index, err := s.entries.Push(w); err == nil { // 尝试将包装条目推入队列
			s.hashmap[hashedKey] = uint64(index) // 更新hashmap中的索引
			return nil                           // 返回成功
		}
		// 上面push失败，这里还pop不了，只能是容量不够
		if s.removeOldestEntry(NoSpace) != nil { // 尝试删除最旧条目以腾出空间
			return errors.New("entry is bigger than max shard size") // 返回条目过大错误
		}
	}
}

// setWrappedEntryWithoutLock 在不持有写锁的情况下设置包装条目
// 参数:
//
//	currentTimestamp: 当前时间戳
//	w: 已经包装好的条目数据
//	hashedKey: 键的哈希值
//
// 返回值:
//
//	error: 错误信息，如果设置失败则返回相应错误
//
// 注意: 调用此函数前必须已经持有写锁
func (s *cacheShard) setWrappedEntryWithoutLock(currentTimestamp uint64, w []byte, hashedKey uint64) error {
	if previousIndex := s.hashmap[hashedKey]; previousIndex != 0 { // 如果已存在相同哈希键的条目
		if previousEntry, err := s.entries.Get(int(previousIndex)); err == nil { // 获取旧条目
			resetHashFromEntry(previousEntry) // 重置旧条目的哈希值
		}
	}

	if !s.cleanEnabled { // 如果未启用自动清理
		if oldestEntry, err := s.entries.Peek(); err == nil { // 查看最旧的条目
			s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry) // 尝试淘汰过期条目
		}
	}

	for {
		// 将新的地址索引放到对应的hash中
		if index, err := s.entries.Push(w); err == nil { // 尝试将包装条目推入队列
			s.hashmap[hashedKey] = uint64(index) // 更新hashmap中的索引
			return nil                           // 返回成功
		}
		// 上面push失败，这里还pop不了，只能是容量不够
		if s.removeOldestEntry(NoSpace) != nil { // 尝试删除最旧条目以腾出空间
			return errors.New("entry is bigger than max shard size") // 返回条目过大错误
		}
	}
}

// append 将新的数据追加到已存在的键对应的条目中
// 参数:
//
//	key: 要追加数据的键
//	hashedKey: 键的哈希值
//	entry: 要追加的数据
//
// 返回值:
//
//	error: 错误信息，如果追加失败则返回相应错误
func (s *cacheShard) append(key string, hashedKey uint64, entry []byte) error {
	s.lock.Lock()                                            // 获取写锁以保证并发安全
	wrappedEntry, err := s.getValidWrapEntry(key, hashedKey) // 获取有效的包装条目

	if errors.Is(err, ErrEntryNotFound) { // 如果条目不存在
		err = s.addNewWithoutLock(key, hashedKey, entry) // 添加新条目
		s.lock.Unlock()                                  // 释放写锁
		return err                                       // 返回结果
	}
	if err != nil { // 如果发生其他错误
		s.lock.Unlock() // 释放写锁
		return err      // 返回错误
	}

	currentTimestamp := uint64(s.clock.Epoch()) // 获取当前时间戳
	// 将新内容追加到旧内容后面
	w := appendToWrappedEntry(currentTimestamp, wrappedEntry, entry, &s.entryBuffer) // 将新数据追加到现有条目

	err = s.setWrappedEntryWithoutLock(currentTimestamp, w, hashedKey) // 设置更新后的条目
	s.lock.Unlock()                                                    // 释放写锁

	return err // 返回结果
}

// del 根据哈希键删除缓存条目
// 参数:
//
//	hashedKey: 要删除条目的哈希键
//
// 返回值:
//
//	error: 错误信息，如果删除失败则返回相应错误
func (s *cacheShard) del(hashedKey uint64) error {
	// Optimistic pre-check using only readlock
	// 使用乐观预检查，只使用读锁来提高性能
	s.lock.RLock()
	{
		itemIndex := s.hashmap[hashedKey] // 获取条目在队列中的索引
		if itemIndex == 0 {               // 如果索引为0，表示条目不存在
			s.lock.RUnlock()        // 释放读锁
			s.delmiss()             // 记录删除未命中统计
			return ErrEntryNotFound // 返回条目未找到错误
		}

		if err := s.entries.CheckGet(int(itemIndex)); err != nil { // 检查条目是否存在
			s.lock.RUnlock() // 释放读锁
			s.delmiss()      // 记录删除未命中统计
			return err       // 返回错误
		}
	}
	s.lock.RUnlock() // 释放读锁

	s.lock.Lock() // 获取写锁以保证并发安全
	{
		// After obtaining the write lock, we need to read the same again
		// since the data delivered earlier may be stale now
		// 获取写锁后需要再次读取，因为之前的数据可能已经过时
		itemIndex := s.hashmap[hashedKey] // 再次获取条目索引

		if itemIndex == 0 { // 如果索引为0，表示条目已被删除
			s.lock.Unlock()         // 释放写锁
			s.delmiss()             // 记录删除未命中统计
			return ErrEntryNotFound // 返回条目未找到错误
		}

		wrappedEntry, err := s.entries.Get(int(itemIndex)) // 获取包装的条目数据
		if err != nil {                                    // 如果获取失败
			s.lock.Unlock() // 释放写锁
			s.delmiss()     // 记录删除未命中统计
			return err      // 返回错误
		}

		delete(s.hashmap, hashedKey)      // 从hashmap中删除条目索引
		s.onRemove(wrappedEntry, Deleted) // 调用删除回调函数
		if s.statsEnabled {               // 如果启用了统计
			delete(s.hashmapStats, hashedKey) // 删除统计信息
		}
		resetHashFromEntry(wrappedEntry) // 重置条目中的哈希值
	}
	s.lock.Unlock() // 释放写锁
	s.delhit()      // 记录删除命中统计

	return nil // 返回成功
}

// onEvict 检查条目是否过期并执行淘汰操作
// 参数:
//
//	oldestEntry: 要检查的最旧条目
//	currentTimestamp: 当前时间戳
//	evict: 淘汰函数
//
// 返回值:
//
//	bool: 如果条目已过期并被成功淘汰则返回true，否则返回false
func (s *cacheShard) onEvict(oldestEntry []byte, currentTimestamp uint64, evict func(reason RemoveReason) error) bool {
	if s.isExpired(oldestEntry, currentTimestamp) { // 检查条目是否过期
		err := evict(Expired) // 调用淘汰函数
		if err != nil {       // 如果淘汰失败
			return false // 返回false
		}
		return true // 返回true表示成功淘汰
	}
	return false // 返回false表示未淘汰
}

// isExpired 检查条目是否已过期
// 参数:
//
//	oldestEntry: 要检查的条目
//	currentTimestamp: 当前时间戳
//
// 返回值:
//
//	bool: 如果条目已过期则返回true，否则返回false
func (s *cacheShard) isExpired(oldestEntry []byte, currentTimestamp uint64) bool {
	oldestTimestamp := readTimestampFromEntry(oldestEntry) // 从条目中读取时间戳
	if currentTimestamp <= oldestTimestamp {               // 如果当前时间小于等于条目时间（防止溢出）
		return false // 返回未过期
	}
	return currentTimestamp-oldestTimestamp > s.lifeWindow // 检查是否超过生存时间窗口
}

// cleanUp 清理过期条目
// 参数:
//
//	currentTimestamp: 当前时间戳
func (s *cacheShard) cleanUp(currentTimestamp uint64) {
	s.lock.Lock() // 获取写锁
	for {
		if oldestEntry, err := s.entries.Peek(); err != nil { // 查看最旧条目
			break // 如果出错则退出循环
		} else if evicted := s.onEvict(oldestEntry, currentTimestamp, s.removeOldestEntry); !evicted { // 尝试淘汰过期条目
			break // 如果未淘汰则退出循环
		}
	}
	s.lock.Unlock() // 释放写锁
}

// getEntry 根据哈希键获取条目数据的副本
// 参数:
//
//	hashedKey: 条目的哈希键
//
// 返回值:
//
//	[]byte: 条目数据的副本
//	error: 错误信息
func (s *cacheShard) getEntry(hashedKey uint64) ([]byte, error) {
	s.lock.RLock()         // 获取读锁
	defer s.lock.RUnlock() // 函数结束时释放读锁

	entry, err := s.getWrappedEntry(hashedKey) // 获取包装的条目
	// copy entry
	newEntry := make([]byte, len(entry)) // 创建新切片用于复制数据
	copy(newEntry, entry)                // 复制条目数据

	return newEntry, err // 返回复制的条目数据和错误信息
}

// copyHashKeys 复制所有哈希键
// 返回值:
//
//	[]uint64: 所有哈希键的切片
//	int: 键的数量
func (s *cacheShard) copyHashKeys() (keys []uint64, next int) {
	s.lock.RLock()                        // 获取读锁
	defer s.lock.RUnlock()                // 函数结束时释放读锁
	keys = make([]uint64, len(s.hashmap)) // 创建足够大的切片

	for key := range s.hashmap { // 遍历所有哈希键
		keys[next] = key // 复制键到切片
		next++           // 增加计数器
	}

	return keys, next // 返回键切片和数量
}

// removeOldestEntry 删除最旧的条目
// 参数:
//
//	reason: 删除原因
//
// 返回值:
//
//	error: 错误信息
func (s *cacheShard) removeOldestEntry(reason RemoveReason) error {
	oldest, err := s.entries.Pop() // 弹出最旧的条目
	if err == nil {                // 如果成功弹出
		hash := readHashFromEntry(oldest) // 读取条目中的哈希值
		if hash == 0 {                    // 如果哈希值为0（已被删除）
			// entry has been explicitly deleted with resetHashFromEntry, ignore
			// 条目已被显式删除，忽略
			return nil // 返回成功
		}
		delete(s.hashmap, hash)    // 从hashmap中删除条目
		s.onRemove(oldest, reason) // 调用删除回调函数
		if s.statsEnabled {        // 如果启用了统计
			delete(s.hashmapStats, hash) // 删除统计信息
		}
		return nil // 返回成功
	}
	return err // 返回错误
}

// reset 重置缓存分片
// 参数:
//
//	config: 配置信息
func (s *cacheShard) reset(config Config) {
	s.lock.Lock()                                                        // 获取写锁
	s.hashmap = make(map[uint64]uint64, config.initialShardSize())       // 重新创建hashmap
	s.entryBuffer = make([]byte, config.MaxEntrySize+headersSizeInBytes) // 重新创建条目缓冲区
	s.entries.Reset()                                                    // 重置字节队列
	s.lock.Unlock()                                                      // 释放写锁
}

// resetStats 重置缓存分片的统计信息
func (s *cacheShard) resetStats() {
	s.lock.Lock()     // 获取写锁以保证并发安全
	s.stats = Stats{} // 重置统计信息
	s.lock.Unlock()   // 释放写锁
}

// len 返回缓存分片中条目的数量
// 返回值: hashmap中条目的数量
func (s *cacheShard) len() int {
	s.lock.RLock()        // 获取读锁以保证并发安全
	res := len(s.hashmap) // 获取hashmap中条目的数量
	s.lock.RUnlock()      // 释放读锁
	return res            // 返回条目数量
}

// capacity 返回缓存分片的容量
// 返回值: 字节队列的容量
func (s *cacheShard) capacity() int {
	s.lock.RLock()              // 获取读锁以保证并发安全
	res := s.entries.Capacity() // 获取字节队列的容量
	s.lock.RUnlock()            // 释放读锁
	return res                  // 返回容量
}

// getStats 获取缓存分片的统计信息
// 返回值: 包含各项统计信息的Stats结构体
func (s *cacheShard) getStats() Stats {
	var stats = Stats{
		Hits:       atomic.LoadInt64(&s.stats.Hits),       // 原子加载命中次数
		Misses:     atomic.LoadInt64(&s.stats.Misses),     // 原子加载未命中次数
		DelHits:    atomic.LoadInt64(&s.stats.DelHits),    // 原子加载删除命中次数
		DelMisses:  atomic.LoadInt64(&s.stats.DelMisses),  // 原子加载删除未命中次数
		Collisions: atomic.LoadInt64(&s.stats.Collisions), // 原子加载哈希冲突次数
	}
	return stats // 返回统计信息
}

// getKeyMetadataWithLock 获取指定键的元数据（带锁保护）
// 参数:
//
//	key: 要获取元数据的键
//
// 返回值: 包含键请求次数的Metadata结构体
func (s *cacheShard) getKeyMetadataWithLock(key uint64) Metadata {
	s.lock.RLock()           // 获取读锁以保证并发安全
	c := s.hashmapStats[key] // 获取键的请求次数统计
	s.lock.RUnlock()         // 释放读锁
	return Metadata{
		RequestCount: c, // 设置请求次数
	}
}

// getKeyMetadata 获取指定键的元数据（无锁版本）
// 参数:
//
//	key: 要获取元数据的键
//
// 返回值: 包含键请求次数的Metadata结构体
func (s *cacheShard) getKeyMetadata(key uint64) Metadata {
	return Metadata{
		RequestCount: s.hashmapStats[key], // 直接获取键的请求次数统计
	}
}

// hit 记录缓存命中事件
// 参数:
//
//	key: 命中的键
func (s *cacheShard) hit(key uint64) {
	atomic.AddInt64(&s.stats.Hits, 1) // 原子增加命中次数
	if s.statsEnabled {               // 如果启用了统计功能
		s.lock.Lock()         // 获取写锁
		defer s.lock.Unlock() // 函数结束时释放写锁
		s.hashmapStats[key]++ // 增加该键的请求次数统计
	}
}

// hitWithoutLock 记录缓存命中事件（无锁版本）
// 参数:
//
//	key: 命中的键
func (s *cacheShard) hitWithoutLock(key uint64) {
	atomic.AddInt64(&s.stats.Hits, 1) // 原子增加命中次数
	if s.statsEnabled {               // 如果启用了统计功能
		s.hashmapStats[key]++ // 直接增加该键的请求次数统计
	}
}

// miss 记录缓存未命中事件
func (s *cacheShard) miss() {
	atomic.AddInt64(&s.stats.Misses, 1) // 原子增加未命中次数
}

// delhit 记录删除成功事件
func (s *cacheShard) delhit() {
	atomic.AddInt64(&s.stats.DelHits, 1) // 原子增加删除命中次数
}

// delmiss 记录删除失败事件
func (s *cacheShard) delmiss() {
	atomic.AddInt64(&s.stats.DelMisses, 1) // 原子增加删除未命中次数
}

// collision 记录哈希冲突事件
func (s *cacheShard) collision() {
	atomic.AddInt64(&s.stats.Collisions, 1) // 原子增加哈希冲突次数
}

// initNewShard 初始化并创建一个新的缓存分片
// 参数:
//
//	config: 缓存配置信息
//	callback: 条目被移除时的回调函数
//	clock: 时钟接口，用于获取时间
//
// 返回值: 指向新创建的 cacheShard 结构体的指针
func initNewShard(config Config, callback onRemoveCallback, clock clock.Clock) *cacheShard {
	bytesQueueInitialCapacity := config.initialShardSize() * config.MaxEntrySize            // 计算字节队列的初始容量
	maximumShardSizeInBytes := config.maximumShardSizeInBytes()                             // 获取分片的最大大小（字节）
	if maximumShardSizeInBytes > 0 && bytesQueueInitialCapacity > maximumShardSizeInBytes { // 如果设置了最大分片大小且初始容量超过最大大小
		bytesQueueInitialCapacity = maximumShardSizeInBytes // 将初始容量调整为最大分片大小
	}
	return &cacheShard{
		hashmap:      make(map[uint64]uint64, config.initialShardSize()),                                            // 创建哈希映射，初始大小为配置的分片大小
		hashmapStats: make(map[uint64]uint32, config.initialShardSize()),                                            // 创建哈希统计映射，初始大小为配置的分片大小
		entries:      *bytesqyeye.NewBytesQueue(bytesQueueInitialCapacity, maximumShardSizeInBytes, config.Verbose), // 创建字节队列
		entryBuffer:  make([]byte, config.MaxEntrySize+headersSizeInBytes),                                          // 创建条目缓冲区，大小为最大条目大小加上头部大小
		onRemove:     callback,                                                                                      // 设置条目移除回调函数

		isVerbose:    config.Verbose,                      // 设置详细日志标志
		logger:       config.Logger,                       // 设置日志记录器
		clock:        clock,                               // 设置时钟
		lifeWindow:   uint64(config.LifeWindow.Seconds()), // 设置条目生存时间窗口（转换为秒）
		statsEnabled: config.StatsEnabled,                 // 设置统计功能启用标志
		cleanEnabled: config.CleanWindow > 0,              // 设置自动清理功能启用标志（如果清理窗口大于0则启用）
	}
}
