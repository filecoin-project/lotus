package lpseal

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/promise"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"time"
)

var log = logging.Logger("lpseal")

const (
	pollerSDR = iota
	pollerTrees
	pollerPrecommitMsg
	pollerPoRep

	numPollers
)

const sealPollerInterval = 10 * time.Second

type SealPollerAPI interface {
	StateSectorPreCommitInfo(context.Context, address.Address, abi.SectorNumber, types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error)
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

func (s *SealPoller) poll(ctx context.Context) error {
	var tasks []struct {
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

		AfterPrecommitMsgSuccess bool `db:"after_precommit_msg_success"`

		TaskPoRep  *int64 `db:"task_id_porep"`
		PoRepProof []byte `db:"porep_proof"`

		TaskCommitMsg  *int64 `db:"task_id_commit_msg"`
		AfterCommitMsg bool   `db:"after_commit_msg"`

		TaskCommitMsgWait     *int64 `db:"task_id_commit_msg_wait"`
		AfterCommitMsgSuccess bool   `db:"after_commit_msg_success"`

		Failed       bool   `db:"failed"`
		FailedReason string `db:"failed_reason"`
	}

	err := s.db.Select(ctx, &tasks, `SELECT 
       sp_id, sector_number,
       task_id_sdr, after_sdr,
       task_id_tree_d, after_tree_d,
       task_id_tree_c, after_tree_c,
       task_id_tree_r, after_tree_r,
       task_id_precommit_msg, after_precommit_msg,
       after_precommit_msg_success,
       task_id_porep, porep_proof,
       task_id_commit_msg, after_commit_msg,
       task_id_commit_msg_wait, after_commit_msg_success,
       failed, failed_reason
    FROM sectors_sdr_pipeline WHERE after_commit_msg_success != true`)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if task.Failed {
			continue
		}

		if task.TaskSDR == nil && s.pollers[pollerSDR].IsSet() {
			s.pollers[pollerSDR].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
				n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_sdr = $1 WHERE sp_id = $2 AND sector_number = $3 and task_id_sdr is null`, id, task.SpID, task.SectorNumber)
				if err != nil {
					return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
				}
				if n != 1 {
					return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
				}

				return true, nil
			})
		}
		if task.TaskTreeD == nil && task.TaskTreeC == nil && task.TaskTreeR == nil && s.pollers[pollerTrees].IsSet() && task.AfterSDR {
			s.pollers[pollerTrees].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
				n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_tree_d = $1, task_id_tree_c = $1, task_id_tree_r = $1
                            WHERE sp_id = $2 AND sector_number = $3 and after_sdr = true and task_id_tree_d is null and task_id_tree_c is null and task_id_tree_r is null`, id, task.SpID, task.SectorNumber)
				if err != nil {
					return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
				}
				if n != 1 {
					return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
				}

				return true, nil
			})
		}

		if task.TaskPrecommitMsg == nil && task.AfterTreeR && task.AfterTreeD {
			s.pollers[pollerPrecommitMsg].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
				n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_precommit_msg = $1 WHERE sp_id = $2 AND sector_number = $3 and task_id_precommit_msg is null and after_tree_r = true and after_tree_d = true`, id, task.SpID, task.SectorNumber)
				if err != nil {
					return false, xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
				}
				if n != 1 {
					return false, xerrors.Errorf("expected to update 1 row, updated %d", n)
				}

				return true, nil
			})
		}

		if task.TaskPrecommitMsg != nil && !task.AfterPrecommitMsgSuccess {
			// join pipeline precommit_msg_cid with message_waits signed_message_cid, if executed_tsk_epoch is not null, return result

			var execResult []struct {
				ExecutedTskCID   string `db:"executed_tsk_cid"`
				ExecutedTskEpoch int64  `db:"executed_tsk_epoch"`
				ExecutedMsgCID   string `db:"executed_msg_cid"`

				ExecutedRcptExitCode int64 `db:"executed_rcpt_exitcode"`
				ExecutedRcptGasUsed  int64 `db:"executed_rcpt_gas_used"`
			}

			err := s.db.Select(ctx, &execResult, `SELECT executed_tsk_cid, executed_tsk_epoch, executed_msg_cid, executed_rcpt_exitcode, executed_rcpt_gas_used
					FROM sectors_sdr_pipeline
					JOIN message_waits ON sectors_sdr_pipeline.precommit_msg_cid = message_waits.signed_message_cid
					WHERE sp_id = $1 AND sector_number = $2 AND executed_tsk_epoch is not null`, task.SpID, task.SectorNumber)
			if err != nil {
				log.Errorw("failed to query message_waits", "error", err)
			}

			if len(execResult) > 0 {
				maddr, err := address.NewIDAddress(uint64(task.SpID))
				if err != nil {
					return err
				}

				pci, err := s.api.StateSectorPreCommitInfo(ctx, maddr, abi.SectorNumber(task.SectorNumber), types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("get precommit info: %w", err)
				}

				if pci != nil {
					randHeight := pci.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

					_, err := s.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET 
                                seed_epoch = $1, precommit_msg_tsk = $2, after_precommit_msg_success = true 
                            WHERE sp_id = $3 AND sector_number = $4 and seed_epoch is NULL`,
						randHeight, execResult[0].ExecutedTskCID, task.SpID, task.SectorNumber)
					if err != nil {
						return xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
					}
				} // todo handle missing precommit info (eg expired precommit)

			}
		}

		todoWaitSeed := false
		if task.TaskPoRep != nil && todoWaitSeed {
			// todo start porep task
		}
	}

	return nil
}
