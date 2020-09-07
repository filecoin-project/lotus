package sectorstorage

import (
	"context"
	"io"
	"os"
	"runtime"

	"github.com/elastic/go-sysinfo"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/specs-actors/actors/abi"
	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

var pathTypes = []storiface.SectorFileType{storiface.FTUnsealed, storiface.FTSealed, storiface.FTCache}

type WorkerConfig struct {
	SealProof abi.RegisteredSealProof
	TaskTypes []sealtasks.TaskType
}

type LocalWorker struct {
	scfg       *ffiwrapper.Config
	storage    stores.Store
	localStore *stores.Local
	sindex     stores.SectorIndex
	ret        storiface.WorkerReturn

	acceptTasks map[sealtasks.TaskType]struct{}
}

func NewLocalWorker(wcfg WorkerConfig, store stores.Store, local *stores.Local, sindex stores.SectorIndex, ret storiface.WorkerReturn) *LocalWorker {
	acceptTasks := map[sealtasks.TaskType]struct{}{}
	for _, taskType := range wcfg.TaskTypes {
		acceptTasks[taskType] = struct{}{}
	}

	return &LocalWorker{
		scfg: &ffiwrapper.Config{
			SealProofType: wcfg.SealProof,
		},
		storage:    store,
		localStore: local,
		sindex:     sindex,
		ret:        ret,

		acceptTasks: acceptTasks,
	}
}

type localWorkerPathProvider struct {
	w  *LocalWorker
	op storiface.AcquireMode
}

func (l *localWorkerPathProvider) AcquireSector(ctx context.Context, sector abi.SectorID, existing storiface.SectorFileType, allocate storiface.SectorFileType, sealing storiface.PathType) (storiface.SectorPaths, func(), error) {

	paths, storageIDs, err := l.w.storage.AcquireSector(ctx, sector, l.w.scfg.SealProofType, existing, allocate, sealing, l.op)
	if err != nil {
		return storiface.SectorPaths{}, nil, err
	}

	releaseStorage, err := l.w.localStore.Reserve(ctx, sector, l.w.scfg.SealProofType, allocate, storageIDs, storiface.FSOverheadSeal)
	if err != nil {
		return storiface.SectorPaths{}, nil, xerrors.Errorf("reserving storage space: %w", err)
	}

	log.Debugf("acquired sector %d (e:%d; a:%d): %v", sector, existing, allocate, paths)

	return paths, func() {
		releaseStorage()

		for _, fileType := range pathTypes {
			if fileType&allocate == 0 {
				continue
			}

			sid := storiface.PathByType(storageIDs, fileType)

			if err := l.w.sindex.StorageDeclareSector(ctx, stores.ID(sid), sector, fileType, l.op == storiface.AcquireMove); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}, nil
}

func (l *LocalWorker) sb() (ffiwrapper.Storage, error) {
	return ffiwrapper.New(&localWorkerPathProvider{w: l}, l.scfg)
}

func (l *LocalWorker) asyncCall(sector abi.SectorID, work func(ci storiface.CallID)) (storiface.CallID, error) {
	ci := storiface.CallID{
		Sector: sector,
		ID:     uuid.New(),
	}

	go work(ci)

	return ci, nil
}

func errstr(err error) string {
	if err != nil {
		return err.Error()
	}

	return ""
}

func (l *LocalWorker) NewSector(ctx context.Context, sector abi.SectorID) error {
	sb, err := l.sb()
	if err != nil {
		return err
	}

	return sb.NewSector(ctx, sector)
}

func (l *LocalWorker) AddPiece(ctx context.Context, sector abi.SectorID, epcs []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (storiface.CallID, error) {
	sb, err := l.sb()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(sector, func(ci storiface.CallID) {
		pi, err := sb.AddPiece(ctx, sector, epcs, sz, r)

		if err := l.ret.ReturnAddPiece(ctx, ci, pi, errstr(err)); err != nil {
			log.Errorf("ReturnAddPiece: %+v", err)
		}
	})
}

func (l *LocalWorker) Fetch(ctx context.Context, sector abi.SectorID, fileType storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	return l.asyncCall(sector, func(ci storiface.CallID) {
		_, done, err := (&localWorkerPathProvider{w: l, op: am}).AcquireSector(ctx, sector, fileType, storiface.FTNone, ptype)
		if err == nil {
			done()
		}

		if err := l.ret.ReturnFetch(ctx, ci, errstr(err)); err != nil {
			log.Errorf("ReturnFetch: %+v", err)
		}
	})
}

func (l *LocalWorker) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return l.asyncCall(sector, func(ci storiface.CallID) {
		var err error
		var p1o storage2.PreCommit1Out
		defer func() {
			if err := l.ret.ReturnSealPreCommit1(ctx, ci, p1o, errstr(err)); err != nil {
				log.Errorf("ReturnSealPreCommit1: %+v", err)
			}
		}()

		{
			// cleanup previous failed attempts if they exist
			if err = l.storage.Remove(ctx, sector, storiface.FTSealed, true); err != nil {
				err = xerrors.Errorf("cleaning up sealed data: %w", err)
				return
			}

			if err = l.storage.Remove(ctx, sector, storiface.FTCache, true); err != nil {
				err = xerrors.Errorf("cleaning up cache data: %w", err)
				return
			}
		}

		var sb ffiwrapper.Storage
		sb, err = l.sb()
		if err != nil {
			return
		}

		p1o, err = sb.SealPreCommit1(ctx, sector, ticket, pieces)
	})
}

func (l *LocalWorker) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.PreCommit1Out) (storiface.CallID, error) {
	sb, err := l.sb()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(sector, func(ci storiface.CallID) {
		cs, err := sb.SealPreCommit2(ctx, sector, phase1Out)

		if err := l.ret.ReturnSealPreCommit2(ctx, ci, cs, errstr(err)); err != nil {
			log.Errorf("ReturnSealPreCommit2: %+v", err)
		}
	})
}

func (l *LocalWorker) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage2.SectorCids) (storiface.CallID, error) {
	sb, err := l.sb()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(sector, func(ci storiface.CallID) {
		c1o, err := sb.SealCommit1(ctx, sector, ticket, seed, pieces, cids)

		if err := l.ret.ReturnSealCommit1(ctx, ci, c1o, errstr(err)); err != nil {
			log.Errorf("ReturnSealCommit1: %+v", err)
		}
	})
}

func (l *LocalWorker) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.Commit1Out) (storiface.CallID, error) {
	sb, err := l.sb()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(sector, func(ci storiface.CallID) {
		proof, err := sb.SealCommit2(ctx, sector, phase1Out)

		if err := l.ret.ReturnSealCommit2(ctx, ci, proof, errstr(err)); err != nil {
			log.Errorf("ReturnSealCommit2: %+v", err)
		}
	})
}

func (l *LocalWorker) FinalizeSector(ctx context.Context, sector abi.SectorID, keepUnsealed []storage2.Range) (storiface.CallID, error) {
	sb, err := l.sb()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(sector, func(ci storiface.CallID) {
		if err := sb.FinalizeSector(ctx, sector, keepUnsealed); err != nil {
			if err := l.ret.ReturnFinalizeSector(ctx, ci, errstr(xerrors.Errorf("finalizing sector: %w", err))); err != nil {
				log.Errorf("ReturnFinalizeSector: %+v", err)
			}
		}

		if len(keepUnsealed) == 0 {
			err = xerrors.Errorf("removing unsealed data: %w", err)
			if err := l.ret.ReturnFinalizeSector(ctx, ci, errstr(err)); err != nil {
				log.Errorf("ReturnFinalizeSector: %+v", err)
			}
		}

		if err := l.ret.ReturnFinalizeSector(ctx, ci, errstr(err)); err != nil {
			log.Errorf("ReturnFinalizeSector: %+v", err)
		}
	})
}

func (l *LocalWorker) ReleaseUnsealed(ctx context.Context, sector abi.SectorID, safeToFree []storage2.Range) (storiface.CallID, error) {
	return storiface.UndefCall, xerrors.Errorf("implement me")
}

func (l *LocalWorker) Remove(ctx context.Context, sector abi.SectorID) error {
	var err error

	if rerr := l.storage.Remove(ctx, sector, storiface.FTSealed, true); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (sealed): %w", rerr))
	}
	if rerr := l.storage.Remove(ctx, sector, storiface.FTCache, true); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (cache): %w", rerr))
	}
	if rerr := l.storage.Remove(ctx, sector, storiface.FTUnsealed, true); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (unsealed): %w", rerr))
	}

	return err
}

func (l *LocalWorker) MoveStorage(ctx context.Context, sector abi.SectorID, types storiface.SectorFileType) (storiface.CallID, error) {
	return l.asyncCall(sector, func(ci storiface.CallID) {
		err := l.storage.MoveStorage(ctx, sector, l.scfg.SealProofType, types)

		if err := l.ret.ReturnMoveStorage(ctx, ci, errstr(err)); err != nil {
			log.Errorf("ReturnMoveStorage: %+v", err)
		}
	})
}

func (l *LocalWorker) UnsealPiece(ctx context.Context, sector abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) (storiface.CallID, error) {
	sb, err := l.sb()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(sector, func(ci storiface.CallID) {
		var err error
		defer func() {
			if err := l.ret.ReturnUnsealPiece(ctx, ci, errstr(err)); err != nil {
				log.Errorf("ReturnUnsealPiece: %+v", err)
			}
		}()

		if err = sb.UnsealPiece(ctx, sector, index, size, randomness, cid); err != nil {
			err = xerrors.Errorf("unsealing sector: %w", err)
			return
		}

		if err = l.storage.RemoveCopies(ctx, sector, storiface.FTSealed); err != nil {
			err = xerrors.Errorf("removing source data: %w", err)
			return
		}

		if err = l.storage.RemoveCopies(ctx, sector, storiface.FTCache); err != nil {
			err = xerrors.Errorf("removing source data: %w", err)
			return
		}
	})
}

func (l *LocalWorker) ReadPiece(ctx context.Context, writer io.Writer, sector abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (storiface.CallID, error) {
	sb, err := l.sb()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(sector, func(ci storiface.CallID) {
		ok, err := sb.ReadPiece(ctx, writer, sector, index, size)

		if err := l.ret.ReturnReadPiece(ctx, ci, ok, errstr(err)); err != nil {
			log.Errorf("ReturnReadPiece: %+v", err)
		}
	})
}

func (l *LocalWorker) TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return l.acceptTasks, nil
}

func (l *LocalWorker) Paths(ctx context.Context) ([]stores.StoragePath, error) {
	return l.localStore.Local(ctx)
}

func (l *LocalWorker) Info(context.Context) (storiface.WorkerInfo, error) {
	hostname, err := os.Hostname() // TODO: allow overriding from config
	if err != nil {
		panic(err)
	}

	gpus, err := ffi.GetGPUDevices()
	if err != nil {
		log.Errorf("getting gpu devices failed: %+v", err)
	}

	h, err := sysinfo.Host()
	if err != nil {
		return storiface.WorkerInfo{}, xerrors.Errorf("getting host info: %w", err)
	}

	mem, err := h.Memory()
	if err != nil {
		return storiface.WorkerInfo{}, xerrors.Errorf("getting memory info: %w", err)
	}

	return storiface.WorkerInfo{
		Hostname: hostname,
		Resources: storiface.WorkerResources{
			MemPhysical: mem.Total,
			MemSwap:     mem.VirtualTotal,
			MemReserved: mem.VirtualUsed + mem.Total - mem.Available, // TODO: sub this process
			CPUs:        uint64(runtime.NumCPU()),
			GPUs:        gpus,
		},
	}, nil
}

func (l *LocalWorker) Closing(ctx context.Context) (<-chan struct{}, error) {
	return make(chan struct{}), nil
}

func (l *LocalWorker) Close() error {
	return nil
}

var _ Worker = &LocalWorker{}
