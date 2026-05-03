package fr32

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/filecoin-project/go-state-types/abi"
)

// UnpaddedFr32Chunk is the minimum amount of data which can be fr32-padded
// Fr32 padding inserts two zero bits every 254 bits, so the minimum amount of
// data which can be padded is 254 bits. 127 bytes is the smallest multiple of
// 254 bits which has a whole number of bytes.
const UnpaddedFr32Chunk abi.UnpaddedPieceSize = 127

// PaddedFr32Chunk is the size of a UnpaddedFr32Chunk chunk after fr32 padding
const PaddedFr32Chunk abi.PaddedPieceSize = 128

func init() {
	if PaddedFr32Chunk != UnpaddedFr32Chunk.Padded() {
		panic("bad math")
	}
}

// MTTresh is the padded-size threshold above which multithreaded dispatch is
// used. Exported for use by readers.go BufSize().
var MTTresh = uint64(512 << 10)

var (
	padImpl   = padGo
	unpadImpl = unpadGo
)

// mtChunkCount returns the number of worker goroutines for a given padded size.
// The per-thread minimum work size is set to balance goroutine overhead against
// parallelism. Pad uses a smaller minimum (512KB) because NT stores benefit from
// more memory controllers. Unpad uses a larger minimum (2MB) because temporal
// stores benefit from L3 cache locality.
func mtChunkCount(usz abi.PaddedPieceSize) uint64 {
	return mtChunkCountMinWork(usz, MTTresh)
}

func mtChunkCountMinWork(usz abi.PaddedPieceSize, minWorkPerThread uint64) uint64 {
	threads := uint64(usz) / minWorkPerThread
	ncpu := uint64(runtime.NumCPU())
	if threads > ncpu {
		threads = ncpu
	}
	if threads == 0 {
		return 1
	}
	return threads
}

func mtWithMinWork(in, out []byte, padLen int, minWork uint64, op func(unpadded, padded []byte)) {
	threads := mtChunkCountMinWork(abi.PaddedPieceSize(padLen), minWork)

	// Ensure threadBytes is aligned to 128-byte chunk boundaries.
	chunksPerThread := (padLen / int(threads)) / 128
	if chunksPerThread == 0 {
		chunksPerThread = 1
	}
	threadBytes := abi.PaddedPieceSize(chunksPerThread * 128)

	var wg sync.WaitGroup
	wg.Add(int(threads))

	for i := 0; i < int(threads); i++ {
		go func(thread int) {
			defer wg.Done()

			start := threadBytes * abi.PaddedPieceSize(thread)
			end := start + threadBytes

			if thread == int(threads)-1 {
				end = abi.PaddedPieceSize(padLen)
			}

			if start >= abi.PaddedPieceSize(padLen) {
				return
			}

			op(in[start.Unpadded():end.Unpadded()], out[start:end])
		}(i)
	}
	wg.Wait()
}

func mt(in, out []byte, padLen int, op func(unpadded, padded []byte)) {
	threads := mtChunkCount(abi.PaddedPieceSize(padLen))

	// Ensure threadBytes is aligned to 128-byte chunk boundaries.
	// Each fr32 chunk is 128 padded bytes / 127 unpadded bytes.
	chunksPerThread := (padLen / int(threads)) / 128
	if chunksPerThread == 0 {
		chunksPerThread = 1
	}
	threadBytes := abi.PaddedPieceSize(chunksPerThread * 128)

	var wg sync.WaitGroup
	wg.Add(int(threads))

	for i := 0; i < int(threads); i++ {
		go func(thread int) {
			defer wg.Done()

			start := threadBytes * abi.PaddedPieceSize(thread)
			end := start + threadBytes

			// Last thread takes any remainder
			if thread == int(threads)-1 {
				end = abi.PaddedPieceSize(padLen)
			}

			// Skip if this thread has no work
			if start >= abi.PaddedPieceSize(padLen) {
				return
			}

			op(in[start.Unpadded():end.Unpadded()], out[start:end])
		}(i)
	}
	wg.Wait()
}

func Pad(in, out []byte) {
	// Assumes len(in)%127==0 and len(out)%128==0
	assertNoOverlap(in, out)
	if len(out) > int(MTTresh) {
		// 512KB per thread: pad uses NT stores which bypass cache, so more
		// threads = more memory controllers utilized.
		mtWithMinWork(in, out, len(out), 512<<10, pad)
		return
	}

	pad(in, out)
}

func PadSingle(in, out []byte) {
	assertNoOverlap(in, out)
	pad(in, out)
}

func assertNoOverlap(in, out []byte) {
	if slicesOverlap(in, out) {
		panic("fr32: input and output buffers must not overlap")
	}
}

func slicesOverlap(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	aStart := uintptr(unsafe.Pointer(&a[0]))
	bStart := uintptr(unsafe.Pointer(&b[0]))
	aEnd := aStart + uintptr(len(a))
	bEnd := bStart + uintptr(len(b))

	return aStart < bEnd && bStart < aEnd
}

func pad(in, out []byte) {
	padImpl(in, out)
}

func padGo(in, out []byte) {
	chunks := len(out) / 128
	for chunk := 0; chunk < chunks; chunk++ {
		inOff := chunk * 127
		outOff := chunk * 128

		copy(out[outOff:outOff+31], in[inOff:inOff+31])

		t := in[inOff+31] >> 6
		out[outOff+31] = in[inOff+31] & 0x3f
		var v byte

		for i := 32; i < 64; i++ {
			v = in[inOff+i]
			out[outOff+i] = (v << 2) | t
			t = v >> 6
		}

		t = v >> 4
		out[outOff+63] &= 0x3f

		for i := 64; i < 96; i++ {
			v = in[inOff+i]
			out[outOff+i] = (v << 4) | t
			t = v >> 4
		}

		t = v >> 2
		out[outOff+95] &= 0x3f

		for i := 96; i < 127; i++ {
			v = in[inOff+i]
			out[outOff+i] = (v << 6) | t
			t = v >> 2
		}

		out[outOff+127] = t & 0x3f
	}
}

func Unpad(in []byte, out []byte) {
	// Assumes len(in)%128==0 and len(out)%127==0
	assertNoOverlap(in, out)
	if len(in) > int(MTTresh) {
		// 2MB per thread: unpad uses temporal stores which benefit from L3
		// cache locality, so fewer larger chunks improve hit rate.
		mtWithMinWork(out, in, len(in), 2<<20, unpad)
		return
	}

	unpad(out, in)
}

func unpad(out, in []byte) {
	unpadImpl(out, in)
}

func unpadGo(out, in []byte) {
	chunks := len(in) / 128
	for chunk := 0; chunk < chunks; chunk++ {
		inOffNext := chunk*128 + 1
		outOff := chunk * 127

		at := in[chunk*128]

		for i := 0; i < 32; i++ {
			next := in[i+inOffNext]

			out[outOff+i] = at
			//out[i] |= next << 8

			at = next
		}

		out[outOff+31] |= at << 6

		for i := 32; i < 64; i++ {
			next := in[i+inOffNext]

			out[outOff+i] = at >> 2
			out[outOff+i] |= next << 6

			at = next
		}

		out[outOff+63] ^= (at << 6) ^ (at << 4)

		for i := 64; i < 96; i++ {
			next := in[i+inOffNext]

			out[outOff+i] = at >> 4
			out[outOff+i] |= next << 4

			at = next
		}

		out[outOff+95] ^= (at << 4) ^ (at << 2)

		for i := 96; i < 127; i++ {
			next := in[i+inOffNext]

			out[outOff+i] = at >> 6
			out[outOff+i] |= next << 2

			at = next
		}
	}
}
