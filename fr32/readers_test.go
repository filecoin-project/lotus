package fr32_test

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/fr32"
)

func TestPadReader(t *testing.T) {
	ps := abi.PaddedPieceSize(64 << 20).Unpadded()

	raw := bytes.Repeat([]byte{0x55}, int(ps))

	r, err := fr32.NewPadReader(bytes.NewReader(raw), ps)
	if err != nil {
		t.Fatal(err)
	}

	readerPadded, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	padOut := make([]byte, ps.Padded())
	fr32.Pad(raw, padOut)

	require.Equal(t, padOut, readerPadded)
}

func TestUnpadReader(t *testing.T) {
	ps := abi.PaddedPieceSize(64 << 20).Unpadded()

	raw := bytes.Repeat([]byte{0x77}, int(ps))

	padOut := make([]byte, ps.Padded())
	fr32.Pad(raw, padOut)

	r, err := fr32.NewUnpadReader(bytes.NewReader(padOut), ps.Padded())
	if err != nil {
		t.Fatal(err)
	}

	readered, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, raw, readered)
}
