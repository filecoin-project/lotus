package sectorstorage

import (
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/stores"
)

var FSOverheadSeal = map[stores.SectorFileType]int{ // 10x overheads
	stores.FTUnsealed: 10,
	stores.FTSealed:   10,
	stores.FTCache:    70, // TODO: confirm for 32G
}

var FsOverheadFinalized = map[stores.SectorFileType]int{
	stores.FTUnsealed: 10,
	stores.FTSealed:   10,
	stores.FTCache:    2,
}

type Resources struct {
	MinMemory uint64 // What Must be in RAM for decent perf
	MaxMemory uint64 // Memory required (swap + ram)

	MultiThread bool
	CanGPU      bool

	BaseMinMemory uint64 // What Must be in RAM for decent perf (shared between threads)
}

const MaxCachingOverhead = 32 << 30

var ResourceTable = map[sealtasks.TaskType]map[abi.RegisteredProof]Resources{
	sealtasks.TTAddPiece: {
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{ // This is probably a bit conservative
			MaxMemory: 32 << 30,
			MinMemory: 32 << 30,

			MultiThread: false,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MultiThread: false,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MultiThread: false,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 2 << 22,
			MinMemory: 2 << 22,

			MultiThread: false,

			BaseMinMemory: 2 << 22,
		},
	},
	sealtasks.TTPreCommit1: {
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 64 << 30,
			MinMemory: 32 << 30,

			MultiThread: false,

			BaseMinMemory: 30 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			MultiThread: false,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MultiThread: false,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 2 << 22,
			MinMemory: 2 << 22,

			MultiThread: false,

			BaseMinMemory: 2 << 22,
		},
	},
	sealtasks.TTPreCommit2: {
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 96 << 30,
			MinMemory: 64 << 30,

			MultiThread: true,

			BaseMinMemory: 30 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			MultiThread: true,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MultiThread: true,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 2 << 22,
			MinMemory: 2 << 22,

			MultiThread: true,

			BaseMinMemory: 2 << 22,
		},
	},
	sealtasks.TTCommit1: { // Very short (~100ms), so params are very light
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MultiThread: false,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 1 << 30,
			MinMemory: 1 << 30,

			MultiThread: false,

			BaseMinMemory: 1 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MultiThread: false,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 2 << 22,
			MinMemory: 2 << 22,

			MultiThread: false,

			BaseMinMemory: 2 << 22,
		},
	},
	sealtasks.TTCommit2: { // TODO: Measure more accurately
		abi.RegisteredProof_StackedDRG32GiBSeal: Resources{
			MaxMemory: 110 << 30,
			MinMemory: 60 << 30,

			MultiThread: true,
			CanGPU:      true,

			BaseMinMemory: 64 << 30, // params
		},
		abi.RegisteredProof_StackedDRG512MiBSeal: Resources{
			MaxMemory: 3 << 29, // 1.5G
			MinMemory: 1 << 30,

			MultiThread: false, // This is fine
			CanGPU:      true,

			BaseMinMemory: 10 << 30,
		},
		abi.RegisteredProof_StackedDRG2KiBSeal: Resources{
			MaxMemory: 2 << 10,
			MinMemory: 2 << 10,

			MultiThread: false,
			CanGPU: true,

			BaseMinMemory: 2 << 10,
		},
		abi.RegisteredProof_StackedDRG8MiBSeal: Resources{
			MaxMemory: 2 << 22,
			MinMemory: 2 << 22,

			MultiThread: false,
			CanGPU: true,

			BaseMinMemory: 2 << 22,
		},
	},
}
