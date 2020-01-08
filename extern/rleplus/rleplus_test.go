package rleplus_test

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/filecoin-project/lotus/extern/rleplus"
	bitvector "github.com/filecoin-project/lotus/extern/rleplus/internal"
	"gotest.tools/assert"
)

func TestRleplus(t *testing.T) {

	t.Run("Encode", func(t *testing.T) {
		// Encode an intset
		ints := []uint64{
			// run of 1
			0,
			// gap of 1
			// run of 1
			2,
			// gap of 1
			// run of 3
			4, 5, 6,
			// gap of 4
			// run of 17
			11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27,
		}

		expectedBits := []byte{
			0, 0, // version
			1,                // first bit
			1,                // run of 1
			1,                // gap of 1
			1,                // run of 1
			1,                // gap of 1
			0, 1, 1, 1, 0, 0, // run of 3
			0, 1, 0, 0, 1, 0, // gap of 4

			// run of 17 < 0 0 (varint) >
			0, 0,
			1, 0, 0, 0, 1, 0, 0, 0,
		}

		v := bitvector.BitVector{}
		for _, bit := range expectedBits {
			v.Push(bit)
		}
		actualBytes, _, err := rleplus.Encode(ints)
		assert.NilError(t, err)

		assert.Equal(t, len(v.Buf), len(actualBytes))
		for idx, expected := range v.Buf {
			assert.Equal(
				t,
				fmt.Sprintf("%08b", expected),
				fmt.Sprintf("%08b", actualBytes[idx]),
			)
		}
	})

	t.Run("Encode allows all runs sizes possible uint64", func(t *testing.T) {
		// create a run of math.MaxUint64
		ints := []uint64{math.MaxUint64}

		// There would be 64 bits(1) for the UvarInt, totally 9 bytes.
		expected := []byte{0xe0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x3f, 0x20}
		encodeBytes, _, err := rleplus.Encode(ints)
		assert.NilError(t, err)
		for idx, v := range encodeBytes {
			assert.Equal(
				t,
				fmt.Sprintf("%8b", v),
				fmt.Sprintf("%8b", expected[idx]),
			)
		}
	})

	t.Run("Encode for some big numbers", func(t *testing.T) {
		// create a run of math.MaxUint64
		ints := make([]uint64, 1024)

		// ints {2^63 .. 2^63+1023}
		for i := uint64(0); i < 1024; i++ {
			ints[i] = uint64(1)<<63 + i
		}

		expected := []byte{0x00, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x30, 0x00, 0x40, 0x04}
		encodeBytes, _, err := rleplus.Encode(ints)
		assert.NilError(t, err)
		for idx, v := range encodeBytes {
			// fmt.Println(v, expected[idx])
			assert.Equal(
				t,
				fmt.Sprintf("%8b", v),
				fmt.Sprintf("%8b", expected[idx]),
			)
		}
	})

	t.Run("Decode", func(t *testing.T) {
		testCases := [][]uint64{
			{},
			{1},
			{0},
			{0, 1, 2, 3},
			{
				// run of 1
				0,
				// gap of 1
				// run of 1
				2,
				// gap of 1
				// run of 3
				4, 5, 6,
				// gap of 4
				// run of 17
				11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27,
			},
		}

		for _, tc := range testCases {
			encoded, _, err := rleplus.Encode(tc)
			assert.NilError(t, err)

			result, err := rleplus.Decode(encoded)
			assert.NilError(t, err)

			sort.Slice(tc, func(i, j int) bool { return tc[i] < tc[j] })
			sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })

			assert.Equal(t, len(tc), len(result))

			for idx, expected := range tc {
				assert.Equal(t, expected, result[idx])
			}
		}
	})

	t.Run("Decode version check", func(t *testing.T) {
		_, err := rleplus.Decode([]byte{0xff})
		assert.Error(t, err, "invalid RLE+ version")
	})

	t.Run("Decode returns an error with a bad encoding", func(t *testing.T) {
		// create an encoding with a buffer with a run which is too long
		_, err := rleplus.Decode([]byte{0xe0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
		assert.Error(t, err, "invalid encoding for RLE+ version 0")
	})

	t.Run("outputs same as reference implementation", func(t *testing.T) {
		// Encoding bitvec![LittleEndian; 1, 0, 1, 0, 1, 1, 1, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
		// in the Rust reference implementation gives an encoding of [223, 145, 136, 0] (without version field)
		// The bit vector is equivalent to the integer set { 0, 2, 4, 5, 6, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27 }

		// This is the above reference output with a version header "00" manually added
		referenceEncoding := []byte{124, 71, 34, 2}

		expectedNumbers := []uint64{0, 2, 4, 5, 6, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}

		encoded, _, err := rleplus.Encode(expectedNumbers)
		assert.NilError(t, err)

		// Our encoded bytes are the same as the ref bytes
		assert.Equal(t, len(referenceEncoding), len(encoded))
		for idx, expected := range referenceEncoding {
			assert.Equal(t, expected, encoded[idx])
		}

		decoded, err := rleplus.Decode(referenceEncoding)
		assert.NilError(t, err)

		// Our decoded integers are the same as expected
		sort.Slice(decoded, func(i, j int) bool { return decoded[i] < decoded[j] })
		assert.Equal(t, len(expectedNumbers), len(decoded))
		for idx, expected := range expectedNumbers {
			assert.Equal(t, expected, decoded[idx])
		}
	})

	t.Run("RunLengths", func(t *testing.T) {
		testCases := []struct {
			ints  []uint64
			first byte
			runs  []uint64
		}{
			// empty
			{},

			// leading with ones
			{[]uint64{0}, 1, []uint64{1}},
			{[]uint64{0, 1}, 1, []uint64{2}},
			{[]uint64{0, 0xffffffff, 0xffffffff + 1}, 1, []uint64{1, 0xffffffff - 1, 2}},

			// leading with zeroes
			{[]uint64{1}, 0, []uint64{1, 1}},
			{[]uint64{2}, 0, []uint64{2, 1}},
			{[]uint64{10, 11, 13, 20}, 0, []uint64{10, 2, 1, 1, 6, 1}},
			{[]uint64{10, 11, 11, 13, 20, 10, 11, 13, 20}, 0, []uint64{10, 2, 1, 1, 6, 1}},
		}

		for _, testCase := range testCases {
			first, runs := rleplus.RunLengths(testCase.ints)
			assert.Equal(t, testCase.first, first)
			assert.Equal(t, len(testCase.runs), len(runs))
			for idx, runLength := range testCase.runs {
				assert.Equal(t, runLength, runs[idx])
			}
		}
	})
}
