package sectorstorage

import (
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/storage/sectorstorage/sealtasks"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

var FSOverheadSeal = map[sectorbuilder.SectorFileType]int{ // 10x overheads
	sectorbuilder.FTUnsealed: 10,
	sectorbuilder.FTSealed:   10,
	sectorbuilder.FTCache:    70, // TODO: confirm for 32G
}

var FsOverheadFinalized = map[sectorbuilder.SectorFileType]int{
	sectorbuilder.FTUnsealed: 10,
	sectorbuilder.FTSealed:   10,
	sectorbuilder.FTCache:    2,
}

type Resources struct {
	MinMemory uint64 // What Must be in RAM for decent perf
	MaxMemory uint64 // Mamory required (swap + ram)

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
	},
}

func init() {
	// for now we just reuse params for 2kib and 8mib from 512mib

	for taskType := range ResourceTable {
		ResourceTable[taskType][abi.RegisteredProof_StackedDRG8MiBSeal] = ResourceTable[taskType][abi.RegisteredProof_StackedDRG512MiBSeal]
		ResourceTable[taskType][abi.RegisteredProof_StackedDRG2KiBSeal] = ResourceTable[taskType][abi.RegisteredProof_StackedDRG512MiBSeal]
	}
}
