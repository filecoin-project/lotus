package fr32_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/fr32"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
)

func TestWriteTwoPcs(t *testing.T) {
	tf, _ := os.CreateTemp("/tmp/", "scrb-")

	paddedSize := abi.PaddedPieceSize(16 << 20)
	n := 2

	var rawBytes []byte

	for i := 0; i < n; i++ {
		buf := bytes.Repeat([]byte{0xab * byte(i)}, int(paddedSize.Unpadded()))
		rawBytes = append(rawBytes, buf...)

		rf, w, _ := mock.ToReadableFile(bytes.NewReader(buf), int64(len(buf)))

		_, _, _, err := ffi.WriteWithAlignment(abi.RegisteredSealProof_StackedDrg32GiBV1, rf, abi.UnpaddedPieceSize(len(buf)), tf, nil)
		if err != nil {
			panic(err)
		}
		if err := w(); err != nil {
			panic(err)
		}
	}

	if _, err := tf.Seek(io.SeekStart, 0); err != nil { // nolint:staticcheck
		panic(err)
	}

	ffiBytes, err := io.ReadAll(tf)
	if err != nil {
		panic(err)
	}

	if err := tf.Close(); err != nil {
		panic(err)
	}

	if err := os.Remove(tf.Name()); err != nil {
		panic(err)
	}

	outBytes := make([]byte, int(paddedSize)*n)
	fr32.Pad(rawBytes, outBytes)
	require.Equal(t, ffiBytes, outBytes)

	unpadBytes := make([]byte, int(paddedSize.Unpadded())*n)
	fr32.Unpad(ffiBytes, unpadBytes)
	require.Equal(t, rawBytes, unpadBytes)
}
