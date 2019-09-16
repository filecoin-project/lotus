package bitvector

import (
	"errors"
	"log"
)

var (
	// ErrOutOfRange - the index passed is out of range for the BitVector
	ErrOutOfRange = errors.New("index out of range")
)

// BitNumbering indicates the ordering of bits, either
// least-significant bit in position 0, or most-significant bit
// in position 0.
//
// It it used in 3 ways with BitVector:
// 1. Ordering of bits within the Buf []byte structure
// 2. What order to add bits when using Extend()
// 3. What order to read bits when using Take()
//
// https://en.wikipedia.org/wiki/Bit_numbering
type BitNumbering int

const (
	// LSB0 - bit ordering starts with the low-order bit
	LSB0 BitNumbering = iota

	// MSB0 - bit ordering starts with the high-order bit
	MSB0
)

// BitVector is used to manipulate ordered collections of bits
type BitVector struct {
	Buf []byte

	// BytePacking is the bit ordering within bytes
	BytePacking BitNumbering

	// Len is the logical number of bits in the vector.
	// The last byte in Buf may have undefined bits if Len is not a multiple of 8
	Len uint
}

// NewBitVector constructs a new BitVector from a slice of bytes.
//
// The bytePacking parameter is required to know how to interpret the bit ordering within the bytes.
func NewBitVector(buf []byte, bytePacking BitNumbering) *BitVector {
	return &BitVector{
		BytePacking: bytePacking,
		Buf:         buf,
		Len:         uint(len(buf) * 8),
	}
}

// Push adds a single bit to the BitVector.
//
// Although it takes a byte, only the low-order bit is used, so just use 0 or 1.
func (v *BitVector) Push(val byte) {
	if v.Len%8 == 0 {
		v.Buf = append(v.Buf, 0)
	}
	lastIdx := v.Len / 8

	switch v.BytePacking {
	case LSB0:
		v.Buf[lastIdx] |= (val & 1) << (v.Len % 8)
	default:
		v.Buf[lastIdx] |= (val & 1) << (7 - (v.Len % 8))
	}

	v.Len++
}

// Get returns a single bit as a byte -- either 0 or 1
func (v *BitVector) Get(idx uint) (byte, error) {
	if idx >= v.Len {
		return 0, ErrOutOfRange
	}
	blockIdx := idx / 8

	switch v.BytePacking {
	case LSB0:
		return v.Buf[blockIdx] >> (idx % 8) & 1, nil
	default:
		return v.Buf[blockIdx] >> (7 - idx%8) & 1, nil
	}
}

// Extend adds up to 8 bits to the receiver
//
// Given a byte b == 0b11010101
// v.Extend(b, 4, LSB0) would add < 1, 0, 1, 0 >
// v.Extend(b, 4, MSB0) would add < 1, 1, 0, 1 >
//
// Panics if count is out of range
func (v *BitVector) Extend(val byte, count uint, order BitNumbering) {
	if count > 8 {
		log.Panicf("invalid count")
	}

	for i := uint(0); i < count; i++ {
		switch order {
		case LSB0:
			v.Push((val >> i) & 1)
		default:
			v.Push((val >> (7 - i)) & 1)
		}
	}
}

// Take reads up to 8 bits at the given index.
//
// Given a BitVector < 1, 1, 0, 1, 0, 1, 0, 1 >
// v.Take(0, 4, LSB0) would return 0b00001011
// v.Take(0, 4, MSB0) would return 0b11010000
//
// Panics if count is out of range
func (v *BitVector) Take(index uint, count uint, order BitNumbering) (out byte) {
	if count > 8 {
		log.Panicf("invalid count")
	}

	for i := uint(0); i < count; i++ {
		val, _ := v.Get(index + i)

		switch order {
		case LSB0:
			out |= val << i
		default:
			out |= val << (7 - i)
		}
	}
	return
}

// Iterator returns a function, which when invoked, returns the number
// of bits requested, and increments an internal cursor.
//
// When the end of the BitVector is reached, it returns zeroes indefinitely
//
// Panics if count is out of range
func (v *BitVector) Iterator(order BitNumbering) func(uint) byte {
	cursor := uint(0)
	return func(count uint) (out byte) {
		if count > 8 {
			log.Panicf("invalid count")
		}

		out = v.Take(cursor, count, order)
		cursor += count
		return
	}
}
