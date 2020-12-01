package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"

	"github.com/filecoin-project/lotus/build"
)

type WorkerAPI interface {
	Version(context.Context) (build.Version, error)
	// TODO: Info() (name, ...) ?

	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) // TaskType -> Weight
	Paths(context.Context) ([]stores.StoragePath, error)
	Info(context.Context) (storiface.WorkerInfo, error)

	storiface.WorkerCalls

	TaskDisable(ctx context.Context, tt sealtasks.TaskType) error
	TaskEnable(ctx context.Context, tt sealtasks.TaskType) error

	// Storage / Other
	Remove(ctx context.Context, sector abi.SectorID) error

	StorageAddLocal(ctx context.Context, path string) error

	// SetEnabled marks the worker as enabled/disabled. Not that this setting
	// may take a few seconds to propagate to task scheduler
	SetEnabled(ctx context.Context, enabled bool) error

	Enabled(ctx context.Context) (bool, error)

	// WaitQuiet blocks until there are no tasks running
	WaitQuiet(ctx context.Context) error

	// returns a random UUID of worker session, generated randomly when worker
	// process starts
	ProcessSession(context.Context) (uuid.UUID, error)

	// Like ProcessSession, but returns an error when worker is disabled
	Session(context.Context) (uuid.UUID, error)
}
