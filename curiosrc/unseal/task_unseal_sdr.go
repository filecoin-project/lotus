package unseal

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"golang.org/x/xerrors"
)

var isDevnet = build.BlockDelaySecs < 30

type UnsealSDRApi interface {
	StateSectorGetInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorOnChainInfo, error)
}

type TaskUnsealSdr struct {
	max int

	sc *ffi.SealCalls
	db *harmonydb.DB
	api UnsealSDRApi
}

func (t *TaskUnsealSdr) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParamsArr []struct {
		SpID         int64 `db:"sp_id"`
		SectorNumber int64 `db:"sector_number"`
	}

	err = t.db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number
		FROM sectors_unseal_pipeline
		WHERE task_id_unseal_sdr = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting sector params: %w", err)
	}

	if len(sectorParamsArr) == 0 {
		return false, xerrors.Errorf("no sector params")
	}

	sectorParams := sectorParamsArr[0]

	maddr, err := address.NewIDAddress(uint64(sectorParams.SpID))
	if err != nil {
		return false, xerrors.Errorf("failed to convert miner ID to address: %w", err)
	}

	sinfo, err := t.api.StateSectorGetInfo(ctx, maddr, abi.SectorNumber(sectorParams.SectorNumber), types.EmptyTSK)
	if err != nil {
		return false, xerrors.Errorf("getting sector info: %w", err)
	}
	if sinfo == nil {
		return false, xerrors.Errorf("sector not found")
	}

	sref := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(sectorParams.SpID),
			Number: abi.SectorNumber(sectorParams.SectorNumber),
		},
		ProofType: sinfo.SealProof,
	}

	t.sc.GenerateSDR(ctx, taskID, storiface.FTKeyCache, sref,
}

func (t *TaskUnsealSdr) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	id := ids[0]
	return &id, nil
}

func (t *TaskUnsealSdr) TypeDetails() harmonytask.TaskTypeDetails {
	ssize := abi.SectorSize(32 << 30) // todo task details needs taskID to get correct sector size
	if isDevnet {
		ssize = abi.SectorSize(2 << 20)
	}

	res := harmonytask.TaskTypeDetails{
		Max:  t.max,
		Name: "SDRKeyRegen",
		Cost: resources.Resources{
			Cpu:     4, // todo multicore sdr
			Gpu:     0,
			Ram:     54 << 30,
			Storage: t.sc.Storage(t.taskToSector, storiface.FTKeyCache, storiface.FTNone, ssize, storiface.PathSealing),
		},
		MaxFailures: 2,
		Follows:     nil,
	}

	if isDevnet {
		res.Cost.Ram = 1 << 30
	}

	return res
}

func (t *TaskUnsealSdr) Adder(taskFunc harmonytask.AddTaskFunc) {
	//TODO implement me
	panic("implement me")
}

func (t *TaskUnsealSdr) taskToSector(id harmonytask.TaskID) (ffi.SectorRef, error) {
	var refs []ffi.SectorRef

	err := t.db.Select(context.Background(), &refs, `SELECT sp_id, sector_number, reg_seal_proof FROM sectors_unseal_pipeline WHERE task_id_unseal_sdr = $1`, id)
	if err != nil {
		return ffi.SectorRef{}, xerrors.Errorf("getting sector ref: %w", err)
	}

	if len(refs) != 1 {
		return ffi.SectorRef{}, xerrors.Errorf("expected 1 sector ref, got %d", len(refs))
	}

	return refs[0], nil
}

var _ harmonytask.TaskInterface = (*TaskUnsealSdr)(nil)
