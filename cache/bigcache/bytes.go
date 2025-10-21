//go:build !appengine

package bigcache

import (
	"unsafe"
)

// 行为：直接将 []byte 的底层指针“强转”为字符串，不复制数据
// 内存布局利用：依赖 Go 中 string 和 []byte 的内部结构相似性：

/*
	    slice 能够强制转化为 string， 但是string不能强制沾化为slice
		type stringStruct struct {
		    str unsafe.Pointer
		    len int
		}

		type slice struct {
		    array unsafe.Pointer
		    len   int
		    cap   int
		}
*/

func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
