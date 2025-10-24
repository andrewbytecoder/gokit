package bigcache

import (
	"encoding/binary"
	"testing"
)

func TestWrapEntry(t *testing.T) {
	// 准备测试数据
	timestamp := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry := []byte("testvalue")
	var buffer []byte

	// 调用 wrapEntry 函数
	result := wrapEntry(timestamp, hash, key, entry, &buffer)

	// 验证结果
	if len(result) != headersSizeInBytes+len(key)+len(entry) {
		t.Errorf("Expected length %d, got %d", headersSizeInBytes+len(key)+len(entry), len(result))
	}

	// 验证时间戳
	if readTimestampFromEntry(result) != timestamp {
		t.Error("Timestamp mismatch")
	}

	// 验证哈希值
	if readHashFromEntry(result) != hash {
		t.Error("Hash mismatch")
	}

	// 验证键
	if readKeyFromEntry(result) != key {
		t.Error("Key mismatch")
	}

	// 验证值的长度（readEntry 只是计算长度，这里验证长度计算是否正确）
	value := readEntry(result)
	expectedValueLen := len(entry)
	if len(value) != expectedValueLen {
		t.Errorf("Expected value length %d, got %d", expectedValueLen, len(value))
	}
}

func TestAppendToWrappedEntry(t *testing.T) {
	// 准先准备初始数据
	timestamp1 := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry1 := []byte("testvalue1")
	var buffer1 []byte

	// 创建初始包装条目
	wrappedEntry := wrapEntry(timestamp1, hash, key, entry1, &buffer1)

	// 准备追加数据
	timestamp2 := uint64(1234567891)
	entry2 := []byte("testvalue2")
	var buffer2 []byte

	// 调用 appendToWrappedEntry 函数
	result := appendToWrappedEntry(timestamp2, wrappedEntry, entry2, &buffer2)

	// 验证结果
	expectedLen := len(wrappedEntry) + len(entry2)
	if len(result) != expectedLen {
		t.Errorf("Expected length %d, got %d", expectedLen, len(result))
	}

	// 验证新时间戳
	if readTimestampFromEntry(result) != timestamp2 {
		t.Error("New timestamp mismatch")
	}

	// 验证其他数据是否保留（检查哈希值）
	if binary.LittleEndian.Uint64(result[timestampSizeInBytes:]) != hash {
		t.Error("Hash not preserved correctly")
	}
}

func TestReadEntry(t *testing.T) {
	// 准备测试数据
	timestamp := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry := []byte("testvalue")
	var buffer []byte

	// 创建包装条目
	wrappedEntry := wrapEntry(timestamp, hash, key, entry, &buffer)

	// 调用 readEntry 函数
	result := readEntry(wrappedEntry)

	// 验证结果长度（虽然 readEntry 函数本身未复制值数据，但长度应该正确）
	if len(result) != len(entry) {
		t.Errorf("Expected value length %d, got %d", len(entry), len(result))
	}
}

func TestReadTimestampFromEntry(t *testing.T) {
	// 准备测试数据
	timestamp := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry := []byte("testvalue")
	var buffer []byte

	// 创建包装条目
	wrappedEntry := wrapEntry(timestamp, hash, key, entry, &buffer)

	// 调用 readTimestampFromEntry 函数
	result := readTimestampFromEntry(wrappedEntry)

	// 验证结果
	if result != timestamp {
		t.Errorf("Expected timestamp %d, got %d", timestamp, result)
	}
}

func TestReadKeyFromEntry(t *testing.T) {
	// 准备测试数据
	timestamp := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry := []byte("testvalue")
	var buffer []byte

	// 创建包装条目
	wrappedEntry := wrapEntry(timestamp, hash, key, entry, &buffer)

	// 调用 readKeyFromEntry 函数
	result := readKeyFromEntry(wrappedEntry)

	// 验证结果
	if result != key {
		t.Errorf("Expected key %s, got %s", key, result)
	}
}

func TestCompareKeyFromEntry(t *testing.T) {
	// 准备测试数据
	timestamp := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry := []byte("testvalue")
	var buffer []byte

	// 创建包装条目
	wrappedEntry := wrapEntry(timestamp, hash, key, entry, &buffer)

	// 测试匹配的键
	if !compareKeyFromEntry(wrappedEntry, key) {
		t.Error("Expected key comparison to return true for matching key")
	}

	// 测试不匹配的键
	if compareKeyFromEntry(wrappedEntry, "wrongkey") {
		t.Error("Expected key comparison to return false for non-matching key")
	}
}

func TestReadHashFromEntry(t *testing.T) {
	// 准备测试数据
	timestamp := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry := []byte("testvalue")
	var buffer []byte

	// 创建包装条目
	wrappedEntry := wrapEntry(timestamp, hash, key, entry, &buffer)

	// 调用 readHashFromEntry 函数
	result := readHashFromEntry(wrappedEntry)

	// 验证结果
	if result != hash {
		t.Errorf("Expected hash %d, got %d", hash, result)
	}
}

func TestResetHashFromEntry(t *testing.T) {
	// 准备测试数据
	timestamp := uint64(1234567890)
	hash := uint64(9876543210)
	key := "testkey"
	entry := []byte("testvalue")
	var buffer []byte

	// 创建包装条目
	wrappedEntry := wrapEntry(timestamp, hash, key, entry, &buffer)

	// 验证初始哈希值
	if readHashFromEntry(wrappedEntry) != hash {
		t.Error("Initial hash mismatch")
	}

	// 调用 resetHashFromEntry 函数
	resetHashFromEntry(wrappedEntry)

	// 验证哈希值是否被重置为0
	if readHashFromEntry(wrappedEntry) != 0 {
		t.Error("Hash was not reset to 0")
	}
}
