package rlepluslazy

import (
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-lotus/extern/rleplus"
	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {
	// Encoding bitvec![LittleEndian; 1, 0, 1, 0, 1, 1, 1, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1]
	// in the Rust reference implementation gives an encoding of [223, 145, 136, 0] (without version field)
	// The bit vector is equivalent to the integer set { 0, 2, 4, 5, 6, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27 }

	// This is the above reference output with a version header "00" manually added
	referenceEncoding := []byte{124, 71, 34, 2}

	expectedNumbers := []uint64{0, 2, 4, 5, 6, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}

	runs, err := RunsFromBits(BitsFromSlice(expectedNumbers))
	assert.NoError(t, err)
	encoded, err := EncodeRuns(runs, []byte{})
	assert.NoError(t, err)

	// Our encoded bytes are the same as the ref bytes
	assert.Equal(t, len(referenceEncoding), len(encoded))
	assert.Equal(t, referenceEncoding, encoded)

	rle, err := FromBuf(encoded)
	assert.NoError(t, err)
	decoded := make([]uint64, 0, len(expectedNumbers))

	rit, err := rle.RunIterator()
	assert.NoError(t, err)

	it, err := BitsFromRuns(rit)
	assert.NoError(t, err)
	for it.HasNext() {
		bit, err := it.Next()
		assert.NoError(t, err)
		decoded = append(decoded, bit)
	}

	// Our decoded integers are the same as expected
	assert.Equal(t, expectedNumbers, decoded)
}

func TestGoldenGen(t *testing.T) {
	t.SkipNow()
	N := 10000
	mod := uint32(1) << 20
	runExProp := float32(0.93)

	bits := make([]uint64, N)

	for i := 0; i < N; i++ {
		x := rand.Uint32() % mod
		bits[i] = uint64(x)
		for rand.Float32() < runExProp && i+1 < N {
			i++
			x = (x + 1) % mod
			bits[i] = uint64(x)
		}
	}

	out, _, err := rleplus.Encode(bits)
	assert.NoError(t, err)
	t.Logf("%#v", out)
	_, runs := rleplus.RunLengths(bits)
	t.Logf("runs: %v", runs)
	t.Logf("len: %d", len(out))
}

func TestGolden(t *testing.T) {
	expected, _ := rleplus.Decode(goldenRLE)
	res := make([]uint64, 0, len(expected))

	rle, err := FromBuf(goldenRLE)
	assert.NoError(t, err)
	rit, err := rle.RunIterator()
	assert.NoError(t, err)
	it, err := BitsFromRuns(rit)
	assert.NoError(t, err)
	for it.HasNext() {
		bit, err := it.Next()
		assert.NoError(t, err)
		res = append(res, bit)
	}
	assert.Equal(t, expected, res)
}

func TestGoldenLoop(t *testing.T) {
	rle, err := FromBuf(goldenRLE)
	assert.NoError(t, err)

	rit, err := rle.RunIterator()
	assert.NoError(t, err)

	buf, err := EncodeRuns(rit, nil)
	assert.NoError(t, err)

	assert.Equal(t, goldenRLE, buf)
}

var Res uint64 = 0

func BenchmarkRunIterator(b *testing.B) {
	b.ReportAllocs()
	var r uint64
	for i := 0; i < b.N; i++ {
		rle, _ := FromBuf(goldenRLE)
		rit, _ := rle.RunIterator()
		for rit.HasNext() {
			run, _ := rit.NextRun()
			if run.Val {
				r = r + run.Len
			}
		}
	}
	Res = Res + r
}

func BenchmarkRunsToBits(b *testing.B) {
	b.ReportAllocs()
	var r uint64
	for i := 0; i < b.N; i++ {
		rle, _ := FromBuf(goldenRLE)
		rit, _ := rle.RunIterator()
		it, _ := BitsFromRuns(rit)
		for it.HasNext() {
			bit, _ := it.Next()
			if bit < 1<<63 {
				r++
			}
		}
	}
	Res = Res + r
}

func BenchmarkOldRLE(b *testing.B) {
	b.ReportAllocs()
	var r uint64
	for i := 0; i < b.N; i++ {
		rle, _ := rleplus.Decode(goldenRLE)
		r = r + uint64(len(rle))
	}
	Res = Res + r
}

func BenchmarkDecodeEncode(b *testing.B) {
	b.ReportAllocs()
	var r uint64
	/*
		out := make([]byte, 0, len(goldenRLE))
		for i := 0; i < b.N; i++ {
			rle, _ := FromBuf(goldenRLE)
			rit, _ := rle.RunIterator()
			out, _ = EncodeRuns(rit, out)
			r = r + uint64(len(out))
		}
	*/

	for i := 0; i < b.N; i++ {
		rle, _ := rleplus.Decode(goldenRLE)
		out, _, _ := rleplus.Encode(rle)
		r = r + uint64(len(out))
	}
	Res = Res + r
}
