package sectorstorage

import (
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/sealtasks"
)

type Resources struct {
	MinMemory uint64 // What Must be in RAM for decent perf
	MaxMemory uint64 // Memory required (swap + ram)

	Threads int // -1 = multithread
	CanGPU  bool

	BaseMinMemory uint64 // What Must be in RAM for decent perf (shared between threads)
}

func (r Resources) MultiThread() bool {
	return r.Threads == -1
}

const MaxCachingOverhead = 32 << 30

var ResourceTable = map[sealtasks.TaskType]map[abi.RegisteredProof]Resources{
	sealtasks.TTAddPiece: {
		abi.RegisteredProof_StackedDRG64GiBSeal: Resources{ // This is probably a bit conservative
			MaxMemory: 64 << 30,
			MinMemory: 64 << 30,

			Threads: 1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{ // This is probably a bit conservative
			MaxMemory: 32 << 30,
			MinMemory: 32 << 30,

			Threads: 1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			Threads: 1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			Threads: 1,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			Threads: 1,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTPreCommit1: {
		abi.RegisteredProof_StackedDRG64GiBSeal: Resources{
			MaxMemory: 128 << 30,
			MinMemory: 96 << 30,

			Threads: 1,

			BaseMinMemory: 60 << 30,
		},
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 64 << 30,
			MinMemory: 48 << 30,

			Threads: 1,

			BaseMinMemory: 30 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			Threads: 1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			Threads: 1,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			Threads: 1,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTPreCommit2: {
		abi.RegisteredProof_StackedDRG64GiBSeal: Resources{
			MaxMemory: 64 << 30,
			MinMemory: 64 << 30,

			Threads: -1,
			CanGPU:  true,

			BaseMinMemory: 60 << 30,
		},
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 32 << 30,
			MinMemory: 32 << 30,

			Threads: -1,
			CanGPU:  true,

			BaseMinMemory: 30 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			Threads: -1,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			Threads: -1,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			Threads: -1,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTCommit1: { // Very short (~100ms), so params are very light
		abi.RegisteredProof_StackedDRG64GiBSeal: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			Threads: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			Threads: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			Threads: 0,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			Threads: 0,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			Threads: 0,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTCommit2: {
		abi.RegisteredProof_StackedDRG64GiBSeal: Resources{
			MaxMemory: 260 << 30, // TODO: Confirm
			MinMemory: 60 << 30,

			Threads: -1,
			CanGPU:  true,

			BaseMinMemory: 64 << 30, // params
		},
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 150 << 30, // TODO: ~30G of this should really be BaseMaxMemory
			MinMemory: 30 << 30,

			Threads: -1,
			CanGPU:  true,

			BaseMinMemory: 32 << 30, // params
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			Threads: 1, // This is fine
			CanGPU:  true,

			BaseMinMemory: 10 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			Threads: 1,
			CanGPU:  true,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 8 << 20,
			MinMemory: 8 << 20,

			Threads: 1,
			CanGPU:  true,

			BaseMinMemory: 8 << 20,
		},
	},
	sealtasks.TTFetch: {
		abi.RegisteredProof_StackedDRG64GiBSeal: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			Threads: 0,
			CanGPU:  false,

			BaseMinMemory: 0,
		},
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			Threads: 0,
			CanGPU:  false,

			BaseMinMemory: 0,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			Threads: 0,
			CanGPU:  false,

			BaseMinMemory: 0,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			Threads: 0,
			CanGPU:  false,

			BaseMinMemory: 0,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 1 << 20,
			MinMemory: 1 << 20,

			Threads: 0,
			CanGPU:  false,

			BaseMinMemory: 0,
		},
	},
}
