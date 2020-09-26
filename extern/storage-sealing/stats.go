package sealing

import (
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
)

type statSectorState int

const (
	sstSealing statSectorState = iota
	sstFailed
	sstProving
	nsst
)

type SectorStats struct {
	lk sync.Mutex

	bySector map[abi.SectorID]statSectorState
	totals   [nsst]uint64
}

func (ss *SectorStats) updateSector(id abi.SectorID, st SectorState) {
	ss.lk.Lock()
	defer ss.lk.Unlock()

	oldst, found := ss.bySector[id]
	if found {
		ss.totals[oldst]--
	}

	sst := toStatState(st)
	ss.bySector[id] = sst
	ss.totals[sst]++
}

// return the number of sectors currently in the sealing pipeline
func (ss *SectorStats) curSealing() uint64 {
	ss.lk.Lock()
	defer ss.lk.Unlock()

	return ss.totals[sstSealing] + ss.totals[sstFailed]
}
