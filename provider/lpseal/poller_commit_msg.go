package lpseal

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
)

func (s *SealPoller) pollStartCommitMsg(ctx context.Context, task pollTask) {
	if task.AfterPoRep && len(task.PoRepProof) > 0 && task.TaskCommitMsg == nil && !task.AfterCommitMsg && s.pollers[pollerCommitMsg].IsSet() {
		s.pollers[pollerCommitMsg].Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, seriousError error) {
			n, err := tx.Exec(`UPDATE sectors_sdr_pipeline SET task_id_commit_msg = $1 WHERE sp_id = $2 AND sector_number = $3 and task_id_commit_msg is null`, id, task.SpID, task.SectorNumber)
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

func (s *SealPoller) pollCommitMsgLanded(ctx context.Context, task pollTask) error {
	if task.AfterCommitMsg && !task.AfterCommitMsgSuccess && s.pollers[pollerCommitMsg].IsSet() {
		var execResult []dbExecResult

		err := s.db.Select(ctx, &execResult, `SELECT spipeline.precommit_msg_cid, spipeline.commit_msg_cid, executed_tsk_cid, executed_tsk_epoch, executed_msg_cid, executed_rcpt_exitcode, executed_rcpt_gas_used
					FROM sectors_sdr_pipeline spipeline
					JOIN message_waits ON spipeline.commit_msg_cid = message_waits.signed_message_cid
					WHERE sp_id = $1 AND sector_number = $2 AND executed_tsk_epoch is not null`, task.SpID, task.SectorNumber)
		if err != nil {
			log.Errorw("failed to query message_waits", "error", err)
		}

		if len(execResult) > 0 {
			maddr, err := address.NewIDAddress(uint64(task.SpID))
			if err != nil {
				return err
			}

			if exitcode.ExitCode(execResult[0].ExecutedRcptExitCode) != exitcode.Ok {
				return s.pollCommitMsgFail(ctx, task, execResult[0])
			}

			si, err := s.api.StateSectorGetInfo(ctx, maddr, abi.SectorNumber(task.SectorNumber), types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("get sector info: %w", err)
			}

			if si == nil {
				log.Errorw("todo handle missing sector info (not found after cron)", "sp", task.SpID, "sector", task.SectorNumber, "exec_epoch", execResult[0].ExecutedTskEpoch, "exec_tskcid", execResult[0].ExecutedTskCID, "msg_cid", execResult[0].ExecutedMsgCID)
				// todo handdle missing sector info (not found after cron)
			} else {
				// yay!

				_, err := s.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET
						after_commit_msg_success = true, commit_msg_tsk = $1
						WHERE sp_id = $2 AND sector_number = $3 and after_commit_msg_success = false`,
					execResult[0].ExecutedTskCID, task.SpID, task.SectorNumber)
				if err != nil {
					return xerrors.Errorf("update sectors_sdr_pipeline: %w", err)
				}
			}
		}
	}

	return nil
}

func (s *SealPoller) pollCommitMsgFail(ctx context.Context, task pollTask, execResult dbExecResult) error {
	switch exitcode.ExitCode(execResult.ExecutedRcptExitCode) {
	case exitcode.SysErrInsufficientFunds:
		fallthrough
	case exitcode.SysErrOutOfGas:
		// just retry
		return s.pollRetryPrecommitMsgSend(ctx, task, execResult)
	default:
		return xerrors.Errorf("commit message failed with exit code %s", exitcode.ExitCode(execResult.ExecutedRcptExitCode))
	}
}

func (s *SealPoller) pollRetryCommitMsgSend(ctx context.Context, task pollTask, execResult dbExecResult) error {
	if execResult.CommitMsgCID == nil {
		return xerrors.Errorf("commit msg cid was nil")
	}

	// make the pipeline entry seem like precommit send didn't happen, next poll loop will retry

	_, err := s.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET
                                commit_msg_cid = null, task_id_commit_msg = null
                            	WHERE commit_msg_cid = $1 AND sp_id = $2 AND sector_number = $3 AND after_commit_msg_success = false`,
		*execResult.CommitMsgCID, task.SpID, task.SectorNumber)
	if err != nil {
		return xerrors.Errorf("update sectors_sdr_pipeline to retry precommit msg send: %w", err)
	}

	return nil
}
