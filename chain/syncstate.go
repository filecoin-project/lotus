package chain

import (
	"fmt"
	"sync"

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
	default:
		return fmt.Sprintf("<unknown: %d>", v)
	}
}

type SyncerState struct {
	lk     sync.Mutex
	Target *types.TipSet
	Base   *types.TipSet
	Stage  api.SyncStateStage
	Height uint64
}

func (ss *SyncerState) SetStage(v api.SyncStateStage) {
	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Stage = v
}

func (ss *SyncerState) Init(base, target *types.TipSet) {
	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Target = target
	ss.Base = base
	ss.Stage = api.StageHeaders
	ss.Height = 0
}

func (ss *SyncerState) SetHeight(h uint64) {
	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Height = h
}

func (ss *SyncerState) Snapshot() SyncerState {
	ss.lk.Lock()
	defer ss.lk.Unlock()
	return SyncerState{
		Base:   ss.Base,
		Target: ss.Target,
		Stage:  ss.Stage,
		Height: ss.Height,
	}
}
