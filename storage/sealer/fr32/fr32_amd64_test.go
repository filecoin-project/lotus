//go:build amd64

package fr32

import (
	"bytes"
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
