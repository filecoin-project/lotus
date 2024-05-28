package sealing

import (
	"context"
	"sync"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

type statSectorState int

const (
	sstStaging statSectorState = iota
	sstSealing
	sstFailed
	sstProving
	nsst
)

type SectorStats struct {
	lk sync.Mutex

	bySector map[abi.SectorID]SectorState
	byState  map[SectorState]int64
	totals   [nsst]uint64
}

func (ss *SectorStats) updateSector(ctx context.Context, cfg sealiface.Config, id abi.SectorID, st SectorState) (updateInput bool) {
	ss.lk.Lock()
	defer ss.lk.Unlock()

	preSealing := ss.curSealingLocked()
	preStaging := ss.curStagingLocked()

	// update totals
	oldst, found := ss.bySector[id]
	if found {
		ss.totals[toStatState(oldst, cfg.FinalizeEarly)]--
		ss.byState[oldst]--

		if ss.byState[oldst] <= 0 {
			delete(ss.byState, oldst)
		}

		mctx, _ := tag.New(ctx, tag.Upsert(metrics.SectorState, string(oldst)))
		stats.Record(mctx, metrics.SectorStates.M(ss.byState[oldst]))
	}

	sst := toStatState(st, cfg.FinalizeEarly)
	ss.bySector[id] = st
	ss.totals[sst]++
	ss.byState[st]++

	mctx, _ := tag.New(ctx, tag.Upsert(metrics.SectorState, string(st)))
	stats.Record(mctx, metrics.SectorStates.M(ss.byState[st]))

	// check if we may need be able to process more deals
	sealing := ss.curSealingLocked()
	staging := ss.curStagingLocked()

	log.Debugw("sector stats", "sealing", sealing, "staging", staging)

	if cfg.MaxSealingSectorsForDeals > 0 && // max sealing deal sector limit set
		preSealing >= cfg.MaxSealingSectorsForDeals && // we were over limit
		sealing < cfg.MaxSealingSectorsForDeals { // and we're below the limit now
		updateInput = true
	}

	if cfg.MaxWaitDealsSectors > 0 && // max waiting deal sector limit set
		preStaging >= cfg.MaxWaitDealsSectors && // we were over limit
		staging < cfg.MaxWaitDealsSectors { // and we're below the limit now
		updateInput = true
	}

	return updateInput
}

func (ss *SectorStats) curSealingLocked() uint64 {
	return ss.totals[sstStaging] + ss.totals[sstSealing] + ss.totals[sstFailed]
}

func (ss *SectorStats) curStagingLocked() uint64 {
	return ss.totals[sstStaging]
}

// return the number of sectors currently in the sealing pipeline
func (ss *SectorStats) curSealing() uint64 {
	ss.lk.Lock()
	defer ss.lk.Unlock()

	return ss.curSealingLocked()
}

// return the number of sectors waiting to enter the sealing pipeline
func (ss *SectorStats) curStaging() uint64 {
	ss.lk.Lock()
	defer ss.lk.Unlock()

	return ss.curStagingLocked()
}
