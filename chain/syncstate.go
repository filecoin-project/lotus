package chain

import (
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

func SyncStageString(v api.SyncStateStage) string {
	switch v {
	case api.StageHeaders:
		return "header sync"
	case api.StagePersistHeaders:
		return "persisting headers"
	case api.StageMessages:
		return "message sync"
	case api.StageSyncComplete:
		return "complete"
	case api.StageSyncErrored:
		return "error"
	default:
		return fmt.Sprintf("<unknown: %d>", v)
	}
}

type SyncerState struct {
	lk      sync.Mutex
	Target  *types.TipSet
	Base    *types.TipSet
	Stage   api.SyncStateStage
	Height  abi.ChainEpoch
	Message string
	Start   time.Time
	End     time.Time
}

func (ss *SyncerState) SetStage(v api.SyncStateStage) {
	if ss == nil {
		return
	}

	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Stage = v
	if v == api.StageSyncComplete {
		ss.End = time.Now()
	}
}

func (ss *SyncerState) Init(base, target *types.TipSet) {
	if ss == nil {
		return
	}

	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Target = target
	ss.Base = base
	ss.Stage = api.StageHeaders
	ss.Height = 0
	ss.Message = ""
	ss.Start = time.Now()
	ss.End = time.Time{}
}

func (ss *SyncerState) SetHeight(h abi.ChainEpoch) {
	if ss == nil {
		return
	}

	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Height = h
}

func (ss *SyncerState) Error(err error) {
	if ss == nil {
		return
	}

	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Message = err.Error()
	ss.Stage = api.StageSyncErrored
	ss.End = time.Now()
}

func (ss *SyncerState) Snapshot() SyncerState {
	ss.lk.Lock()
	defer ss.lk.Unlock()
	return SyncerState{
		Base:    ss.Base,
		Target:  ss.Target,
		Stage:   ss.Stage,
		Height:  ss.Height,
		Message: ss.Message,
		Start:   ss.Start,
		End:     ss.End,
	}
}
