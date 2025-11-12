package fr32_test

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fr32"
)

func TestUnpadReader(t *testing.T) {
	ps := abi.PaddedPieceSize(64 << 20).Unpadded()

	raw := bytes.Repeat([]byte{0x77}, int(ps))

	padOut := make([]byte, ps.Padded())
	fr32.Pad(raw, padOut)

	r, err := fr32.NewUnpadReader(bytes.NewReader(padOut), ps.Padded())
	if err != nil {
		t.Fatal(err)
	}

	// using bufio reader to make sure reads are big enough for the padreader - it can't handle small reads right now
	readered, err := io.ReadAll(bufio.NewReaderSize(r, 512))
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, raw, readered)
}

func TestUnpadReaderBufWithSmallWorkBuf(t *testing.T) {
	ps := abi.PaddedPieceSize(64 << 20).Unpadded()

	raw := bytes.Repeat([]byte{0x77}, int(ps))

	padOut := make([]byte, ps.Padded())
	fr32.Pad(raw, padOut)

	buf := make([]byte, abi.PaddedPieceSize(uint64(128)))
	r, err := fr32.NewUnpadReaderBuf(bytes.NewReader(padOut), ps.Padded(), buf)
	if err != nil {
		t.Fatal(err)
	}

	// using bufio reader to make sure reads are big enough for the padreader - it can't handle small reads right now
	readered, err := io.ReadAll(bufio.NewReaderSize(r, 512))
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, raw, readered)
}

func TestPadWriterUnpadReader(t *testing.T) {
	testCases := []struct {
		name      string
		unpadSize abi.UnpaddedPieceSize
		readSizes []int
	}{
		{
			name:      "2K with aligned reads",
			unpadSize: 2 * 127 * 8, // 2K unpadded
			readSizes: []int{127, 127 * 4},
		},
		{
			name:      "Small piece, various read sizes",
			unpadSize: 2 * 127 * 8, // 1016 bytes unpadded
			readSizes: []int{1, 63, 127, 128, 255, 1016},
		},
		{
			name:      "Medium piece, various read sizes",
			unpadSize: 127 * 1024, // 128KB unpadded
			readSizes: []int{1, 127, 128, 255, 1024, 4096, 65536},
		},
		{
			name:      "Large piece, various read sizes",
			unpadSize: 127 * 1024 * 1024, // 128MB unpadded
			readSizes: []int{1, 127, 128, 255, 1024, 4096, 65536, 10 << 20, 11 << 20, 11<<20 + 134},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate random unpadded data
			unpadded := make([]byte, tc.unpadSize)
			n, err := rand.Read(unpadded)
			require.NoError(t, err)
			require.Equal(t, int(tc.unpadSize), n)

			// Create a buffer to store padded data
			paddedBuf := new(bytes.Buffer)

			// Create and use PadWriter
			padWriter := fr32.NewPadWriter(paddedBuf)
			written, err := padWriter.Write(unpadded)
			require.NoError(t, err)
			require.Equal(t, int(tc.unpadSize), written)
			require.NoError(t, padWriter.Close())

			// Create UnpadReader
			paddedSize := tc.unpadSize.Padded()
			unpadReader, err := fr32.NewUnpadReader(bytes.NewReader(paddedBuf.Bytes()), paddedSize)
			require.NoError(t, err)

			offset := int64(0)
			for _, readSize := range tc.readSizes {
				t.Run(fmt.Sprintf("ReadSize_%d_Offset_%d", readSize, offset), func(t *testing.T) {
					// Seek to offset
					require.NoError(t, err)

					// Read data
					readBuf := make([]byte, readSize)
					n, err := io.ReadFull(unpadReader, readBuf)
					require.NoError(t, err)
					require.Equal(t, readSize, n)

					// Compare with original unpadded data
					expected := unpadded[offset : offset+int64(len(readBuf))]
					require.Equal(t, expected, readBuf)
					offset += int64(n)
				})
			}
		})
	}
}

func TestUnpadReaderSmallReads(t *testing.T) {
	unpadSize := abi.UnpaddedPieceSize(127 * 1024) // 128KB unpadded
	unpadded := make([]byte, unpadSize)
	n, err := rand.Read(unpadded)
	require.NoError(t, err)
	require.Equal(t, int(unpadSize), n)

	paddedBuf := new(bytes.Buffer)
	padWriter := fr32.NewPadWriter(paddedBuf)
	_, err = padWriter.Write(unpadded)
	require.NoError(t, err)
	require.NoError(t, padWriter.Close())

	paddedSize := unpadSize.Padded()
	unpadReader, err := fr32.NewUnpadReader(bytes.NewReader(paddedBuf.Bytes()), paddedSize)
	require.NoError(t, err)

	result := make([]byte, 0, unpadSize)
	smallBuf := make([]byte, 1) // Read one byte at a time

	for {
		n, err := unpadReader.Read(smallBuf)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		result = append(result, smallBuf[:n]...)
	}

	require.Equal(t, unpadded, result)
}

func TestUnpadReaderLargeReads(t *testing.T) {
	unpadSize := abi.UnpaddedPieceSize(127 * 1024 * 1024) // 128MB unpadded
	unpadded := make([]byte, unpadSize)
	n, err := rand.Read(unpadded)
	require.NoError(t, err)
	require.Equal(t, int(unpadSize), n)

	paddedBuf := new(bytes.Buffer)
	padWriter := fr32.NewPadWriter(paddedBuf)
	_, err = padWriter.Write(unpadded)
	require.NoError(t, err)
	require.NoError(t, padWriter.Close())

	paddedSize := unpadSize.Padded()
	unpadReader, err := fr32.NewUnpadReader(bytes.NewReader(paddedBuf.Bytes()), paddedSize)
	require.NoError(t, err)

	largeBuf := make([]byte, 10*1024*1024) // 10MB buffer
	result := make([]byte, 0, unpadSize)

	for {
		n, err := unpadReader.Read(largeBuf)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		result = append(result, largeBuf[:n]...)
	}

	require.Equal(t, unpadded, result)
}

// TestUnpadReaderPartialPiece tests the edge case where the underlying reader
// has less data than UnpadReader expects. This happens when dealing with pieces
// that are not exact power-of-2 sizes.
// This test captures the fix for the EOF errors that occurred after PR #12884.
func TestUnpadReaderPartialPiece(t *testing.T) {
	actualUnpadded := abi.UnpaddedPieceSize(127 * 100) // 100 chunks = 12700 bytes
	declaredSize := abi.PaddedPieceSize(128 * 128)     // Declare 128 chunks but only have 100

	// Generate test data
	unpadded := make([]byte, actualUnpadded)
	n, err := rand.Read(unpadded)
	require.NoError(t, err)
	require.Equal(t, int(actualUnpadded), n)

	// Pad the data
	paddedActual := actualUnpadded.Padded()
	padded := make([]byte, paddedActual)
	fr32.Pad(unpadded, padded)

	// Create a reader that returns EOF after the actual data
	limitedReader := bytes.NewReader(padded)

	// Create UnpadReader with the larger declared size
	unpadReader, err := fr32.NewUnpadReader(limitedReader, declaredSize)
	require.NoError(t, err)

	// Read all data
	result := make([]byte, 0, actualUnpadded*2)
	buf := make([]byte, 1024)

	for {
		n, err := unpadReader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		// The fix allows UnpadReader to handle partial reads gracefully
		require.NoError(t, err, "UnpadReader should handle partial pieces without error")
	}

	// Verify we got the correct data
	require.Equal(t, int(actualUnpadded), len(result), "Should read all available data")
	require.Equal(t, unpadded, result, "Data should match original")
}
