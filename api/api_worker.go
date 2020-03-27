package api

import (
	"context"

	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/sector-storage"
	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/stores"
)

type WorkerApi interface {
	Version(context.Context) (build.Version, error)
	// TODO: Info() (name, ...) ?

	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) // TaskType -> Weight
	Paths(context.Context) ([]stores.StoragePath, error)
	Info(context.Context) (sectorstorage.WorkerInfo, error)

	storage.Sealer
}
