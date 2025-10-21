package bytesqyeye

import (
	"encoding/binary"
	"errors"
)

// 使用 Bytes 模仿RingBuffer

const (
	// Number of bytes to encode 0 in uvarint format  bigcache [timestamp][hashcode][key length]
	minimumHeaderSize = 17 // 1 byte blobsize + timestampSizeInBytes + hashSizeInBytes
	// Bytes before left margin are not used. Zero index means element dos not exist in queue. useful while reading slice from index
	leftMarginIndex = 1
)

var (
	errEmptyQueue       = errors.New("queue is empty")
	errInvalidIndex     = errors.New("index must be greater than zero, Invalid index")
	errIndexOutOfBounds = errors.New("index out of bounds")
	errFullQueue        = errors.New("queue is full, Maximum size limit reached")
)

type queueError struct {
	message string
}

// BytesQueue is a non-thead safe queue type of fifo based on bytes array
// for every  push operation index of entry is returned. It can be used to read the entry later
type BytesQueue struct {
	full         bool   // flag to indicate if queue is full
	array        []byte // underlying byte array
	capacity     int    // capacity of queue
	maxCapacity  int    // maximum capacity of queue
	head         int    // index of first element in queue
	tail         int    // index of last element in queue
	count        int    // number of elements in queue
	rightMargin  int    // right margin index
	headerBuffer []byte // header buffer
	verbose      bool   // verbose mode
}

// getNeededSize returns the number of bytes an entry of length need in the queue
func getNeededSize(length int) int {
	// 实际需要的字节数为 header + length
	var header int
	switch {
	// 1 byte for header, length <= 126
	case length < 127: // 1 << 7 - 1
		header = 1
	// 2 byte for header, length <= 16381, 1 << 14 - 1 = 16383
	case length < 16382:
		header = 2
	// 3 byte for header, length <= 2097148, 1 << 21 - 1 = 2097151
	case length < 2097149:
		header = 3
	// 4 byte for header, length <= 268435451, 1 << 28 - 1 = 268435455
	case length < 268435452:
		header = 4
	default:
		header = 5
	}
	return header + length
}

// NewBytesQueue initializes new bytes queue.
// capacity is the used in bytes array allocated for queue.
// When verbose flag is set then information about memory allocation are printed to console
func NewBytesQueue(capacity int, maxCapacity int, verbose bool) *BytesQueue {
	return &BytesQueue{
		array:        make([]byte, capacity),              // 初始化一个字节数组
		capacity:     capacity,                            // 容量
		maxCapacity:  maxCapacity,                         // 最大容量
		headerBuffer: make([]byte, binary.MaxVarintLen32), // header buffer
		tail:         leftMarginIndex,                     // tail index
		head:         leftMarginIndex,                     // head index
		rightMargin:  leftMarginIndex,                     // right margin index
		verbose:      verbose,                             // verbose
	}
}

// Reset removes all entries from queue
func (q *BytesQueue) Reset() {
	// Just reset indexes
	q.tail = leftMarginIndex
	q.head = leftMarginIndex
	q.rightMargin = leftMarginIndex
	q.count = 0
	q.full = false
}

// Push copies entry at the end of queue and moves tail pointer. allocates more space if needed.
// Returns index for pushed data or error if maximum size queue limit is reached
func (q *BytesQueue) Push(entry []byte) (int, error) {
	neededSize := getNeededSize(len(entry))

	if !q.canInsertAfterTail(neededSize) {
		if q.canInsertBeforeHead(neededSize) {
			// 后端插入不了，但是 前端能插入，直接将tail 移动到leftMarginIndex，也就是移动到队列的最开始
			q.tail = leftMarginIndex
		} else if q.capacity+neededSize >= q.maxCapacity && q.maxCapacity > 0 {
			return -1, errFullQueue
		} else {
			// 扩容

		}
	}

	// 上次的tail就是这次的index指向位置
	index := q.tail
	q.push(entry, neededSize)

	return index, nil
}

// allocateAdditionalMemory 为BytesQueue 分配额外的内存，使其容量至少能容纳minimum字节。
// 该方法在当前容量不足时被调用 （例如Push 空间不够）
// 扩容策略： 至少翻倍，但是不超过maxCapacity （如果设置）
func (q *BytesQueue) allocateAdditionalMemory(minimum int) {}

func (q *BytesQueue) push(data []byte, len int) {
	headerEntrySize := binary.PutUvarint(q.headerBuffer, uint64(len))
	// 将headerEntrySize字节写入到 array中
	q.copy(q.headerBuffer, headerEntrySize)
	// 将data 写入到 array中
	q.copy(data, len-headerEntrySize)

	if q.tail > q.head {
		q.rightMargin = q.tail
	}
	if q.tail == q.head {
		q.full = true
	}

	q.count++
}

func (q *BytesQueue) copy(data []byte, len int) {
	q.tail += copy(q.array[q.tail:], data[:len])
}

// canInsertAfterTail returns true if it's possible to insert an entry of size of need after the tail of the queue
func (q *BytesQueue) canInsertAfterTail(need int) bool {
	if q.full {
		return false
	}
	if q.tail >= q.head {
		return q.capacity-q.tail >= need
	}
	// 必须保留至少 minimumHeaderSize 字节，用于在将来队列“刚好填满”时，能写入一个“空条目”或“结束标记”，以区分“空”和“满”状态
	return q.head-q.tail == need || q.head-q.tail >= need+minimumHeaderSize
}

// canInsertBeforeHead returns true if it's possible to insert an entry of size of need before the head of the queue
func (q *BytesQueue) canInsertBeforeHead(need int) bool {
	if q.full {
		return false
	}
	// 空闲区域在 [1, head) 和 [tail+1, capacity)
	if q.tail >= q.head {
		return q.head-leftMarginIndex == need || q.head-leftMarginIndex >= need+minimumHeaderSize
	}

	return q.head-q.tail == need || q.head-q.tail >= need+minimumHeaderSize
}
