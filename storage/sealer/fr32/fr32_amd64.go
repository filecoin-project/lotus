//go:build amd64

package fr32

import "golang.org/x/sys/cpu"

// ntPadThresh is the padded-size threshold above which non-temporal stores
// are used for pad to avoid write-allocate traffic. Only works for pad because
// the 128-byte output stride is cache-line aligned. Unpad's 127-byte output
// stride prevents effective write-combining.
const ntPadThresh = 256 * 1024

func init() {
	padImpl = padSWAR
	unpadImpl = unpadScalar

	if cpu.X86.HasAVX512BW && cpu.X86.HasAVX512VL && cpu.X86.HasAVX512VBMI {
		unpadImpl = unpadAVX512
	}
}

func padSWAR(in, out []byte) {
	if len(out) >= ntPadThresh {
		padScalarNT(in, out)
	} else {
		padScalar(in, out)
	}
}

//go:noescape
func padScalar(in, out []byte)

//go:noescape
func padScalarNT(in, out []byte)

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
