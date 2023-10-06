package wdpost

import (
	"context"

	"github.com/samber/lo"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type WindowPostTaskHandler struct {
	max                     int // TODO read from Flags
	*harmonytask.TaskEngine     // TODO populate at setup time
	Chain                   full.ChainModuleAPI
}

func New(chain full.ChainModuleAPI) *WindowPostTaskHandler {
	// TODO

	return &WindowPostTaskHandler{
		Chain: chain,
	}
}

func (wp *WindowPostTaskHandler) CanAccept(tids []harmonytask.TaskID) (*harmonytask.TaskID, error) {
	// GetEpoch
	ts, err := wp.Chain.ChainHead(context.Background())
	if err != nil {
		return nil, err
	}
	// TODO GetDeadline Epochs for tasks
	type wdTaskDef struct {
		abi.RegisteredSealProof
	}
	var tasks []wdTaskDef
	// TODO accept those past deadline, then do the right thing in Do()
	// TODO Exit nil if no disk available?
	// Discard those too big for our free RAM
	freeRAM := wp.TaskEngine.ResourcesAvailable().Ram
	tasks = lo.Filter(tasks, func(d wdTaskDef, _ int) bool {
		return res[d.RegisteredSealProof].MaxMemory <= freeRAM
	})
	// TODO If Local Disk, discard others
	// TODO If Shared Disk entries, discard others
	// TODO Select the one closest to the deadline

	// FUTURE: Be less greedy: let the best machine do the work.
	// FUTURE: balance avoiding 2nd retries (3rd run)
	return nil, nil
}

var res = storiface.ResourceTable[sealtasks.TTGenerateWindowPoSt]

func (wp *WindowPostTaskHandler) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Name:        "WdPost",
		Max:         wp.max,
		MaxFailures: 3,
		Follows:     nil,
		Cost: resources.Resources{
			Cpu: 1,
			Gpu: 1,
			// RAM of smallest proof's max is listed here
			Ram: lo.Reduce(lo.Keys(res), func(i uint64, k abi.RegisteredSealProof, _ int) uint64 {
				if res[k].MaxMemory < i {
					return res[k].MaxMemory
				}
				return i
			}, 1<<63),
		},
	}
}
