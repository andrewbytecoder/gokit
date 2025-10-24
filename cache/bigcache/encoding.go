package bigcache

import "encoding/binary"

// 定义各种头部信息在条目中的字节大小
const (
	timestampSizeInBytes = 8                                                       // 时间戳占用的字节数
	hashSizeInBytes      = 8                                                       // 哈希值占用的字节数
	keySizeInBytes       = 2                                                       // 键长度信息占用的字节数
	headersSizeInBytes   = timestampSizeInBytes + hashSizeInBytes + keySizeInBytes // 所有头部信息总共占用的字节数
)

// wrapEntry 将时间戳、哈希值、键和值打包成一个字节切片
// 参数:
//
//	timestamp: 条目的时间戳
//	hash: 键的哈希值
//	key: 条目的键
//	entry: 条目的值
//	buffer: 用于存储打包数据的缓冲区
//
// 返回值: 包含所有信息的字节切片
func wrapEntry(timestamp uint64, hash uint64, key string, entry []byte, buffer *[]byte) []byte {
	keyLength := len(key)                                     // 获取键的长度
	blobLength := len(entry) + headersSizeInBytes + keyLength // 计算整个条目需要的总字节数

	if blobLength > len(*buffer) { // 如果缓冲区不够大
		*buffer = make([]byte, blobLength) // 重新分配足够大的缓冲区
	}

	blob := *buffer // 获取缓冲区引用

	binary.LittleEndian.PutUint64(blob, timestamp)                                                // 在缓冲区开头写入时间戳(8字节)
	binary.LittleEndian.PutUint64(blob[timestampSizeInBytes:], hash)                              // 在时间戳后写入哈希值(8字节)
	binary.LittleEndian.PutUint16(blob[timestampSizeInBytes+hashSizeInBytes:], uint16(keyLength)) // 在哈希值后写入键长度(2字节)
	copy(blob[headersSizeInBytes:], key)                                                          // 在头部信息后写入键内容
	copy(blob[headersSizeInBytes+keyLength:], entry)                                              // 在键内容后写入值内容
	return blob[:blobLength]                                                                      // 返回完整的条目数据
}

// appendToWrappedEntry 将新的条目数据追加到已包装的条目后面
// 参数:
//
//	timestamp: 新的时间戳
//	wrappedEntry: 已经包装好的条目
//	entry: 要追加的数据
//	buffer: 用于存储结果的缓冲区
//
// 返回值: 包含新时间戳和追加数据的字节切片
func appendToWrappedEntry(timestamp uint64, wrappedEntry []byte, entry []byte, buffer *[]byte) []byte {
	blobLength := len(wrappedEntry) + len(entry) // 计算新条目需要的总字节数
	if blobLength > len(*buffer) {               // 如果缓冲区不够大
		*buffer = make([]byte, blobLength) // 重新分配足够大的缓冲区
	}

	blob := *buffer // 获取缓冲区引用

	binary.LittleEndian.PutUint64(blob, timestamp)                         // 在缓冲区开头写入新的时间戳
	copy(blob[timestampSizeInBytes:], wrappedEntry[timestampSizeInBytes:]) // 复制原条目中除时间戳外的所有数据
	copy(blob[len(wrappedEntry):], entry)                                  // 在原条目后追加新数据

	return blob[:blobLength] // 返回完整的条目数据
}

// readEntry 从包装的条目中读取值数据
// 参数:
//
//	data: 包含完整条目信息的字节切片
//
// 返回值: 条目中的值数据
func readEntry(data []byte) []byte {
	// timestamp + hash + key length + key + value
	length := binary.LittleEndian.Uint16(data[timestampSizeInBytes+hashSizeInBytes:]) // 读取键长度(2字节)
	// 去除 timestamp hash key-length + key 之后就是value了
	dst := make([]byte, len(data)-int(headersSizeInBytes+length)) // 计算并分配值数据所需的空间
	copy(dst, data[headersSizeInBytes+length:])
	
	return dst // 返回值数据(注意:此处未实际复制值数据)
}

// readTimestampFromEntry 从包装的条目中读取时间戳
// 参数:
//
//	data: 包含完整条目信息的字节切片
//
// 返回值: 条目中的时间戳
func readTimestampFromEntry(data []byte) uint64 {
	// timestamp + hash + key length + key + value
	return binary.LittleEndian.Uint64(data) // 读取前8个字节作为时间戳
}

// readKeyFromEntry 从包装的条目中读取键
// 参数:
//
//	data: 包含完整条目信息的字节切片
//
// 返回值: 条目中的键字符串
func readKeyFromEntry(data []byte) string {
	// timestamp + hash + key length + key + value
	length := binary.LittleEndian.Uint16(data[timestampSizeInBytes+hashSizeInBytes:]) // 读取键长度(2字节)

	// copy on read
	dst := make([]byte, length)                                   // 分配存储键数据的空间
	copy(dst, data[headersSizeInBytes:headersSizeInBytes+length]) // 从条目中复制键数据

	return bytesToString(dst) // 将字节切片转换为字符串并返回
}

// compareKeyFromEntry 比较条目中的键与给定的键是否相等
// 参数:
//
//	data: 包含完整条目信息的字节切片
//	key: 要比较的键字符串
//
// 返回值: 如果条目中的键与给定键相等则返回true，否则返回false
func compareKeyFromEntry(data []byte, key string) bool {
	// timestamp + hash + key length + key + value
	length := binary.LittleEndian.Uint16(data[timestampSizeInBytes+hashSizeInBytes:]) // 读取键长度(2字节)

	return bytesToString(data[headersSizeInBytes:headersSizeInBytes+length]) == key // 将条目中的键与给定键进行比较
}

// readHashFromEntry 从包装的条目中读取哈希值
// 参数:
//
//	data: 包含完整条目信息的字节切片
//
// 返回值: 条目中的哈希值
func readHashFromEntry(data []byte) uint64 {
	// timestamp + hash + key length + key + value
	return binary.LittleEndian.Uint64(data[timestampSizeInBytes:]) // 读取时间戳后的8个字节作为哈希值
}

// resetHashFromEntry 将条目中的哈希值重置为0
// 参数:
//
//	data: 包含完整条目信息的字节切片
func resetHashFromEntry(data []byte) {
	// timestamp + hash + key length + key + value
	binary.LittleEndian.PutUint64(data[timestampSizeInBytes:], 0) // 将哈希值位置的数据设置为0
}
