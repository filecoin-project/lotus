package sealing

import (
	"context"
	"fmt"
	"math/bits"
	"sort"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
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

	// internal
	sstLateSeal
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

func (m *Sealing) PipelineStats(ctx context.Context) (api.PipelineStats, error) {
	cfg, err := m.getConfig()
	if err != nil {
		return api.PipelineStats{}, err
	}

	out := api.PipelineStats{
		SectorsStaging: m.stats.curStaging(),
		SectorsSealing: m.stats.curSealing(),

		OpenCapacityEstimate: map[string]uint64{},

		MaxWaitDealsSectors:       cfg.MaxWaitDealsSectors,
		MaxSealingSectors:         cfg.MaxSealingSectors,
		MaxSealingSectorsForDeals: cfg.MaxSealingSectorsForDeals,
		PreferNewSectorsForDeals:  cfg.PreferNewSectorsForDeals,
		MaxUpgradingSectors:       cfg.MaxUpgradingSectors,
		FinalizeEarly:             cfg.FinalizeEarly,
		MakeNewSectorForDeals:     cfg.MakeNewSectorForDeals,
	}

	m.inputLk.Lock()

	for _, sector := range m.openSectors {
		maxPiece := abi.PaddedPieceSize(1<<bits.LeadingZeros64(uint64(sector.used.Padded())) - 1).Unpadded()
		out.OpenCapacity += uint64(maxPiece)
		out.OpenSectors = append(out.OpenSectors, uint64(maxPiece))
	}

	m.inputLk.Unlock()

	spt, err := m.currentSealProof(ctx)
	if err != nil {
		return api.PipelineStats{}, err
	}
	ssizeRaw, err := spt.SectorSize()
	if err != nil {
		return api.PipelineStats{}, err
	}
	ssize := uint64(abi.PaddedPieceSize(ssizeRaw).Unpadded())

	// account for unused sealing sector limits in OpenCapacity
	maxDealSectors := cfg.MaxSealingSectorsForDeals
	if cfg.MaxUpgradingSectors > cfg.MaxSealingSectorsForDeals {
		maxDealSectors = cfg.MaxUpgradingSectors
	}
	maxDealSectors -= out.SectorsSealing
	if maxDealSectors > 0 {
		out.OpenCapacity += maxDealSectors * ssize
	}

	// calculate estimated future capacity

	m.stats.lk.Lock()

	doneIn := map[time.Duration]int64{}
	for state, n := range m.stats.byState {
		durEst := ValidSectorStateList[state].sealDurationEstimate
		if durEst > 0 {
			continue
		}

		doneIn[durEst] += n
	}

	m.stats.lk.Unlock()

	type estimateBucket struct {
		in time.Duration
		n  int64
	}

	doneList := make([]estimateBucket, 0, len(doneIn))
	for duration, n := range doneIn {
		doneList = append(doneList, estimateBucket{in: duration, n: n})
	}

	sort.Slice(doneList, func(i, j int) bool {
		return doneList[i].in < doneList[j].in
	})

	var doneSectors uint64
	for i := range doneList {
		doneSectors += uint64(doneList[i].n)

		out.OpenCapacityEstimate[fmt.Sprint(time.Now().Add(doneList[i].in).Unix())] = (ssize * doneSectors) + out.OpenCapacity
	}

	return out, err
}
