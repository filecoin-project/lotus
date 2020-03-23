package api

import (
	"context"

	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/storage/sectorstorage/sealtasks"
	"github.com/filecoin-project/lotus/storage/sectorstorage/stores"
)

type WorkerApi interface {
	Version(context.Context) (build.Version, error)
	// TODO: Info() (name, ...) ?

	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) // TaskType -> Weight
	Paths(context.Context) ([]stores.StoragePath, error)
	Info(context.Context) (WorkerInfo, error)

	storage.Sealer
}

type WorkerResources struct {
	MemPhysical uint64
	MemSwap     uint64

	MemReserved uint64 // Used by system / other processes

	GPUs []string
}

type WorkerInfo struct {
	Hostname string

	Resources WorkerResources
}
