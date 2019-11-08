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

func (m *Miner) ListSectors() ([]SectorInfo, error) {
	var sectors []SectorInfo
	if err := m.sectors.List(&sectors); err != nil {
		return nil, err
	}
	return sectors, nil
}

func (m *Miner) GetSectorInfo(sid uint64) (SectorInfo, error) {
	var out SectorInfo
	err := m.sectors.Get(sid, &out)
	return out, err
}
