package lpproof

import (
	"crypto/rand"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTreeSize(t *testing.T) {
	require.Equal(t, uint64(32), treeSize(abi.PaddedPieceSize(32)))
	require.Equal(t, uint64(64+32), treeSize(abi.PaddedPieceSize(64)))
	require.Equal(t, uint64(128+64+32), treeSize(abi.PaddedPieceSize(128)))
	require.Equal(t, uint64(256+128+64+32), treeSize(abi.PaddedPieceSize(256)))
}

func TestTreeLayerOffset(t *testing.T) {
	require.Equal(t, uint64(0), layerOffset(128, 0))
	require.Equal(t, uint64(128), layerOffset(128, 1))
	require.Equal(t, uint64(128+64), layerOffset(128, 2))
	require.Equal(t, uint64(128+64+32), layerOffset(128, 3))
}

func TestHashChunk(t *testing.T) {
	chunk := make([]byte, 64)
	chunk[0] = 0x01

	out := make([]byte, 32)

	data := [][]byte{chunk, out}
	hashChunk(data)

	// 16 ab ab 34 1f b7 f3 70  e2 7e 4d ad cf 81 76 6d
	// d0 df d0 ae 64 46 94 77  bb 2c f6 61 49 38 b2 2f
	expect := []byte{
		0x16, 0xab, 0xab, 0x34, 0x1f, 0xb7, 0xf3, 0x70,
		0xe2, 0x7e, 0x4d, 0xad, 0xcf, 0x81, 0x76, 0x6d,
		0xd0, 0xdf, 0xd0, 0xae, 0x64, 0x46, 0x94, 0x77,
		0xbb, 0x2c, 0xf6, 0x61, 0x49, 0x38, 0xb2, 0x2f,
	}

	require.Equal(t, expect, out)
}

func TestHashChunk2L(t *testing.T) {
	data0 := make([]byte, 128)
	data0[0] = 0x01

	l1 := make([]byte, 64)
	l2 := make([]byte, 32)

	data := [][]byte{data0, l1, l2}
	hashChunk(data)

	// 16 ab ab 34 1f b7 f3 70  e2 7e 4d ad cf 81 76 6d
	// d0 df d0 ae 64 46 94 77  bb 2c f6 61 49 38 b2 2f
	expectL1Left := []byte{
		0x16, 0xab, 0xab, 0x34, 0x1f, 0xb7, 0xf3, 0x70,
		0xe2, 0x7e, 0x4d, 0xad, 0xcf, 0x81, 0x76, 0x6d,
		0xd0, 0xdf, 0xd0, 0xae, 0x64, 0x46, 0x94, 0x77,
		0xbb, 0x2c, 0xf6, 0x61, 0x49, 0x38, 0xb2, 0x2f,
	}

	// f5 a5 fd 42 d1 6a 20 30  27 98 ef 6e d3 09 97 9b
	// 43 00 3d 23 20 d9 f0 e8  ea 98 31 a9 27 59 fb 0b
	expectL1Rest := []byte{
		0xf5, 0xa5, 0xfd, 0x42, 0xd1, 0x6a, 0x20, 0x30,
		0x27, 0x98, 0xef, 0x6e, 0xd3, 0x09, 0x97, 0x9b,
		0x43, 0x00, 0x3d, 0x23, 0x20, 0xd9, 0xf0, 0xe8,
		0xea, 0x98, 0x31, 0xa9, 0x27, 0x59, 0xfb, 0x0b,
	}

	require.Equal(t, expectL1Left, l1[:32])
	require.Equal(t, expectL1Rest, l1[32:])

	// 0d d6 da e4 1c 2f 75 55  01 29 59 4f b6 44 e4 a8
	// 42 cf af b3 16 a2 d5 93  21 e3 88 fe 84 a1 ec 2f
	expectL2 := []byte{
		0x0d, 0xd6, 0xda, 0xe4, 0x1c, 0x2f, 0x75, 0x55,
		0x01, 0x29, 0x59, 0x4f, 0xb6, 0x44, 0xe4, 0xa8,
		0x42, 0xcf, 0xaf, 0xb3, 0x16, 0xa2, 0xd5, 0x93,
		0x21, 0xe3, 0x88, 0xfe, 0x84, 0xa1, 0xec, 0x2f,
	}

	require.Equal(t, expectL2, l2)
}

func BenchmarkHashChunk(b *testing.B) {
	const benchSize = 1024 * 1024

	// Generate 1 MiB of random data
	randomData := make([]byte, benchSize)
	if _, err := rand.Read(randomData); err != nil {
		b.Fatalf("Failed to generate random data: %v", err)
	}

	// Prepare data structure for hashChunk
	data := make([][]byte, 1)
	data[0] = randomData

	// append levels until we get to a 32 byte level
	for len(data[len(data)-1]) > 32 {
		newLevel := make([]byte, len(data[len(data)-1])/2)
		data = append(data, newLevel)
	}

	b.SetBytes(benchSize) // Set the number of bytes for the benchmark

	b.ResetTimer() // Start the timer after setup

	for i := 0; i < b.N; i++ {
		hashChunk(data)
		// Use the result in some way to avoid compiler optimization
		_ = data[1]
	}
}
