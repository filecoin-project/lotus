package api

import (
	"context"

	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/storage/sealmgr"
)

type WorkerApi interface {
	Version(context.Context) (build.Version, error)
	// TODO: Info() (name, ...) ?

	TaskTypes(context.Context) (map[sealmgr.TaskType]struct{}, error) // TaskType -> Weight

	storage.Sealer
}
