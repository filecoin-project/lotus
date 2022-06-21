package sealer

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type testWorker struct {
	acceptTasks map[sealtasks.TaskType]struct{}
	lstor       *paths.Local
	ret         storiface.WorkerReturn

	mockSeal *mock.SectorMgr

	pc1s    int
	pc1lk   sync.Mutex
	pc1wait *sync.WaitGroup

	session uuid.UUID

	Worker
}

func newTestWorker(wcfg WorkerConfig, lstor *paths.Local, ret storiface.WorkerReturn) *testWorker {
	acceptTasks := map[sealtasks.TaskType]struct{}{}
	for _, taskType := range wcfg.TaskTypes {
		acceptTasks[taskType] = struct{}{}
	}

	return &testWorker{
		acceptTasks: acceptTasks,
		lstor:       lstor,
		ret:         ret,

		mockSeal: mock.NewMockSectorMgr(nil),

		session: uuid.New(),
	}
}

func (t *testWorker) asyncCall(sector storiface.SectorRef, work func(ci storiface.CallID)) (storiface.CallID, error) {
	ci := storiface.CallID{
		Sector: sector.ID,
		ID:     uuid.New(),
	}

	go work(ci)

	return ci, nil
}

func (t *testWorker) AddPiece(ctx context.Context, sector storiface.SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error) {
	return t.asyncCall(sector, func(ci storiface.CallID) {
		p, err := t.mockSeal.AddPiece(ctx, sector, pieceSizes, newPieceSize, pieceData)
		if err := t.ret.ReturnAddPiece(ctx, ci, p, toCallError(err)); err != nil {
			log.Error(err)
		}
	})
}

func (t *testWorker) ReplicaUpdate(ctx context.Context, sector storiface.SectorRef, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return t.asyncCall(sector, func(ci storiface.CallID) {
		out, err := t.mockSeal.ReplicaUpdate(ctx, sector, pieces)
		if err := t.ret.ReturnReplicaUpdate(ctx, ci, out, toCallError(err)); err != nil {
			log.Error(err)
		}
	})
}

func (t *testWorker) ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.CallID, error) {
	return t.asyncCall(sector, func(ci storiface.CallID) {
		vanillaProofs, err := t.mockSeal.ProveReplicaUpdate1(ctx, sector, sectorKey, newSealed, newUnsealed)
		if err := t.ret.ReturnProveReplicaUpdate1(ctx, ci, vanillaProofs, toCallError(err)); err != nil {
			log.Error(err)
		}
	})
}

func (t *testWorker) ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (storiface.CallID, error) {
	return t.asyncCall(sector, func(ci storiface.CallID) {
		proof, err := t.mockSeal.ProveReplicaUpdate2(ctx, sector, sectorKey, newSealed, newUnsealed, vanillaProofs)
		if err := t.ret.ReturnProveReplicaUpdate2(ctx, ci, proof, toCallError(err)); err != nil {
			log.Error(err)
		}
	})
}

func (t *testWorker) SealPreCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return t.asyncCall(sector, func(ci storiface.CallID) {
		t.pc1s++

		if t.pc1wait != nil {
			t.pc1wait.Done()
		}

		t.pc1lk.Lock()
		defer t.pc1lk.Unlock()

		p1o, err := t.mockSeal.SealPreCommit1(ctx, sector, ticket, pieces)
		if err := t.ret.ReturnSealPreCommit1(ctx, ci, p1o, toCallError(err)); err != nil {
			log.Error(err)
		}
	})
}

func (t *testWorker) Fetch(ctx context.Context, sector storiface.SectorRef, fileType storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	return t.asyncCall(sector, func(ci storiface.CallID) {
		if err := t.ret.ReturnFetch(ctx, ci, nil); err != nil {
			log.Error(err)
		}
	})
}

func (t *testWorker) TaskTypes(ctx context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return t.acceptTasks, nil
}

func (t *testWorker) Paths(ctx context.Context) ([]storiface.StoragePath, error) {
	return t.lstor.Local(ctx)
}

func (t *testWorker) Info(ctx context.Context) (storiface.WorkerInfo, error) {
	res := storiface.ResourceTable[sealtasks.TTPreCommit2][abi.RegisteredSealProof_StackedDrg2KiBV1]

	return storiface.WorkerInfo{
		Hostname: "testworkerer",
		Resources: storiface.WorkerResources{
			MemPhysical: res.MinMemory * 3,
			MemUsed:     res.MinMemory,
			MemSwapUsed: 0,
			MemSwap:     0,
			CPUs:        32,
			GPUs:        nil,
		},
	}, nil
}

func (t *testWorker) Session(context.Context) (uuid.UUID, error) {
	return t.session, nil
}

func (t *testWorker) Close() error {
	panic("implement me")
}

var _ Worker = &testWorker{}
