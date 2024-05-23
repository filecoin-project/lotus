package seal

import (
	"context"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/promise"
)

var log = logging.Logger("cu/seal")

const (
	pollerSDR = iota
	pollerTreeD
	pollerTreeRC
	pollerPrecommitMsg
	pollerPoRep
	pollerCommitMsg
	pollerFinalize
	pollerMoveStorage

	numPollers
)

const sealPollerInterval = 10 * time.Second
const seedEpochConfidence = 3

type SealPollerAPI interface {
	StateSectorPreCommitInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	ChainHead(context.Context) (*types.TipSet, error)
}

type SealPoller struct {
	db  *harmonydb.DB
	api SealPollerAPI

	pollers [numPollers]promise.Promise[harmonytask.AddTaskFunc]
}

func NewPoller(db *harmonydb.DB, api SealPollerAPI) *SealPoller {
	return &SealPoller{
		db:  db,
		api: api,
	}
}

func (s *SealPoller) RunPoller(ctx context.Context) {
	ticker := time.NewTicker(sealPollerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.poll(ctx); err != nil {
				log.Errorw("polling failed", "error", err)
			}
		}
	}
}

/*
NOTE: TaskIDs are ONLY set while the tasks are executing or waiting to execute.
      This means that there are ~4 states each task can be in:
* Not run, and dependencies not solved (dependencies are 'After' fields of previous stages), task is null, After is false
* Not run, and dependencies solved, task is null, After is false
* Running or queued, task is set, After is false
* Finished, task is null, After is true
*/

type pollTask struct {
	SpID         int64 `db:"sp_id"`
	SectorNumber int64 `db:"sector_number"`

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

func (s *SealPoller) poll(ctx context.Context) error {
	var tasks []pollTask

	err := s.db.Select(ctx, &tasks, `SELECT 
       sp_id, sector_number,
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
    FROM sectors_sdr_pipeline WHERE after_commit_msg_success != TRUE OR after_move_storage != TRUE`)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		task := task
		if task.Failed {
			continue
		}

		ts, err := s.api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		s.pollStartSDR(ctx, task)
		s.pollStartSDRTreeD(ctx, task)
		s.pollStartSDRTreeRC(ctx, task)
		s.pollStartPrecommitMsg(ctx, task)
		s.mustPoll(s.pollPrecommitMsgLanded(ctx, task))
		s.pollStartPoRep(ctx, task, ts)
		s.pollStartFinalize(ctx, task, ts)
		s.pollStartMoveStorage(ctx, task)
		s.pollStartCommitMsg(ctx, task)
		s.mustPoll(s.pollCommitMsgLanded(ctx, task))
	}

	return nil
}

func (s *SealPoller) pollStartSDR(ctx context.Context, task pollTask) {
	if !task.AfterSDR && task.TaskSDR == nil && s.pollers[pollerSDR].IsSet() {
		s.pollers[pollerSDR].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
			n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_sdr = $1 WHERE sp_id = $2 AND sector_number = $3 AND task_id_sdr IS NULL`, id, task.SpID, task.SectorNumber)
			if err != nil {
				return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
			}
			if n != 1 {
				return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
			}

			return true, nil
		})
	}
}

func (t pollTask) afterSDR() bool {
	return t.AfterSDR
}

func (s *SealPoller) pollStartSDRTreeD(ctx context.Context, task pollTask) {
	if !task.AfterTreeD && task.TaskTreeD == nil && s.pollers[pollerTreeD].IsSet() && task.afterSDR() {
		s.pollers[pollerTreeD].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
			n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_tree_d = $1 WHERE sp_id = $2 AND sector_number = $3 AND after_sdr = TRUE AND task_id_tree_d IS NULL`, id, task.SpID, task.SectorNumber)
			if err != nil {
				return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
			}
			if n != 1 {
				return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
			}

			return true, nil
		})
	}
}

func (t pollTask) afterTreeD() bool {
	return t.AfterTreeD && t.afterSDR()
}

func (s *SealPoller) pollStartSDRTreeRC(ctx context.Context, task pollTask) {
	if !task.AfterTreeC && !task.AfterTreeR && task.TaskTreeC == nil && task.TaskTreeR == nil && s.pollers[pollerTreeRC].IsSet() && task.afterTreeD() {
		s.pollers[pollerTreeRC].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
			n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_tree_c = $1, task_id_tree_r = $1
                            WHERE sp_id = $2 AND sector_number = $3 AND after_tree_d = TRUE AND task_id_tree_c IS NULL AND task_id_tree_r IS NULL`, id, task.SpID, task.SectorNumber)
			if err != nil {
				return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
			}
			if n != 1 {
				return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
			}

			return true, nil
		})
	}
}

func (t pollTask) afterTreeRC() bool {
	return t.AfterTreeC && t.AfterTreeR && t.afterTreeD()
}

func (t pollTask) afterPrecommitMsg() bool {
	return t.AfterPrecommitMsg && t.afterTreeRC()
}

func (t pollTask) afterPrecommitMsgSuccess() bool {
	return t.AfterPrecommitMsgSuccess && t.afterPrecommitMsg()
}

func (s *SealPoller) pollStartPoRep(ctx context.Context, task pollTask, ts *types.TipSet) {
	if s.pollers[pollerPoRep].IsSet() && task.afterPrecommitMsgSuccess() && task.SeedEpoch != nil &&
		task.TaskPoRep == nil && !task.AfterPoRep &&
		ts.Height() >= abi.ChainEpoch(*task.SeedEpoch+seedEpochConfidence) {

		s.pollers[pollerPoRep].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
			n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_porep = $1 WHERE sp_id = $2 AND sector_number = $3 AND task_id_porep IS NULL`, id, task.SpID, task.SectorNumber)
			if err != nil {
				return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
			}
			if n != 1 {
				return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
			}

			return true, nil
		})
	}
}

func (t pollTask) afterPoRep() bool {
	return t.AfterPoRep && t.afterPrecommitMsgSuccess()
}

func (s *SealPoller) pollStartFinalize(ctx context.Context, task pollTask, ts *types.TipSet) {
	if s.pollers[pollerFinalize].IsSet() && task.afterPoRep() && !task.AfterFinalize && task.TaskFinalize == nil {
		s.pollers[pollerFinalize].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
			n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_finalize = $1 WHERE sp_id = $2 AND sector_number = $3 AND task_id_finalize IS NULL`, id, task.SpID, task.SectorNumber)
			if err != nil {
				return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
			}
			if n != 1 {
				return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
			}

			return true, nil
		})
	}
}

func (t pollTask) afterFinalize() bool {
	return t.AfterFinalize && t.afterPoRep()
}

func (s *SealPoller) pollStartMoveStorage(ctx context.Context, task pollTask) {
	if s.pollers[pollerMoveStorage].IsSet() && task.afterFinalize() && !task.AfterMoveStorage && task.TaskMoveStorage == nil {
		s.pollers[pollerMoveStorage].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
			n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_move_storage = $1 WHERE sp_id = $2 AND sector_number = $3 AND task_id_move_storage IS NULL`, id, task.SpID, task.SectorNumber)
			if err != nil {
				return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
			}
			if n != 1 {
				return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
			}

			return true, nil
		})
	}
}

func (s *SealPoller) mustPoll(err error) {
	if err != nil {
		log.Errorw("poller operation failed", "error", err)
	}
}
