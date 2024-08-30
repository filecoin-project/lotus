package proofs

import (
	"math/bits"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-commp-utils/v2"
	"github.com/filecoin-project/go-commp-utils/v2/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
)

// GenerateUnsealedCID generates the UnsealedCID for a sector of size determined by the proofType
// containing pieces of the specified sizes. Where there is not one piece the size of the sector, or
// zero pieces (in which case one piece can be used), it fills in the remaining space with pieces of
// the minimum size required to pad the sector.
func GenerateUnsealedCID(proofType abi.RegisteredSealProof, pieces []abi.PieceInfo) (cid.Cid, error) {
	ssize, err := proofType.SectorSize()
	if err != nil {
		return cid.Undef, err
	}
	upssize := abi.PaddedPieceSize(ssize).Unpadded()

	if len(pieces) == 0 {
		return zerocomm.ZeroPieceCommitment(upssize), nil
	}

	pcid, psz, err := commp.PieceAggregateCommP(proofType, pieces)
	if err != nil {
		return cid.Undef, err
	}

	return commp.ZeroPadPieceCommitment(pcid, psz.Unpadded(), upssize)
}

func GetRequiredPadding(oldLength abi.PaddedPieceSize, newPieceLength abi.PaddedPieceSize) ([]abi.PaddedPieceSize, abi.PaddedPieceSize) {

	padPieces := make([]abi.PaddedPieceSize, 0)

	toFill := uint64(-oldLength % newPieceLength)

	n := bits.OnesCount64(toFill)
	var sum abi.PaddedPieceSize
	for i := 0; i < n; i++ {
		next := bits.TrailingZeros64(toFill)
		psize := uint64(1) << uint(next)
		toFill ^= psize

		padded := abi.PaddedPieceSize(psize)
		padPieces = append(padPieces, padded)
		sum += padded
	}

	return padPieces, sum
}
