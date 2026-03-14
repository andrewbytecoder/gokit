package hash

import (
	"encoding/binary"
	"hash"
)

type Murmur uint32

func AppendUint32(b []byte, v uint32) []byte {
	return append(b,
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v),
	)
}
func (s *Murmur) Sum(b []byte) []byte {
	v := uint32(*s)
	return AppendUint32(b, v)
}

func (s *Murmur) Reset() {
	*s = 0
}

func (s *Murmur) Size() int {
	return 4
}

func (s *Murmur) BlockSize() int {
	return 1
}

// NewMurmur returns a new 32-bit FNV-1 [hash.Hash].
// Its Sum method will lay the value out in big-endian byte order.
func NewMurmur(seed uint32) hash.Hash32 {
	var s Murmur = Murmur(seed)
	return &s
}

func (s *Murmur) Write(data []byte) (int, error) {

	seed := uint32(*s)
	// Similar to murmur hash
	const (
		m = uint32(0xc6a4a793)
		r = uint32(24)
	)
	var (
		h = seed ^ (uint32(len(data)) * m)
		i int
	)

	for n := len(data) - len(data)%4; i < n; i += 4 {
		h += binary.LittleEndian.Uint32(data[i:])
		h *= m
		h ^= (h >> 16)
	}

	switch len(data) - i {
	default:
		panic("not reached")
	case 3:
		h += uint32(data[i+2]) << 16
		fallthrough
	case 2:
		h += uint32(data[i+1]) << 8
		fallthrough
	case 1:
		h += uint32(data[i])
		h *= m
		h ^= (h >> r)
	case 0:
	}
	*s = Murmur(h)
	return len(data), nil
}

// Sum32 计算给定数据的哈希值。
func (s *Murmur) Sum32() uint32 {
	return uint32(*s)
}
