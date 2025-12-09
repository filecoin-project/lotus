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

// TestUnpadReaderSizeMismatch_PieceProviderPattern reproduces the bug where
// piece_provider.go creates an unpadReader with pieceSize.Padded() but the
// underlying reader only has a portion of the data. This simulates reading
// a range from a piece.
//
// The bug: When NewUnpadReaderBuf is given a size much larger than the
// underlying reader actually has, data corruption occurs at the boundary
// where the underlying reader exhausts.
func TestUnpadReaderSizeMismatch_PieceProviderPattern(t *testing.T) {
	// Simulate a large piece (e.g., 32MiB) - must be power of 2
	fullPiecePadded := abi.PaddedPieceSize(32 << 20) // 32MiB padded (power of 2)
	fullPieceUnpadded := fullPiecePadded.Unpadded()

	// But we only want to read a portion (e.g., 8MiB range) - must be power of 2
	rangePadded := abi.PaddedPieceSize(8 << 20) // 8MiB padded (power of 2)
	rangeUnpadded := rangePadded.Unpadded()

	// Generate the full piece data
	fullData := make([]byte, fullPieceUnpadded)
	n, err := rand.Read(fullData)
	require.NoError(t, err)
	require.Equal(t, int(fullPieceUnpadded), n)

	// Pad only the range we'll provide (simulating what readerGetter returns)
	rangeData := fullData[:rangeUnpadded]
	paddedRange := make([]byte, rangePadded)
	fr32.Pad(rangeData, paddedRange)

	// Create underlying reader with ONLY the range data
	underlyingReader := bytes.NewReader(paddedRange)

	// BUG REPRODUCTION: Create UnpadReader with FULL piece size
	// (this is what piece_provider.go does incorrectly)
	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	// Try to read all available data
	result := make([]byte, 0, rangeUnpadded)
	readBuf := make([]byte, 64*1024) // 64KB read buffer

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Got error after reading %d bytes: %v", len(result), err)
			break
		}
	}

	// Verify we got the correct amount of data
	require.Equal(t, int(rangeUnpadded), len(result),
		"Should read exactly the available data, got %d, expected %d", len(result), rangeUnpadded)

	// Verify data integrity - no corruption
	require.Equal(t, rangeData, result, "Data should match original without corruption")
}

// TestUnpadReaderSizeMismatch_8MiBBoundary specifically tests the 8MiB boundary
// issue where zeros appear and subsequent data is bit-shifted.
func TestUnpadReaderSizeMismatch_8MiBBoundary(t *testing.T) {
	// Simulate reading exactly at the 8MiB boundary
	// 8MiB = 0x800000 bytes (power of 2)
	fullPiecePadded := abi.PaddedPieceSize(64 << 20) // 64MiB full piece (power of 2)
	fullPieceUnpadded := fullPiecePadded.Unpadded()

	// Underlying reader has exactly 8MiB of padded data (power of 2)
	eightMiBPadded := abi.PaddedPieceSize(8 << 20)
	eightMiBUnpadded := eightMiBPadded.Unpadded()

	// Generate test data with a recognizable pattern
	fullData := make([]byte, fullPieceUnpadded)
	// Fill with pattern: each 127-byte chunk starts with its chunk number
	for i := 0; i < int(fullPieceUnpadded); i++ {
		chunkNum := i / 127
		posInChunk := i % 127
		fullData[i] = byte((chunkNum + posInChunk) & 0xFF)
	}

	// Pad only the 8MiB range
	rangeData := fullData[:eightMiBUnpadded]
	paddedRange := make([]byte, eightMiBPadded)
	fr32.Pad(rangeData, paddedRange)

	underlyingReader := bytes.NewReader(paddedRange)

	// BUG: Create with full piece size but only 8MiB available
	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	// Read all available data
	result := make([]byte, 0, eightMiBUnpadded+1024)
	readBuf := make([]byte, 128*1024)

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Error after %d bytes: %v", len(result), err)
			break
		}
	}

	// Check for zeros near the 8MiB boundary (0x7fffe0 to 0x800000 in the original bug)
	// These offsets are in unpadded space
	boundaryStart := int(eightMiBUnpadded) - 256 // Check last 256 bytes
	if len(result) >= boundaryStart+256 {
		boundaryData := result[boundaryStart : boundaryStart+256]
		zeros := 0
		for _, b := range boundaryData {
			if b == 0 {
				zeros++
			}
		}
		// The bug caused zeros to appear where they shouldn't
		expectedData := rangeData[boundaryStart : boundaryStart+256]
		expectedZeros := 0
		for _, b := range expectedData {
			if b == 0 {
				expectedZeros++
			}
		}
		if zeros > expectedZeros+10 {
			t.Errorf("Found %d unexpected zeros near 8MiB boundary (expected ~%d)", zeros, expectedZeros)
		}
	}

	// Verify we got correct amount of data
	require.Equal(t, int(eightMiBUnpadded), len(result),
		"Should read all available data without data loss")

	// Verify data integrity
	require.Equal(t, rangeData, result, "Data should match original")
}

// TestUnpadReaderSizeMismatch_BitShiftCorruption tests that data doesn't get
// bit-shifted when the underlying reader exhausts before the declared size.
func TestUnpadReaderSizeMismatch_BitShiftCorruption(t *testing.T) {
	// Create a scenario where partial chunk handling could cause bit shift
	// Both sizes must be power of 2
	fullPiecePadded := abi.PaddedPieceSize(16 << 20) // 16MiB (power of 2)
	fullPieceUnpadded := fullPiecePadded.Unpadded()

	// Use 8MiB as the actual available data (power of 2)
	rangePadded := abi.PaddedPieceSize(8 << 20) // 8MiB (power of 2)
	rangeUnpadded := rangePadded.Unpadded()

	// Generate data with a specific bit pattern to detect shifts
	fullData := make([]byte, fullPieceUnpadded)
	for i := range fullData {
		// Pattern: 0xAA (10101010) - easy to see if bits shift
		fullData[i] = 0xAA
	}

	rangeData := fullData[:rangeUnpadded]
	paddedRange := make([]byte, rangePadded)
	fr32.Pad(rangeData, paddedRange)

	underlyingReader := bytes.NewReader(paddedRange)

	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	result := make([]byte, 0, rangeUnpadded)
	readBuf := make([]byte, 64*1024)

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}

	require.Equal(t, int(rangeUnpadded), len(result), "Should read all data")

	// Check for bit shift corruption
	// If bits are shifted, 0xAA (10101010) would become something else
	// like 0x55 (01010101) for 1-bit shift, or other patterns
	shiftedBytes := 0
	for i, b := range result {
		if b != 0xAA {
			shiftedBytes++
			if shiftedBytes <= 10 {
				t.Logf("Byte at offset %d (0x%x): got 0x%02x, expected 0xAA", i, i, b)
			}
		}
	}

	if shiftedBytes > 0 {
		t.Errorf("Found %d bytes with potential bit-shift corruption (expected 0)", shiftedBytes)
	}
}

// TestUnpadReaderSizeMismatch_MultipleReads tests the scenario where multiple
// sequential reads cross the boundary where underlying data exhausts.
func TestUnpadReaderSizeMismatch_MultipleReads(t *testing.T) {
	fullPiecePadded := abi.PaddedPieceSize(32 << 20)

	// Underlying has 4MiB
	actualPadded := abi.PaddedPieceSize(4 << 20)
	actualUnpadded := actualPadded.Unpadded()

	// Generate random data
	fullData := make([]byte, actualUnpadded)
	_, err := rand.Read(fullData)
	require.NoError(t, err)

	paddedData := make([]byte, actualPadded)
	fr32.Pad(fullData, paddedData)

	underlyingReader := bytes.NewReader(paddedData)

	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	// Read in small chunks to trigger multiple reads across boundary
	result := make([]byte, 0, actualUnpadded)
	readBuf := make([]byte, 127*100) // ~12KB reads

	readCount := 0
	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
			readCount++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Error on read %d after %d bytes: %v", readCount, len(result), err)
			break
		}
	}

	t.Logf("Completed %d reads, got %d bytes", readCount, len(result))

	require.Equal(t, int(actualUnpadded), len(result),
		"Should read all available data (%d bytes), got %d", actualUnpadded, len(result))
	require.Equal(t, fullData, result, "Data integrity check failed")
}

// TestUnpadReaderSizeMismatch_SmallReadBuffer tests the issue with small read
// buffers that trigger the stash mechanism.
func TestUnpadReaderSizeMismatch_SmallReadBuffer(t *testing.T) {
	fullPiecePadded := abi.PaddedPieceSize(16 << 20)

	actualPadded := abi.PaddedPieceSize(2 << 20) // 2MiB actual
	actualUnpadded := actualPadded.Unpadded()

	fullData := make([]byte, actualUnpadded)
	_, err := rand.Read(fullData)
	require.NoError(t, err)

	paddedData := make([]byte, actualPadded)
	fr32.Pad(fullData, paddedData)

	underlyingReader := bytes.NewReader(paddedData)

	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	// Use very small reads to heavily exercise the stash mechanism
	result := make([]byte, 0, actualUnpadded)
	readBuf := make([]byte, 100) // Very small - will trigger stash

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Error after %d bytes: %v", len(result), err)
			break
		}
	}

	require.Equal(t, int(actualUnpadded), len(result), "Should read all data")
	require.Equal(t, fullData, result, "Data should match")
}

// TestUnpadReaderSizeMismatch_OffsetRead simulates the piece_provider pattern
// more closely: reading from an offset within a piece, where the underlying
// reader only has data for the requested range but the unpadReader is told
// about the full piece size.
func TestUnpadReaderSizeMismatch_OffsetRead(t *testing.T) {
	// Full piece is 64MiB
	fullPiecePadded := abi.PaddedPieceSize(64 << 20)
	fullPieceUnpadded := fullPiecePadded.Unpadded()

	// Generate full piece data
	fullData := make([]byte, fullPieceUnpadded)
	_, err := rand.Read(fullData)
	require.NoError(t, err)

	// Pad the full piece
	fullPadded := make([]byte, fullPiecePadded)
	fr32.Pad(fullData, fullPadded)

	// We want to read starting at 8MiB offset, for 4MiB
	// These must be fr32-chunk-aligned (127 byte boundaries for unpadded)
	startOffsetUnpadded := abi.UnpaddedPieceSize(8 << 20)   // 8MiB
	startOffsetUnpadded = (startOffsetUnpadded / 127) * 127 // Align to 127
	readSizeUnpadded := abi.UnpaddedPieceSize(4 << 20)      // 4MiB
	readSizeUnpadded = (readSizeUnpadded / 127) * 127       // Align to 127
	endOffsetUnpadded := startOffsetUnpadded + readSizeUnpadded

	startOffsetPadded := startOffsetUnpadded.Padded()
	endOffsetPadded := endOffsetUnpadded.Padded()
	rangePadded := endOffsetPadded - startOffsetPadded

	// Extract the padded range from the full padded data
	paddedRange := fullPadded[startOffsetPadded:endOffsetPadded]

	// Create reader with only the range data
	underlyingReader := bytes.NewReader(paddedRange)

	// BUG PATTERN: Create unpadReader with full piece size but only range available
	// Note: We use a valid piece size that encompasses our range
	declaredSize := abi.PaddedPieceSize(rangePadded)
	// Round up to valid piece size (power of 2)
	for declaredSize&(declaredSize-1) != 0 {
		declaredSize = declaredSize & (declaredSize - 1)
	}
	declaredSize *= 2
	if declaredSize < 128 {
		declaredSize = 128
	}

	// Try with declared size = full piece size (the bug pattern)
	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	// Read all available data
	result := make([]byte, 0, readSizeUnpadded)
	readBuf := make([]byte, 64*1024)

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Error after %d bytes: %v", len(result), err)
			break
		}
	}

	expectedData := fullData[startOffsetUnpadded:endOffsetUnpadded]

	require.Equal(t, len(expectedData), len(result),
		"Should read exactly %d bytes, got %d", len(expectedData), len(result))
	require.Equal(t, expectedData, result, "Data should match without corruption")
}

// TestUnpadReaderSizeMismatch_WorkBufferBoundary tests the specific case where
// the underlying data exhausts exactly at a work buffer boundary.
func TestUnpadReaderSizeMismatch_WorkBufferBoundary(t *testing.T) {
	// Use MTTresh (512KB) as a boundary point
	mtTresh := fr32.MTTresh

	// Full piece is larger than MTTresh
	fullPiecePadded := abi.PaddedPieceSize(16 << 20) // 16MiB

	// Actual available data is exactly MTTresh (512KB padded)
	actualPadded := abi.PaddedPieceSize(mtTresh)
	actualUnpadded := actualPadded.Unpadded()

	// Generate and pad data
	originalData := make([]byte, actualUnpadded)
	_, err := rand.Read(originalData)
	require.NoError(t, err)

	paddedData := make([]byte, actualPadded)
	fr32.Pad(originalData, paddedData)

	underlyingReader := bytes.NewReader(paddedData)

	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	// Read in chunks that don't align with MTTresh
	result := make([]byte, 0, actualUnpadded)
	readBuf := make([]byte, 100*1024) // 100KB reads

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Error after %d bytes: %v", len(result), err)
			break
		}
	}

	require.Equal(t, int(actualUnpadded), len(result),
		"Should read all %d bytes, got %d", actualUnpadded, len(result))
	require.Equal(t, originalData, result, "Data integrity check failed")
}

// TestUnpadReaderSizeMismatch_NonPowerOf2ActualSize tests when the actual
// available data is not a nice power of 2. This is closer to real-world
// scenarios where piece ranges might not align perfectly.
func TestUnpadReaderSizeMismatch_NonPowerOf2ActualSize(t *testing.T) {
	fullPiecePadded := abi.PaddedPieceSize(16 << 20)

	// Actual data is 100 chunks = 12800 bytes padded (not power of 2)
	numChunks := 100
	actualPadded := abi.PaddedPieceSize(numChunks * 128)
	actualUnpadded := abi.UnpaddedPieceSize(numChunks * 127)

	originalData := make([]byte, actualUnpadded)
	_, err := rand.Read(originalData)
	require.NoError(t, err)

	paddedData := make([]byte, actualPadded)
	fr32.Pad(originalData, paddedData)

	underlyingReader := bytes.NewReader(paddedData)

	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	result := make([]byte, 0, actualUnpadded)
	readBuf := make([]byte, 4096)

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Error after %d bytes: %v", len(result), err)
			break
		}
	}

	require.Equal(t, int(actualUnpadded), len(result),
		"Should read all %d bytes, got %d", actualUnpadded, len(result))
	require.Equal(t, originalData, result, "Data should match")
}

// TestUnpadReaderSizeMismatch_ExactByteLoss tests reading data and verifies
// no bytes are lost at boundaries. This test specifically checks the range
// around 8MiB (0x800000) where the original bug was observed.
func TestUnpadReaderSizeMismatch_ExactByteLoss(t *testing.T) {
	// Create a piece where we can detect exact byte loss
	fullPiecePadded := abi.PaddedPieceSize(32 << 20)

	// Actual data is just over 8MiB to cross the boundary
	targetBytes := uint64(8<<20) + 1024 // 8MiB + 1KB
	// Round to chunk boundary
	numChunks := (targetBytes + 126) / 127
	actualUnpadded := abi.UnpaddedPieceSize(numChunks * 127)
	actualPadded := actualUnpadded.Padded()

	// Create sequential data so we can detect any loss or duplication
	originalData := make([]byte, actualUnpadded)
	for i := range originalData {
		originalData[i] = byte(i & 0xFF)
	}

	paddedData := make([]byte, actualPadded)
	fr32.Pad(originalData, paddedData)

	underlyingReader := bytes.NewReader(paddedData)

	buf := make([]byte, fr32.BufSize(fullPiecePadded))
	unpadReader, err := fr32.NewUnpadReaderBuf(underlyingReader, fullPiecePadded, buf)
	require.NoError(t, err)

	result := make([]byte, 0, actualUnpadded)
	readBuf := make([]byte, 64*1024)

	for {
		n, err := unpadReader.Read(readBuf)
		if n > 0 {
			result = append(result, readBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("Error after %d bytes: %v", len(result), err)
			break
		}
	}

	// Check length
	require.Equal(t, int(actualUnpadded), len(result),
		"Byte count mismatch: expected %d, got %d", actualUnpadded, len(result))

	// Check data integrity byte by byte around 8MiB boundary
	boundaryOffset := 8 << 20
	checkStart := boundaryOffset - 256
	checkEnd := min(boundaryOffset+256, len(result))

	if len(result) >= checkEnd {
		for i := checkStart; i < checkEnd; i++ {
			expected := byte(i & 0xFF)
			if result[i] != expected {
				t.Errorf("Byte mismatch at offset %d (0x%x): got 0x%02x, expected 0x%02x",
					i, i, result[i], expected)
			}
		}
	}

	// Full comparison
	require.Equal(t, originalData, result, "Data integrity check failed")
}
