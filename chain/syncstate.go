package chain

import (
	"fmt"
	"sync"

	"github.com/filecoin-project/go-lotus/chain/types"
)

const (
	StageIdle = iota
	StageHeaders
	StagePersistHeaders
	StageMessages
	StageSyncComplete
)

func SyncStageString(v int) string {
	switch v {
	case StageHeaders:
		return "header sync"
	case StagePersistHeaders:
		return "persisting headers"
	case StageMessages:
		return "message sync"
	case StageSyncComplete:
		return "complete"
	default:
		return fmt.Sprintf("<unknown: %d>", v)
	}
}

type SyncerState struct {
	lk     sync.Mutex
	Target *types.TipSet
	Base   *types.TipSet
	Stage  int
	Height uint64
}

func (ss *SyncerState) SetStage(v int) {
	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Stage = v
}

func (ss *SyncerState) Init(base, target *types.TipSet) {
	ss.lk.Lock()
	defer ss.lk.Unlock()
	ss.Target = target
	ss.Base = base
	ss.Stage = StageHeaders
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
