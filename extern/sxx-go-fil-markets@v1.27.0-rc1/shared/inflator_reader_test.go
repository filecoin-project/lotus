package shared

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

func TestInflatorReader(t *testing.T) {
	req := require.New(t)

	// Create a temp file
	f, err := os.CreateTemp(t.TempDir(), "buff")
	req.NoError(err)
	defer f.Close() // nolint

	// Store a sample string to the temp file
	sampleString := "Testing 123"
	n, err := f.WriteString(sampleString)
	req.NoError(err)
	req.Len(sampleString, n)

	// Seek to the start of the file
	_, err = f.Seek(0, io.SeekStart)
	req.NoError(err)

	// Create an inflator reader over the file
	paddedSize := 1024
	padded := abi.PaddedPieceSize(paddedSize)
	ir, err := NewInflatorReader(f, uint64(n), padded.Unpadded())
	req.NoError(err)

	// Read all bytes into a buffer
	buff := make([]byte, paddedSize)
	_, err = ir.Read(buff)
	req.NoError(err)

	// Check that the correct number of bytes was read
	req.Len(buff, paddedSize)
	// Check that the first part of the buffer matches the sample string
	req.Equal([]byte(sampleString), buff[:len(sampleString)])
	// Check that the rest of the buffer is zeros
	for _, b := range buff[len(sampleString):] {
		req.EqualValues(0, b)
	}

	// Seek to the start of the reader
	err = ir.SeekStart()
	req.NoError(err)

	// Verify that the reader returns the correct bytes, as above
	buff = make([]byte, paddedSize)
	_, err = ir.Read(buff)
	req.NoError(err)
	req.Len(buff, paddedSize)
	req.Equal([]byte(sampleString), buff[:len(sampleString)])
	for _, b := range buff[len(sampleString):] {
		req.EqualValues(0, b)
	}
}
