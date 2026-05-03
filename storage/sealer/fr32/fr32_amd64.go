//go:build amd64

package fr32

import "golang.org/x/sys/cpu"

func init() {
	padImpl = padScalar
	unpadImpl = unpadScalar

	// The 64-bit SWAR pad path is faster than the current AVX2/AVX512 paths on
	// Zen4. AVX512 still wins slightly for unpad after masked tail vectorization.
	if cpu.X86.HasAVX512BW && cpu.X86.HasAVX512VL && cpu.X86.HasAVX512VBMI {
		unpadImpl = unpadAVX512
	}
}

//go:noescape
func padScalar(in, out []byte)

//go:noescape
func unpadScalar(out, in []byte)

//go:noescape
func padAVX2(in, out []byte)

//go:noescape
func unpadAVX2(out, in []byte)

//go:noescape
func padAVX512(in, out []byte)

//go:noescape
func unpadAVX512(out, in []byte)
