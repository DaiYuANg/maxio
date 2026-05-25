package s3

import (
	"encoding/binary"
	"math/bits"
)

const (
	protocolMD5BlockSize = 64
	protocolMD5Size      = 16
)

var protocolMD5Shift = [64]uint32{
	7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
	5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
	4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
	6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
}

var protocolMD5Table = [64]uint32{
	0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee,
	0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
	0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be,
	0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
	0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa,
	0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
	0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed,
	0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
	0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c,
	0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
	0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05,
	0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
	0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039,
	0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
	0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1,
	0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391,
}

type protocolMD5 struct {
	state [4]uint32
	buf   [protocolMD5BlockSize]byte
	nx    int
	len   uint64
}

func newProtocolMD5() *protocolMD5 {
	digest := &protocolMD5{}
	digest.Reset()
	return digest
}

func protocolMD5Sum(data []byte) [protocolMD5Size]byte {
	digest := newProtocolMD5()
	if _, err := digest.Write(data); err != nil {
		return [protocolMD5Size]byte{}
	}
	return digest.Sum()
}

func (d *protocolMD5) Reset() {
	d.state = [4]uint32{0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476}
	d.nx = 0
	d.len = 0
}

func (d *protocolMD5) Write(data []byte) (int, error) {
	written := len(data)
	d.len += uint64(written)
	if d.nx > 0 {
		n := copy(d.buf[d.nx:], data)
		d.nx += n
		if d.nx == protocolMD5BlockSize {
			d.block(d.buf[:])
			d.nx = 0
		}
		data = data[n:]
	}
	for len(data) >= protocolMD5BlockSize {
		d.block(data[:protocolMD5BlockSize])
		data = data[protocolMD5BlockSize:]
	}
	if len(data) > 0 {
		d.nx = copy(d.buf[:], data)
	}
	return written, nil
}

func (d *protocolMD5) Sum() [protocolMD5Size]byte {
	copyDigest := *d
	copyDigest.finish()
	var output [protocolMD5Size]byte
	binary.LittleEndian.PutUint32(output[0:], copyDigest.state[0])
	binary.LittleEndian.PutUint32(output[4:], copyDigest.state[1])
	binary.LittleEndian.PutUint32(output[8:], copyDigest.state[2])
	binary.LittleEndian.PutUint32(output[12:], copyDigest.state[3])
	return output
}

func (d *protocolMD5) finish() {
	lengthBits := d.len << 3
	var padding [72]byte
	padding[0] = 0x80
	padLen := 56 - d.nx
	if padLen <= 0 {
		padLen += protocolMD5BlockSize
	}
	if _, err := d.Write(padding[:padLen]); err != nil {
		return
	}
	var lengthBytes [8]byte
	binary.LittleEndian.PutUint64(lengthBytes[:], lengthBits)
	if _, err := d.Write(lengthBytes[:]); err != nil {
		return
	}
}

func (d *protocolMD5) block(data []byte) {
	if len(data) < protocolMD5BlockSize {
		return
	}
	var words [16]uint32
	for index := range words {
		offset := index * 4
		words[index] = binary.LittleEndian.Uint32(data[offset:])
	}

	a, b, c, v := d.state[0], d.state[1], d.state[2], d.state[3]
	for round := range 64 {
		f := protocolMD5Round(round, b, c, v)
		word := protocolMD5Word(round, words)
		a, b, c, v = v, b+bits.RotateLeft32(a+f+protocolMD5Table[round]+word, int(protocolMD5Shift[round])), b, c
	}
	d.state[0] += a
	d.state[1] += b
	d.state[2] += c
	d.state[3] += v
}

func protocolMD5Round(round int, b, c, d uint32) uint32 {
	switch {
	case round < 16:
		return (b & c) | (^b & d)
	case round < 32:
		return (d & b) | (^d & c)
	case round < 48:
		return b ^ c ^ d
	default:
		return c ^ (b | ^d)
	}
}

func protocolMD5Word(round int, words [16]uint32) uint32 {
	index := protocolMD5WordIndex(round)
	if index < 8 {
		return protocolMD5WordLow(index, words)
	}
	return protocolMD5WordHigh(index, words)
}

func protocolMD5WordLow(index int, words [16]uint32) uint32 {
	switch index {
	case 0:
		return words[0]
	case 1:
		return words[1]
	case 2:
		return words[2]
	case 3:
		return words[3]
	case 4:
		return words[4]
	case 5:
		return words[5]
	case 6:
		return words[6]
	default:
		return words[7]
	}
}

func protocolMD5WordHigh(index int, words [16]uint32) uint32 {
	switch index {
	case 8:
		return words[8]
	case 9:
		return words[9]
	case 10:
		return words[10]
	case 11:
		return words[11]
	case 12:
		return words[12]
	case 13:
		return words[13]
	case 14:
		return words[14]
	default:
		return words[15]
	}
}

func protocolMD5WordIndex(round int) int {
	switch {
	case round < 16:
		return round
	case round < 32:
		return (5*round + 1) % 16
	case round < 48:
		return (3*round + 5) % 16
	default:
		return (7 * round) % 16
	}
}
