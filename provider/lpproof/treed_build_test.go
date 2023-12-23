package lpproof

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
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

func Test2K(t *testing.T) {
	data := make([]byte, 2048)
	data[0] = 0x01

	tempFile := filepath.Join(t.TempDir(), "tree.dat")

	commd, err := BuildTreeD(bytes.NewReader(data), tempFile, 2048)
	require.NoError(t, err)
	fmt.Println(commd)

	// dump tree.dat
	dat, err := os.ReadFile(tempFile)
	require.NoError(t, err)

	for i, b := range dat {
		// 32 values per line
		if i%32 == 0 {
			fmt.Println()

			// line offset hexdump style
			fmt.Printf("%04x: ", i)
		}
		fmt.Printf("%02x ", b)
	}
	fmt.Println()

	require.Equal(t, "baga6ea4seaqovgk4kr4eoifujh6jfmdqvw3m6zrvyjqzu6s6abkketui6jjoydi", commd.String())

}

func Test8MiB(t *testing.T) {
	data := make([]byte, 8<<20)
	data[0] = 0x01

	tempFile := filepath.Join(t.TempDir(), "tree.dat")

	commd, err := BuildTreeD(bytes.NewReader(data), tempFile, 8<<20)
	require.NoError(t, err)
	fmt.Println(commd)

	// dump tree.dat
	dat, err := os.ReadFile(tempFile)
	require.NoError(t, err)

	actualD := hexPrint32LDedup(dat)

	expectD := `00000000: 01 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 
00000020: 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 
*
00800000: 16 ab ab 34 1f b7 f3 70 e2 7e 4d ad cf 81 76 6d d0 df d0 ae 64 46 94 77 bb 2c f6 61 49 38 b2 2f 
00800020: f5 a5 fd 42 d1 6a 20 30 27 98 ef 6e d3 09 97 9b 43 00 3d 23 20 d9 f0 e8 ea 98 31 a9 27 59 fb 0b 
*
00c00000: 0d d6 da e4 1c 2f 75 55 01 29 59 4f b6 44 e4 a8 42 cf af b3 16 a2 d5 93 21 e3 88 fe 84 a1 ec 2f 
00c00020: 37 31 bb 99 ac 68 9f 66 ee f5 97 3e 4a 94 da 18 8f 4d dc ae 58 07 24 fc 6f 3f d6 0d fd 48 83 33 
*
00e00000: 11 b1 c4 80 05 21 d5 e5 83 4a de b3 70 7c 74 15 9f f3 37 b0 96 16 3c 94 31 16 73 40 e7 b1 17 1d 
00e00020: 64 2a 60 7e f8 86 b0 04 bf 2c 19 78 46 3a e1 d4 69 3a c0 f4 10 eb 2d 1b 7a 47 fe 20 5e 5e 75 0f 
*
00f00000: ec 69 25 55 9b cc 52 84 0a 22 38 5b 2b 6b 35 b4 50 14 50 04 28 f4 59 fe c1 23 01 0f e7 ef 18 1c 
00f00020: 57 a2 38 1a 28 65 2b f4 7f 6b ef 7a ca 67 9b e4 ae de 58 71 ab 5c f3 eb 2c 08 11 44 88 cb 85 26 
*
00f80000: 3d d2 eb 19 3e e2 f0 47 34 87 bf 4b 83 aa 3a bd a9 c8 4e fa e5 52 6d 8a fd 61 2d 5d 9e 3d 79 34 
00f80020: 1f 7a c9 59 55 10 e0 9e a4 1c 46 0b 17 64 30 bb 32 2c d6 fb 41 2e c5 7c b1 7d 98 9a 43 10 37 2f 
*
00fc0000: ea 99 5c 54 78 47 20 b4 49 fc 92 b0 70 ad b6 cf 66 35 c2 61 9a 7a 5e 00 54 a2 4e 88 f2 52 ec 0d 
00fc0020: fc 7e 92 82 96 e5 16 fa ad e9 86 b2 8f 92 d4 4a 4f 24 b9 35 48 52 23 37 6a 79 90 27 bc 18 f8 33 
*
00fe0000: b9 97 02 8b 06 d7 2e 96 07 86 79 58 e1 5f 8d 07 b7 ae 37 ab 29 ab 3f a9 de fe c9 8e aa 37 6e 28 
00fe0020: 08 c4 7b 38 ee 13 bc 43 f4 1b 91 5c 0e ed 99 11 a2 60 86 b3 ed 62 40 1b f9 d5 8b 8d 19 df f6 24 
*
00ff0000: a0 c4 4f 7b a4 4c d2 3c 2e bf 75 98 7b e8 98 a5 63 80 73 b2 f9 11 cf ee ce 14 5a 77 58 0c 6c 12 
00ff0020: b2 e4 7b fb 11 fa cd 94 1f 62 af 5c 75 0f 3e a5 cc 4d f5 17 d5 c4 f1 6d b2 b4 d7 7b ae c1 a3 2f 
*
00ff8000: 89 2d 2b 00 a5 c1 54 10 94 ca 65 de 21 3b bd 45 90 14 15 ed d1 10 17 cd 29 f3 ed 75 73 02 a0 3f 
00ff8020: f9 22 61 60 c8 f9 27 bf dc c4 18 cd f2 03 49 31 46 00 8e ae fb 7d 02 19 4d 5e 54 81 89 00 51 08 
*
00ffc000: 22 48 54 8b ba a5 8f e2 db 0b 07 18 c1 d7 20 1f ed 64 c7 8d 7d 22 88 36 b2 a1 b2 f9 42 0b ef 3c 
00ffc020: 2c 1a 96 4b b9 0b 59 eb fe 0f 6d a2 9a d6 5a e3 e4 17 72 4a 8f 7c 11 74 5a 40 ca c1 e5 e7 40 11 
*
00ffe000: 1c 6a 48 08 3e 17 49 90 ef c0 56 ec b1 44 75 1d e2 76 d8 a5 1c 3d 93 d7 4c 81 92 48 ab 78 cc 30 
00ffe020: fe e3 78 ce f1 64 04 b1 99 ed e0 b1 3e 11 b6 24 ff 9d 78 4f bb ed 87 8d 83 29 7e 79 5e 02 4f 02 
*
00fff000: 0a b4 26 38 1b 72 cd 3b b3 e3 c7 82 18 fe 1f 18 3b 3a 19 db c4 d9 26 94 30 03 cd 01 b6 d1 8d 0b 
00fff020: 8e 9e 24 03 fa 88 4c f6 23 7f 60 df 25 f8 3e e4 0d ca 9e d8 79 eb 6f 63 52 d1 50 84 f5 ad 0d 3f 
*
00fff800: 16 0d 87 17 1b e7 ae e4 20 a3 54 24 cf df 4f fe a2 fd 7b 94 58 89 58 f3 45 11 57 fc 39 8f 34 26 
00fff820: 75 2d 96 93 fa 16 75 24 39 54 76 e3 17 a9 85 80 f0 09 47 af b7 a3 05 40 d6 25 a9 29 1c c1 2a 07 
*
00fffc00: 1f 40 60 11 da 08 f8 09 80 63 97 dc 1c 57 b9 87 83 37 5a 59 5d d6 81 42 6c 1e cd d4 3c ab e3 3c 
00fffc20: 70 22 f6 0f 7e f6 ad fa 17 11 7a 52 61 9e 30 ce a8 2c 68 07 5a df 1c 66 77 86 ec 50 6e ef 2d 19 
*
00fffe00: 51 4e dd 2f 6f 8f 6d fd 54 b0 d1 20 7b b7 06 df 85 c5 a3 19 0e af 38 72 37 20 c5 07 56 67 7f 14 
00fffe20: d9 98 87 b9 73 57 3a 96 e1 13 93 64 52 36 c1 7b 1f 4c 70 34 d7 23 c7 a9 9f 70 9b b4 da 61 16 2b 
*
00ffff00: 5a 1d 84 74 85 a3 4b 28 08 93 a9 cf b2 8b 54 44 67 12 8b eb c0 22 bd de c1 04 be ca b4 f4 81 31 
00ffff20: d0 b5 30 db b0 b4 f2 5c 5d 2f 2a 28 df ee 80 8b 53 41 2a 02 93 1f 18 c4 99 f5 a2 54 08 6b 13 26 
*
00ffff80: c5 fb f3 f9 4c c2 2b 3c 51 ad c1 ea af e9 4b a0 9f b2 73 f3 73 d2 10 1f 12 0b 11 c6 85 21 66 2f 
00ffffa0: 84 c0 42 1b a0 68 5a 01 bf 79 5a 23 44 06 4f e4 24 bd 52 a9 d2 43 77 b3 94 ff 4c 4b 45 68 e8 11 
00ffffc0: 23 40 4a 88 80 f9 cb c7 20 39 cb 86 14 35 9c 28 34 84 55 70 fe 95 19 0b bd 4d 93 41 42 e8 25 2c 
`

	fmt.Println(actualD)

	require.EqualValues(t, expectD, actualD)

	require.Equal(t, "baga6ea4seaqcgqckrcapts6hea44xbqugwocqneekvyp5fizbo6u3e2biluckla", commd.String())
}

func hexPrint32LDedup(b []byte) string {
	var prevLine string
	var currentLine string
	var isDuplicate bool

	var outStr string

	for i, v := range b {
		// Build the current line
		currentLine += fmt.Sprintf("%02x ", v)

		if (i+1)%32 == 0 || i == len(b)-1 {
			// Check for duplicate line
			wasDuplicate := isDuplicate
			isDuplicate = currentLine == prevLine

			if !isDuplicate {
				if wasDuplicate {
					//fmt.Println("*")
					outStr += fmt.Sprintln("*")
				}

				//fmt.Printf("%08x: %s\n", i-31, currentLine)
				outStr += fmt.Sprintf("%08x: %s\n", i-31, currentLine)
			}

			// Update the previous line and reset current line
			prevLine = currentLine
			currentLine = ""
		}
	}

	return outStr
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
