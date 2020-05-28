package fr32

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
)

func TestWriteTwoPcs(t *testing.T) {
	tf, _ := ioutil.TempFile("/tmp/", "scrb-")

	paddedSize := abi.PaddedPieceSize(16 << 20)
	n := 2

	var rawBytes []byte

	for i := 0; i < n; i++ {
		buf := bytes.Repeat([]byte{0xab * byte(i)}, int(paddedSize.Unpadded()))
		rawBytes = append(rawBytes, buf...)

		rf, w, _ := ffiwrapper.ToReadableFile(bytes.NewReader(buf), int64(len(buf)))

		_, _, _, err := ffi.WriteWithAlignment(abi.RegisteredProof_StackedDRG32GiBSeal, rf, abi.UnpaddedPieceSize(len(buf)), tf, nil)
		if err != nil {
			panic(err)
		}
		if err := w(); err != nil {
			panic(err)
		}
	}

	if _, err := tf.Seek(io.SeekStart, 0); err != nil {
		panic(err)
	}

	ffiBytes, err := ioutil.ReadAll(tf)
	if err != nil {
		panic(err)
	}

	if err := tf.Close(); err != nil {
		panic(err)
	}

	if err := os.Remove(tf.Name()); err != nil {
		panic(err)
	}

	outBytes := make([]byte, int(paddedSize) * n)
	Pad(rawBytes, outBytes)
	require.Equal(t, ffiBytes, outBytes)

	unpadBytes := make([]byte, int(paddedSize.Unpadded()) * n)
	Unpad(ffiBytes, unpadBytes)
	require.Equal(t, rawBytes, unpadBytes)
}

