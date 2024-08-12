package proofs_test

import (
	"bytes"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	ffi "github.com/filecoin-project/filecoin-ffi"
	commpffi "github.com/filecoin-project/go-commp-utils/ffiwrapper"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/proofs"
)

func TestGenerateUnsealedCID(t *testing.T) {
	pt := abi.RegisteredSealProof_StackedDrg2KiBV1
	ups := int(abi.PaddedPieceSize(2048).Unpadded())

	commP := func(b []byte) cid.Cid {
		pf, werr, err := commpffi.ToReadableFile(bytes.NewReader(b), int64(len(b)))
		require.NoError(t, err)

		c, err := ffi.GeneratePieceCIDFromFile(pt, pf, abi.UnpaddedPieceSize(len(b)))
		require.NoError(t, err)

		require.NoError(t, werr())

		return c
	}

	testCommEq := func(name string, in [][]byte, expect [][]byte) {
		t.Run(name, func(t *testing.T) {
			upi := make([]abi.PieceInfo, len(in))
			for i, b := range in {
				upi[i] = abi.PieceInfo{
					Size:     abi.UnpaddedPieceSize(len(b)).Padded(),
					PieceCID: commP(b),
				}
			}

			sectorPi := []abi.PieceInfo{
				{
					Size:     2048,
					PieceCID: commP(bytes.Join(expect, nil)),
				},
			}

			expectCid, err := proofs.GenerateUnsealedCID(pt, sectorPi)
			require.NoError(t, err)

			actualCid, err := proofs.GenerateUnsealedCID(pt, upi)
			require.NoError(t, err)

			require.Equal(t, expectCid, actualCid)
		})
	}

	barr := func(b byte, den int) []byte {
		return bytes.Repeat([]byte{b}, ups/den)
	}

	// 0000
	testCommEq("zero",
		nil,
		[][]byte{barr(0, 1)},
	)

	// 1111
	testCommEq("one",
		[][]byte{barr(1, 1)},
		[][]byte{barr(1, 1)},
	)

	// 11 00
	testCommEq("one|2",
		[][]byte{barr(1, 2)},
		[][]byte{barr(1, 2), barr(0, 2)},
	)

	// 1 0 00
	testCommEq("one|4",
		[][]byte{barr(1, 4)},
		[][]byte{barr(1, 4), barr(0, 4), barr(0, 2)},
	)

	// 11 2 0
	testCommEq("one|2-two|4",
		[][]byte{barr(1, 2), barr(2, 4)},
		[][]byte{barr(1, 2), barr(2, 4), barr(0, 4)},
	)

	// 1 0 22
	testCommEq("one|4-two|2",
		[][]byte{barr(1, 4), barr(2, 2)},
		[][]byte{barr(1, 4), barr(0, 4), barr(2, 2)},
	)

	// 1 0 22 0000
	testCommEq("one|8-two|4",
		[][]byte{barr(1, 8), barr(2, 4)},
		[][]byte{barr(1, 8), barr(0, 8), barr(2, 4), barr(0, 2)},
	)

	// 11 2 0 0000
	testCommEq("one|4-two|8",
		[][]byte{barr(1, 4), barr(2, 8)},
		[][]byte{barr(1, 4), barr(2, 8), barr(0, 8), barr(0, 2)},
	)

	// 1 0 22 3 0 00 4444 5 0 00
	testCommEq("one|16-two|8-three|16-four|4-five|16",
		[][]byte{barr(1, 16), barr(2, 8), barr(3, 16), barr(4, 4), barr(5, 16)},
		[][]byte{barr(1, 16), barr(0, 16), barr(2, 8), barr(3, 16), barr(0, 16), barr(0, 8), barr(4, 4), barr(5, 16), barr(0, 16), barr(0, 8)},
	)
}
