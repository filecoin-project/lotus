package hapi

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/must"
)

type PipelineTask struct {
	SpID         int64 `db:"sp_id"`
	SectorNumber int64 `db:"sector_number"`

	CreateTime time.Time `db:"create_time"`

	TaskSDR  *int64 `db:"task_id_sdr"`
	AfterSDR bool   `db:"after_sdr"`

	TaskTreeD  *int64 `db:"task_id_tree_d"`
	AfterTreeD bool   `db:"after_tree_d"`

	TaskTreeC  *int64 `db:"task_id_tree_c"`
	AfterTreeC bool   `db:"after_tree_c"`

	TaskTreeR  *int64 `db:"task_id_tree_r"`
	AfterTreeR bool   `db:"after_tree_r"`

	TaskPrecommitMsg  *int64 `db:"task_id_precommit_msg"`
	AfterPrecommitMsg bool   `db:"after_precommit_msg"`

	AfterPrecommitMsgSuccess bool   `db:"after_precommit_msg_success"`
	SeedEpoch                *int64 `db:"seed_epoch"`

	TaskPoRep  *int64 `db:"task_id_porep"`
	PoRepProof []byte `db:"porep_proof"`
	AfterPoRep bool   `db:"after_porep"`

	TaskFinalize  *int64 `db:"task_id_finalize"`
	AfterFinalize bool   `db:"after_finalize"`

	TaskMoveStorage  *int64 `db:"task_id_move_storage"`
	AfterMoveStorage bool   `db:"after_move_storage"`

	TaskCommitMsg  *int64 `db:"task_id_commit_msg"`
	AfterCommitMsg bool   `db:"after_commit_msg"`

	AfterCommitMsgSuccess bool `db:"after_commit_msg_success"`

	Failed       bool   `db:"failed"`
	FailedReason string `db:"failed_reason"`
}

type sectorListEntry struct {
	PipelineTask

	Address    address.Address
	CreateTime string
	AfterSeed  bool

	ChainAlloc, ChainSector, ChainActive, ChainUnproven, ChainFaulty bool
}

type minerBitfields struct {
	alloc, sectorSet, active, unproven, faulty bitfield.BitField
}

func (a *app) pipelinePorepSectors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var tasks []PipelineTask

	err := a.db.Select(ctx, &tasks, `SELECT 
       sp_id, sector_number,
       create_time,
       task_id_sdr, after_sdr,
       task_id_tree_d, after_tree_d,
       task_id_tree_c, after_tree_c,
       task_id_tree_r, after_tree_r,
       task_id_precommit_msg, after_precommit_msg,
       after_precommit_msg_success, seed_epoch,
       task_id_porep, porep_proof, after_porep,
       task_id_finalize, after_finalize,
       task_id_move_storage, after_move_storage,
       task_id_commit_msg, after_commit_msg,
       after_commit_msg_success,
       failed, failed_reason
    FROM sectors_sdr_pipeline order by sp_id, sector_number`) // todo where constrain list
	if err != nil {
		http.Error(w, xerrors.Errorf("failed to fetch pipeline tasks: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	head, err := a.workingApi.ChainHead(ctx)
	if err != nil {
		http.Error(w, xerrors.Errorf("failed to fetch chain head: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	epoch := head.Height()

	minerBitfieldCache := map[address.Address]minerBitfields{}

	sectorList := make([]sectorListEntry, 0, len(tasks))
	for _, task := range tasks {
		task := task

		task.CreateTime = task.CreateTime.Local()

		addr, err := address.NewIDAddress(uint64(task.SpID))
		if err != nil {
			http.Error(w, xerrors.Errorf("failed to create actor address: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		mbf, ok := minerBitfieldCache[addr]
		if !ok {
			mbf, err = a.getMinerBitfields(ctx, addr, a.stor)
			if err != nil {
				http.Error(w, xerrors.Errorf("failed to load miner bitfields: %w", err).Error(), http.StatusInternalServerError)
				return
			}
			minerBitfieldCache[addr] = mbf
		}

		afterSeed := task.SeedEpoch != nil && *task.SeedEpoch <= int64(epoch)

		sectorList = append(sectorList, sectorListEntry{
			PipelineTask: task,
			Address:      addr,
			CreateTime:   task.CreateTime.Format(time.DateTime),
			AfterSeed:    afterSeed,

			ChainAlloc:    must.One(mbf.alloc.IsSet(uint64(task.SectorNumber))),
			ChainSector:   must.One(mbf.sectorSet.IsSet(uint64(task.SectorNumber))),
			ChainActive:   must.One(mbf.active.IsSet(uint64(task.SectorNumber))),
			ChainUnproven: must.One(mbf.unproven.IsSet(uint64(task.SectorNumber))),
			ChainFaulty:   must.One(mbf.faulty.IsSet(uint64(task.SectorNumber))),
		})
	}

	a.executeTemplate(w, "pipeline_porep_sectors", sectorList)
}

func (a *app) getMinerBitfields(ctx context.Context, addr address.Address, stor adt.Store) (minerBitfields, error) {
	act, err := a.workingApi.StateGetActor(ctx, addr, types.EmptyTSK)
	if err != nil {
		return minerBitfields{}, xerrors.Errorf("failed to load actor: %w", err)
	}

	mas, err := miner.Load(stor, act)
	if err != nil {
		return minerBitfields{}, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	activeSectors, err := miner.AllPartSectors(mas, miner.Partition.ActiveSectors)
	if err != nil {
		return minerBitfields{}, xerrors.Errorf("failed to load active sectors: %w", err)
	}

	allSectors, err := miner.AllPartSectors(mas, miner.Partition.AllSectors)
	if err != nil {
		return minerBitfields{}, xerrors.Errorf("failed to load all sectors: %w", err)
	}

	unproved, err := miner.AllPartSectors(mas, miner.Partition.UnprovenSectors)
	if err != nil {
		return minerBitfields{}, xerrors.Errorf("failed to load unproven sectors: %w", err)
	}

	faulty, err := miner.AllPartSectors(mas, miner.Partition.FaultySectors)
	if err != nil {
		return minerBitfields{}, xerrors.Errorf("failed to load faulty sectors: %w", err)
	}

	alloc, err := mas.GetAllocatedSectors()
	if err != nil {
		return minerBitfields{}, xerrors.Errorf("failed to load allocated sectors: %w", err)
	}

	return minerBitfields{
		alloc:     *alloc,
		sectorSet: allSectors,
		active:    activeSectors,
		unproven:  unproved,
		faulty:    faulty,
	}, nil
}
