//go:build amd64

package fr32

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"golang.org/x/sys/cpu"
)

type asmImpl struct {
	name      string
	pad       func([]byte, []byte)
	unpad     func([]byte, []byte)
	supported bool
}

func asmImpls() []asmImpl {
	return []asmImpl{
		{"go", padGo, unpadGo, true},
		{"scalar", padScalar, unpadScalar, true},
		{"scalar-nt-pad", padScalarNT, unpadScalar, true},
		{"avx2", padAVX2, unpadAVX2, cpu.X86.HasAVX2},
		{"avx512", padAVX512, unpadAVX512, cpu.X86.HasAVX512BW && cpu.X86.HasAVX512VL && cpu.X86.HasAVX512VBMI},
	}
}

func TestAsmImplementations(t *testing.T) {
	inputs := [][]byte{
		bytes.Repeat([]byte{0x00}, 127),
		bytes.Repeat([]byte{0x01}, 127),
		bytes.Repeat([]byte{0x80}, 127),
		bytes.Repeat([]byte{0xff}, 127),
		make([]byte, 127*17),
	}
	rand.New(rand.NewSource(1)).Read(inputs[len(inputs)-1])

	for _, impl := range asmImpls() {
		for _, input := range inputs {
			t.Run(impl.name, func(t *testing.T) {
				if !impl.supported {
					t.Skip("CPU does not support this backend")
				}

				expect := make([]byte, len(input)/127*128)
				padGo(input, expect)

				padded := make([]byte, len(expect))
				impl.pad(input, padded)
				if !bytes.Equal(expect, padded) {
					i := firstDiff(expect, padded)
					t.Fatalf("pad mismatch at %d: expected %02x got %02x", i, expect[i], padded[i])
				}

				out := make([]byte, len(input))
				impl.unpad(out, padded)
				if !bytes.Equal(input, out) {
					i := firstDiff(input, out)
					t.Fatalf("unpad mismatch at %d: expected %02x got %02x", i, input[i], out[i])
				}

			})
		}
	}
}

// TestAsmImplementationsLarge verifies every backend against the pure Go
// implementation for multi-chunk random data at various sizes, including
// sizes that cross the NT store threshold.
func TestAsmImplementationsLarge(t *testing.T) {
	sizes := []struct {
		name      string
		numChunks int
	}{
		{"2_chunks", 2},
		{"7_chunks", 7},
		{"64_chunks", 64},
		{"1000_chunks", 1000},         // ~127KB, below NT threshold
		{"2048_chunks", 2048},         // ~256KB, at NT threshold boundary
		{"2049_chunks", 2049},         // just above NT threshold
		{"4096_chunks", 4096},         // ~512KB
		{"8192_chunks_1M", 8192},      // ~1MB
		{"131072_chunks_16M", 131072}, // ~16MB, exercises multithread path
	}

	rng := rand.New(rand.NewSource(42))

	for _, impl := range asmImpls() {
		for _, sz := range sizes {
			t.Run(impl.name+"/"+sz.name, func(t *testing.T) {
				if !impl.supported {
					t.Skip("CPU does not support this backend")
				}

				unpaddedSize := sz.numChunks * 127
				paddedSize := sz.numChunks * 128

				input := make([]byte, unpaddedSize)
				rng.Read(input)

				// Reference: pure Go
				expectPadded := make([]byte, paddedSize)
				padGo(input, expectPadded)

				// Test pad
				gotPadded := make([]byte, paddedSize)
				impl.pad(input, gotPadded)
				if !bytes.Equal(expectPadded, gotPadded) {
					i := firstDiff(expectPadded, gotPadded)
					t.Fatalf("pad mismatch at byte %d (chunk %d offset %d): expected %02x got %02x",
						i, i/128, i%128, expectPadded[i], gotPadded[i])
				}

				// Test unpad
				gotUnpadded := make([]byte, unpaddedSize)
				impl.unpad(gotUnpadded, gotPadded)
				if !bytes.Equal(input, gotUnpadded) {
					i := firstDiff(input, gotUnpadded)
					t.Fatalf("unpad mismatch at byte %d (chunk %d offset %d): expected %02x got %02x",
						i, i/127, i%127, input[i], gotUnpadded[i])
				}
			})
		}
	}
}

// TestAsmRoundtripRandom verifies pad then unpad produces the original data for
// each backend with many random inputs.
func TestAsmRoundtripRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(99))

	for _, impl := range asmImpls() {
		t.Run(impl.name, func(t *testing.T) {
			if !impl.supported {
				t.Skip("CPU does not support this backend")
			}

			for i := 0; i < 500; i++ {
				// Random number of chunks 1..64
				numChunks := rng.Intn(64) + 1
				input := make([]byte, numChunks*127)
				rng.Read(input)

				padded := make([]byte, numChunks*128)
				impl.pad(input, padded)

				output := make([]byte, numChunks*127)
				impl.unpad(output, padded)

				if !bytes.Equal(input, output) {
					i := firstDiff(input, output)
					t.Fatalf("roundtrip mismatch at byte %d (chunk %d): expected %02x got %02x",
						i, i/127, input[i], output[i])
				}
			}
		})
	}
}

// TestDispatchedPadMatchesGo verifies the default-dispatched Pad/Unpad
// (which selects backends based on CPU and size thresholds) matches padGo.
func TestDispatchedPadMatchesGo(t *testing.T) {
	sizes := []int{
		127,          // 1 chunk
		127 * 17,     // 17 chunks
		127 * 2048,   // at NT threshold
		127 * 2049,   // just above
		127 * 131072, // 16MB, multithread
	}

	rng := rand.New(rand.NewSource(77))

	for _, unpaddedSize := range sizes {
		paddedSize := unpaddedSize / 127 * 128
		t.Run(fmt.Sprintf("%d_bytes", unpaddedSize), func(t *testing.T) {
			input := make([]byte, unpaddedSize)
			rng.Read(input)

			expectPadded := make([]byte, paddedSize)
			padGo(input, expectPadded)

			gotPadded := make([]byte, paddedSize)
			pad(input, gotPadded)
			if !bytes.Equal(expectPadded, gotPadded) {
				i := firstDiff(expectPadded, gotPadded)
				t.Fatalf("dispatched pad mismatch at byte %d: expected %02x got %02x",
					i, expectPadded[i], gotPadded[i])
			}

			expectUnpadded := make([]byte, unpaddedSize)
			unpadGo(expectUnpadded, expectPadded)

			gotUnpadded := make([]byte, unpaddedSize)
			unpad(gotUnpadded, gotPadded)
			if !bytes.Equal(expectUnpadded, gotUnpadded) {
				i := firstDiff(expectUnpadded, gotUnpadded)
				t.Fatalf("dispatched unpad mismatch at byte %d: expected %02x got %02x",
					i, expectUnpadded[i], gotUnpadded[i])
			}
		})
	}
}

func BenchmarkAsmPadMatrix(b *testing.B) {
	benchPadMatrix(b, 128)
}

func BenchmarkAsmPadMatrix16M(b *testing.B) {
	benchPadMatrix(b, 16<<20)
}

func BenchmarkAsmUnpadMatrix(b *testing.B) {
	benchUnpadMatrix(b, 128)
}

func BenchmarkAsmUnpadMatrix16M(b *testing.B) {
	benchUnpadMatrix(b, 16<<20)
}

func BenchmarkSequentialCopyBaseline(b *testing.B) {
	benchSequentialCopyBaseline(b, 128)
}

func BenchmarkSequentialCopyBaseline16M(b *testing.B) {
	benchSequentialCopyBaseline(b, 16<<20)
}

func benchPadMatrix(b *testing.B, paddedSize int) {
	input := make([]byte, paddedSize/128*127)
	out := make([]byte, paddedSize)
	rand.New(rand.NewSource(2)).Read(input)

	for _, impl := range asmImpls() {
		b.Run(impl.name, func(b *testing.B) {
			if !impl.supported {
				b.Skip("CPU does not support this backend")
			}
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				impl.pad(input, out)
			}
		})
	}
}

func benchUnpadMatrix(b *testing.B, paddedSize int) {
	input := make([]byte, paddedSize/128*127)
	padded := make([]byte, paddedSize)
	out := make([]byte, len(input))
	rand.New(rand.NewSource(3)).Read(input)
	padGo(input, padded)

	for _, impl := range asmImpls() {
		b.Run(impl.name, func(b *testing.B) {
			if !impl.supported {
				b.Skip("CPU does not support this backend")
			}
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				impl.unpad(out, padded)
			}
		})
	}
}

func benchSequentialCopyBaseline(b *testing.B, size int) {
	src := make([]byte, size)
	dst := make([]byte, size)
	rand.New(rand.NewSource(4)).Read(src)

	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		copy(dst, src)
	}
}

func firstDiff(a, b []byte) int {
	for i := range a {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return min(len(a), len(b))
	}
	return -1
}
