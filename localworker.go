package sectorstorage

import (
	"context"
	"io"
	"os"
	"runtime"

	"github.com/elastic/go-sysinfo"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/specs-actors/actors/abi"
	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/stores"
	"github.com/filecoin-project/sector-storage/storiface"
)

var pathTypes = []stores.SectorFileType{stores.FTUnsealed, stores.FTSealed, stores.FTCache}

type WorkerConfig struct {
	SealProof abi.RegisteredProof
	TaskTypes []sealtasks.TaskType
}

type LocalWorker struct {
	scfg       *ffiwrapper.Config
	storage    stores.Store
	localStore *stores.Local
	sindex     stores.SectorIndex

	acceptTasks map[sealtasks.TaskType]struct{}
}

func NewLocalWorker(wcfg WorkerConfig, store stores.Store, local *stores.Local, sindex stores.SectorIndex) *LocalWorker {
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

		acceptTasks: acceptTasks,
	}
}

type localWorkerPathProvider struct {
	w *LocalWorker
}

func (l *localWorkerPathProvider) AcquireSector(ctx context.Context, sector abi.SectorID, existing stores.SectorFileType, allocate stores.SectorFileType, sealing bool) (stores.SectorPaths, func(), error) {
	paths, storageIDs, done, err := l.w.storage.AcquireSector(ctx, sector, l.w.scfg.SealProofType, existing, allocate, sealing)
	if err != nil {
		return stores.SectorPaths{}, nil, err
	}

	log.Debugf("acquired sector %d (e:%d; a:%d): %v", sector, existing, allocate, paths)

	return paths, func() {
		done()

		for _, fileType := range pathTypes {
			if fileType&allocate == 0 {
				continue
			}

			sid := stores.PathByType(storageIDs, fileType)

			if err := l.w.sindex.StorageDeclareSector(ctx, stores.ID(sid), sector, fileType); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}, nil
}

func (l *LocalWorker) sb() (ffiwrapper.Storage, error) {
	return ffiwrapper.New(&localWorkerPathProvider{w: l}, l.scfg)
}

func (l *LocalWorker) NewSector(ctx context.Context, sector abi.SectorID) error {
	sb, err := l.sb()
	if err != nil {
		return err
	}

	return sb.NewSector(ctx, sector)
}

func (l *LocalWorker) AddPiece(ctx context.Context, sector abi.SectorID, epcs []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	sb, err := l.sb()
	if err != nil {
		return abi.PieceInfo{}, err
	}

	return sb.AddPiece(ctx, sector, epcs, sz, r)
}

func (l *LocalWorker) Fetch(ctx context.Context, sector abi.SectorID, fileType stores.SectorFileType, sealing bool) error {
	_, done, err := (&localWorkerPathProvider{w: l}).AcquireSector(ctx, sector, fileType, stores.FTNone, sealing)
	if err != nil {
		return err
	}
	done()
	return nil
}

func (l *LocalWorker) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage2.PreCommit1Out, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealPreCommit1(ctx, sector, ticket, pieces)
}

func (l *LocalWorker) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.PreCommit1Out) (cids storage2.SectorCids, err error) {
	sb, err := l.sb()
	if err != nil {
		return storage2.SectorCids{}, err
	}

	return sb.SealPreCommit2(ctx, sector, phase1Out)
}

func (l *LocalWorker) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage2.SectorCids) (output storage2.Commit1Out, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
}

func (l *LocalWorker) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.Commit1Out) (proof storage2.Proof, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit2(ctx, sector, phase1Out)
}

func (l *LocalWorker) FinalizeSector(ctx context.Context, sector abi.SectorID) error {
	sb, err := l.sb()
	if err != nil {
		return err
	}

	if err := sb.FinalizeSector(ctx, sector); err != nil {
		return xerrors.Errorf("finalizing sector: %w", err)
	}

	if err := l.storage.Remove(ctx, sector, stores.FTUnsealed); err != nil {
		return xerrors.Errorf("removing unsealed data: %w", err)
	}

	if err := l.storage.MoveStorage(ctx, sector, l.scfg.SealProofType, stores.FTSealed|stores.FTCache); err != nil {
		return xerrors.Errorf("moving sealed data to storage: %w", err)
	}

	return nil
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
