package bigcache

import (
	"bytes"
	"testing"
	"unsafe"
)

// BenchmarkBytesToString 测试 bytesToString 函数的性能
func BenchmarkBytesToString(b *testing.B) {
	// 创建测试数据
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesToString(data)
	}
}

// BenchmarkStringCast 测试标准 string() 转换的性能
func BenchmarkStringCast(b *testing.B) {
	// 创建测试数据
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = string(data)
	}
}

// BenchmarkBytesToStringSmall 测试小字节数组下 bytesToString 函数的性能
func BenchmarkBytesToStringSmall(b *testing.B) {
	// 创建小测试数据
	data := []byte("hello world")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesToString(data)
	}
}

// BenchmarkStringCastSmall 测试小字节数组下标准 string() 转换的性能
func BenchmarkStringCastSmall(b *testing.B) {
	// 创建小测试数据
	data := []byte("hello world")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = string(data)
	}
}

// BenchmarkBytesToStringLarge 测试大字节数组下 bytesToString 函数的性能
func BenchmarkBytesToStringLarge(b *testing.B) {
	// 创建大测试数据
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesToString(data)
	}
}

// BenchmarkStringCastLarge 测试大字节数组下标准 string() 转换的性能
func BenchmarkStringCastLarge(b *testing.B) {
	// 创建大测试数据
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = string(data)
	}
}

// VerifySameResult 验证两种转换方法结果是否一致
func TestConversionEquivalence(t *testing.T) {
	data := []byte("test data for conversion")

	result1 := bytesToString(data)
	result2 := string(data)

	if result1 != result2 {
		t.Errorf("Conversions produce different results: bytesToString=%s, string()=%s", result1, result2)
	}
}

// TestNoCopyBehavior 测试 bytesToString 不复制数据的行为
func TestNoCopyBehavior(t *testing.T) {
	data := []byte("test data")
	result := bytesToString(data)

	// 检查字符串和原始字节切片是否共享相同的底层数组
	if unsafe.StringData(result) != unsafe.SliceData(data) {
		t.Error("bytesToString should not copy data but appears to have copied")
	}
}

// TestStringCastCopies 测试 string() 转换确实复制了数据
func TestStringCastCopies(t *testing.T) {
	data := []byte("test data")
	result := string(data)

	// 检查字符串和原始字节切片是否使用不同的内存地址
	if unsafe.StringData(result) == unsafe.SliceData(data) {
		t.Error("string() should copy data but appears to share memory")
	}
}

// BenchmarkBytesBufferString 测试使用 bytes.Buffer 的方式
func BenchmarkBytesBufferString(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := bytes.NewBuffer(data)
		_ = buf.String()
	}
}
