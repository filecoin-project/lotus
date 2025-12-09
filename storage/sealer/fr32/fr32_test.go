package fr32_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fr32"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
)

func padFFI(buf []byte) []byte {
	rf, w, _ := mock.ToReadableFile(bytes.NewReader(buf), int64(len(buf)))
	tf, _ := os.CreateTemp("/tmp/", "scrb-")

	_, _, _, err := ffi.WriteWithAlignment(abi.RegisteredSealProof_StackedDrg32GiBV1, rf, abi.UnpaddedPieceSize(len(buf)), tf, nil)
	if err != nil {
		panic(err)
	}
	if err := w(); err != nil {
		panic(err)
	}

	if _, err := tf.Seek(io.SeekStart, 0); err != nil { // nolint:staticcheck
		panic(err)
	}

	padded, err := io.ReadAll(tf)
	if err != nil {
		panic(err)
	}

	if err := tf.Close(); err != nil {
		panic(err)
	}

	if err := os.Remove(tf.Name()); err != nil {
		panic(err)
	}

	return padded
}

func TestPadChunkFFI(t *testing.T) {
	testByteChunk := func(b byte) func(*testing.T) {
		return func(t *testing.T) {
			var buf [128]byte
			copy(buf[:], bytes.Repeat([]byte{b}, 127))

			fr32.Pad(buf[:], buf[:])

			expect := padFFI(bytes.Repeat([]byte{b}, 127))

			require.Equal(t, expect, buf[:])
		}
	}

	t.Run("ones", testByteChunk(0xff))
	t.Run("lsb1", testByteChunk(0x01))
	t.Run("msb1", testByteChunk(0x80))
	t.Run("zero", testByteChunk(0x0))
	t.Run("mid", testByteChunk(0x3c))
}

func TestPadChunkRandEqFFI(t *testing.T) {

	for i := 0; i < 200; i++ {
		var input [127]byte
		_, err := rand.Read(input[:])
		if err != nil {
			panic(err)
		}
		var buf [128]byte

		fr32.Pad(input[:], buf[:])

		expect := padFFI(input[:])

		require.Equal(t, expect, buf[:])
	}
}

func TestRoundtrip(t *testing.T) {
	testByteChunk := func(b byte) func(*testing.T) {
		return func(t *testing.T) {
			var buf [128]byte
			input := bytes.Repeat([]byte{0x01}, 127)

			fr32.Pad(input, buf[:])

			var out [127]byte
			fr32.Unpad(buf[:], out[:])

			require.Equal(t, input, out[:])
		}
	}

	t.Run("ones", testByteChunk(0xff))
	t.Run("lsb1", testByteChunk(0x01))
	t.Run("msb1", testByteChunk(0x80))
	t.Run("zero", testByteChunk(0x0))
	t.Run("mid", testByteChunk(0x3c))
}

func TestRoundtripChunkRand(t *testing.T) {
	for i := 0; i < 200; i++ {
		var input [127]byte
		_, err := rand.Read(input[:])
		if err != nil {
			panic(err)
		}
		var buf [128]byte
		copy(buf[:], input[:])

		fr32.Pad(buf[:], buf[:])

		var out [127]byte
		fr32.Unpad(buf[:], out[:])

		require.Equal(t, input[:], out[:])
	}
}

func TestRoundtrip16MRand(t *testing.T) {
	up := abi.PaddedPieceSize(16 << 20).Unpadded()

	input := make([]byte, up)
	_, err := rand.Read(input[:])
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 16<<20)

	fr32.Pad(input, buf)

	out := make([]byte, up)
	fr32.Unpad(buf, out)

	require.Equal(t, input, out)

	ffi := padFFI(input)
	require.Equal(t, ffi, buf)
}

// TestRoundtripMisalignedSizes tests the multithreaded Pad/Unpad with sizes that
// previously caused data corruption due to thread boundary misalignment.
// The bug occurred when (padLen / threads) was not a multiple of 128 bytes,
// causing partial chunks at thread boundaries to be skipped.
func TestRoundtripMisalignedSizes(t *testing.T) {
	// These sizes are chosen to trigger the multithreaded path (> 512KB)
	// and create thread boundaries that don't align to 128-byte chunks.
	testCases := []struct {
		name      string
		numChunks int
	}{
		// 66061 chunks = 8455808 padded bytes
		// With 16 threads: 8455808/16 = 528488 bytes per thread
		// 528488/128 = 4128.5 - NOT aligned! This was the original bug case.
		{"66061_chunks_8MiB_boundary", 66061},

		// Various sizes that create misaligned thread boundaries
		{"prime_chunks_1009", 1009 * 8},  // ~1MB, prime-ish number of chunks
		{"odd_chunks_8193", 8193},        // Just over 8192 (power of 2)
		{"odd_chunks_65537", 65537},      // Just over 65536 (power of 2)
		{"odd_chunks_100003", 100003},    // Large prime
		{"boundary_chunks_66000", 66000}, // Near the original bug size
		{"boundary_chunks_70000", 70000}, // Larger odd size
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			unpaddedSize := tc.numChunks * 127
			paddedSize := tc.numChunks * 128

			// Skip if too large for this test
			if paddedSize > 64<<20 {
				t.Skip("Size too large for this test")
			}

			input := make([]byte, unpaddedSize)
			_, err := rand.Read(input)
			require.NoError(t, err)

			padded := make([]byte, paddedSize)
			fr32.Pad(input, padded)

			output := make([]byte, unpaddedSize)
			fr32.Unpad(padded, output)

			require.Equal(t, input, output, "Roundtrip failed for %d chunks", tc.numChunks)
		})
	}
}

// TestUnpadMisalignedThreadBoundaries specifically tests the fix for the
// multithreaded Unpad bug where thread boundaries weren't aligned to
// 128-byte fr32 chunks, causing data loss.
func TestUnpadMisalignedThreadBoundaries(t *testing.T) {
	// Create data that's just over 8MiB to trigger the original bug
	// 66061 chunks * 127 bytes = 8389747 unpadded bytes
	// 66061 chunks * 128 bytes = 8455808 padded bytes
	numChunks := 66061
	unpaddedSize := numChunks * 127
	paddedSize := numChunks * 128

	// Create sequential data so we can detect exactly where corruption occurs
	input := make([]byte, unpaddedSize)
	for i := range input {
		input[i] = byte(i & 0xFF)
	}

	padded := make([]byte, paddedSize)
	fr32.Pad(input, padded)

	output := make([]byte, unpaddedSize)
	fr32.Unpad(padded, output)

	// Check for corruption at thread boundaries
	// With the original bug, corruption occurred at offsets like:
	// 528384 (thread 0/1 boundary), 1056768 (thread 1/2 boundary), etc.

	// First verify total length
	require.Equal(t, len(input), len(output), "Output length mismatch")

	// Check every byte
	for i := 0; i < len(input); i++ {
		if input[i] != output[i] {
			// Find the extent of the corruption
			corruptStart := i
			corruptEnd := i
			for corruptEnd < len(input) && input[corruptEnd] != output[corruptEnd] {
				corruptEnd++
			}
			t.Fatalf("Data corruption at offset %d (0x%x) to %d (0x%x): expected 0x%02x, got 0x%02x (corrupt bytes: %d)",
				corruptStart, corruptStart, corruptEnd, corruptEnd,
				input[i], output[i], corruptEnd-corruptStart)
		}
	}
}

// TestPadUnpadVariousSizesAboveMTTresh tests Pad/Unpad roundtrip for various
// sizes above the MTTresh (512KB) threshold that triggers multithreading.
func TestPadUnpadVariousSizesAboveMTTresh(t *testing.T) {
	// Test sizes from just above MTTresh to several MB
	// These should all use the multithreaded path
	sizes := []int{
		513 * 1024 / 127 * 127,        // Just above 512KB, aligned to chunks
		1 * 1024 * 1024 / 127 * 127,   // ~1MB aligned
		2*1024*1024/127*127 + 127*100, // ~2MB + extra chunks
		4*1024*1024/127*127 + 127*333, // ~4MB + odd chunks
		8*1024*1024/127*127 + 127*777, // ~8MB + odd chunks
	}

	for _, unpaddedSize := range sizes {
		paddedSize := unpaddedSize / 127 * 128

		t.Run(fmt.Sprintf("%d_bytes", unpaddedSize), func(t *testing.T) {
			input := make([]byte, unpaddedSize)
			_, err := rand.Read(input)
			require.NoError(t, err)

			padded := make([]byte, paddedSize)
			fr32.Pad(input, padded)

			output := make([]byte, unpaddedSize)
			fr32.Unpad(padded, output)

			require.Equal(t, input, output)
		})
	}
}

func BenchmarkPadChunk(b *testing.B) {
	var buf [128]byte
	in := bytes.Repeat([]byte{0xff}, 127)

	b.SetBytes(127)

	for i := 0; i < b.N; i++ {
		fr32.Pad(in, buf[:])
	}
}

func BenchmarkChunkRoundtrip(b *testing.B) {
	var buf [128]byte
	copy(buf[:], bytes.Repeat([]byte{0xff}, 127))
	var out [127]byte

	b.SetBytes(127)

	for i := 0; i < b.N; i++ {
		fr32.Pad(buf[:], buf[:])
		fr32.Unpad(buf[:], out[:])
	}
}

func BenchmarkUnpadChunk(b *testing.B) {
	var buf [128]byte
	copy(buf[:], bytes.Repeat([]byte{0xff}, 127))

	fr32.Pad(buf[:], buf[:])
	var out [127]byte

	b.SetBytes(127)
	b.ReportAllocs()

	bs := buf[:]

	for i := 0; i < b.N; i++ {
		fr32.Unpad(bs, out[:])
	}
}

func BenchmarkUnpad16MChunk(b *testing.B) {
	up := abi.PaddedPieceSize(16 << 20).Unpadded()

	var buf [16 << 20]byte

	fr32.Pad(bytes.Repeat([]byte{0xff}, int(up)), buf[:])
	var out [16 << 20]byte

	b.SetBytes(16 << 20)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fr32.Unpad(buf[:], out[:])
	}
}

func BenchmarkPad16MChunk(b *testing.B) {
	up := abi.PaddedPieceSize(16 << 20).Unpadded()

	var buf [16 << 20]byte

	in := bytes.Repeat([]byte{0xff}, int(up))

	b.SetBytes(16 << 20)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fr32.Pad(in, buf[:])
	}
}

func BenchmarkPad1GChunk(b *testing.B) {
	up := abi.PaddedPieceSize(1 << 30).Unpadded()

	var buf [1 << 30]byte

	in := bytes.Repeat([]byte{0xff}, int(up))

	b.SetBytes(1 << 30)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fr32.Pad(in, buf[:])
	}
}

func BenchmarkUnpad1GChunk(b *testing.B) {
	up := abi.PaddedPieceSize(1 << 30).Unpadded()

	var buf [1 << 30]byte

	fr32.Pad(bytes.Repeat([]byte{0xff}, int(up)), buf[:])
	var out [1 << 30]byte

	b.SetBytes(1 << 30)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fr32.Unpad(buf[:], out[:])
	}
}
