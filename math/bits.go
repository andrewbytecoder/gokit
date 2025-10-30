package math

// Integer 定义了一个类型约束，表示所有整数类型
// 包括有符号整数: int, int8, int16, int32, int64
// 以及无符号整数: uint, uint8, uint16, uint32, uint64, uintptr
// ~int 的作用是底层类型是 int 也能兼容，比如 type MyInt int MyInt类型也包含在Integer中
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// IsPowerOfTwo 判断一个整数是否为2的幂次方
// 使用泛型支持所有整数类型
// 算法原理: 对于2的幂次方的数字，其二进制表示中只有一个位为1
// 例如: 8 (1000), 4 (0100), 2 (0010)
// n&(n-1) 操作会将最右边的1位清零，如果结果为0则说明原数只有一个1位
// 参数:
//
//	n: 待判断的整数
//
// 返回值:
//
//	bool: 如果n是2的幂次方且大于0则返回true，否则返回false
func IsPowerOfTwo[T Integer](n T) bool {
	return n > 0 && (n&(n-1)) == 0
}

//func IsPowerOfTwo32(n int) bool {
//	return n > 0 && (n&(n-1)) == 0
//}
//
//func IsPowerOfTwo64(n uint64) bool {
//	return n > 0 && (n&(n-1)) == 0
//}

// 性能略低
//func IsPowerOfTwo(n uint32) bool {
//	return bits.OnesCount(uint(n)) == 1
//}
