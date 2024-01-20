package lpseal

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
)

func (s *SealPoller) pollStartPrecommitMsg(ctx context.Context, task pollTask) {
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
}

type dbExecResult struct {
	PrecommitMsgCID *string `db:"precommit_msg_cid"`
	CommitMsgCID    *string `db:"commit_msg_cid"`

	ExecutedTskCID   string `db:"executed_tsk_cid"`
	ExecutedTskEpoch int64  `db:"executed_tsk_epoch"`
	ExecutedMsgCID   string `db:"executed_msg_cid"`

	ExecutedRcptExitCode int64 `db:"executed_rcpt_exitcode"`
	ExecutedRcptGasUsed  int64 `db:"executed_rcpt_gas_used"`
}

func (s *SealPoller) pollPrecommitMsgLanded(ctx context.Context, task pollTask) error {
	if task.TaskPrecommitMsg != nil && !task.AfterPrecommitMsgSuccess {
		var execResult []dbExecResult

		err := s.db.Select(ctx, &execResult, `SELECT spipeline.precommit_msg_cid, spipeline.commit_msg_cid, executed_tsk_cid, executed_tsk_epoch, executed_msg_cid, executed_rcpt_exitcode, executed_rcpt_gas_used
					FROM sectors_sdr_pipeline spipeline
					JOIN message_waits ON spipeline.precommit_msg_cid = message_waits.signed_message_cid
					WHERE sp_id = $1 AND sector_number = $2 AND executed_tsk_epoch is not null`, task.SpID, task.SectorNumber)
		if err != nil {
			log.Errorw("failed to query message_waits", "error", err)
		}

		if len(execResult) > 0 {
			if exitcode.ExitCode(execResult[0].ExecutedRcptExitCode) != exitcode.Ok {
				return s.pollPrecommitMsgFail(ctx, task, execResult[0])
			}

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

	return nil
}

func (s *SealPoller) pollPrecommitMsgFail(ctx context.Context, task pollTask, execResult dbExecResult) error {
	switch exitcode.ExitCode(execResult.ExecutedRcptExitCode) {
	case exitcode.SysErrInsufficientFunds:
		fallthrough
	case exitcode.SysErrOutOfGas:
		// just retry
		return s.pollRetryPrecommitMsgSend(ctx, task, execResult)
	default:
		return xerrors.Errorf("precommit message failed with exit code %s", exitcode.ExitCode(execResult.ExecutedRcptExitCode))
	}
}

func (s *SealPoller) pollRetryPrecommitMsgSend(ctx context.Context, task pollTask, execResult dbExecResult) error {
	if execResult.PrecommitMsgCID == nil {
		return xerrors.Errorf("precommit msg cid was nil")
	}

	// make the pipeline entry seem like precommit send didn't happen, next poll loop will retry

	_, err := s.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET
                                precommit_msg_cid = null, task_id_precommit_msg = null
                            	WHERE precommit_msg_cid = $1 AND sp_id = $2 AND sector_number = $3 AND after_precommit_msg_success = false`,
		*execResult.PrecommitMsgCID, task.SpID, task.SectorNumber)
	if err != nil {
		return xerrors.Errorf("update sectors_sdr_pipeline to retry precommit msg send: %w", err)
	}

	return nil
}
