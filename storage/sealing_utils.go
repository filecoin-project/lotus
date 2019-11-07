package storage

import (
	"math/bits"

	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

func fillersFromRem(toFill uint64) ([]uint64, error) {
	toFill += toFill / 127 // convert to in-sector bytes for easier math

	out := make([]uint64, bits.OnesCount64(toFill))
	for i := range out {
		next := bits.TrailingZeros64(toFill)
		psize := uint64(1) << next
		toFill ^= psize
		out[i] = sectorbuilder.UserBytesForSectorSize(psize)
	}
	return out, nil
}
