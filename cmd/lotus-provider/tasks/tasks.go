// Package tasks contains tasks that can be run by the lotus-provider command.
package tasks

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/provider"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/provider/lpmessage"
	"github.com/filecoin-project/lotus/provider/lpseal"
	"github.com/filecoin-project/lotus/provider/lpwinning"
)

var log = logging.Logger("lotus-provider/deps")

func StartTasks(ctx context.Context, dependencies *deps.Deps) (*harmonytask.TaskEngine, error) {
	cfg := dependencies.Cfg
	db := dependencies.DB
	full := dependencies.Full
	verif := dependencies.Verif
	lw := dependencies.LW
	as := dependencies.As
	maddrs := dependencies.Maddrs
	stor := dependencies.Stor
	lstor := dependencies.LocalStore
	si := dependencies.Si
	var activeTasks []harmonytask.TaskInterface

	sender, sendTask := lpmessage.NewSender(full, full, db)
	activeTasks = append(activeTasks, sendTask)

	chainSched := chainsched.New(full)

	var needProofParams bool

	///////////////////////////////////////////////////////////////////////
	///// Task Selection
	///////////////////////////////////////////////////////////////////////
	{
		// PoSt

		if cfg.Subsystems.EnableWindowPost {
			wdPostTask, wdPoStSubmitTask, derlareRecoverTask, err := provider.WindowPostScheduler(ctx, cfg.Fees, cfg.Proving, full, verif, lw, sender,
				chainSched, as, maddrs, db, stor, si, cfg.Subsystems.WindowPostMaxTasks)
			if err != nil {
				return nil, err
			}
			activeTasks = append(activeTasks, wdPostTask, wdPoStSubmitTask, derlareRecoverTask)
			needProofParams = true
		}

		if cfg.Subsystems.EnableWinningPost {
			winPoStTask := lpwinning.NewWinPostTask(cfg.Subsystems.WinningPostMaxTasks, db, lw, verif, full, maddrs)
			activeTasks = append(activeTasks, winPoStTask)
			needProofParams = true
		}
	}

	hasAnySealingTask := cfg.Subsystems.EnableSealSDR ||
		cfg.Subsystems.EnableSealSDRTrees ||
		cfg.Subsystems.EnableSendPrecommitMsg ||
		cfg.Subsystems.EnablePoRepProof ||
		cfg.Subsystems.EnableMoveStorage ||
		cfg.Subsystems.EnableSendCommitMsg
	{
		// Sealing

		var sp *lpseal.SealPoller
		var slr *lpffi.SealCalls
		if hasAnySealingTask {
			sp = lpseal.NewPoller(db, full)
			go sp.RunPoller(ctx)

			slr = lpffi.NewSealCalls(stor, lstor, si)
		}

		// NOTE: Tasks with the LEAST priority are at the top
		if cfg.Subsystems.EnableSealSDR {
			sdrTask := lpseal.NewSDRTask(full, db, sp, slr, cfg.Subsystems.SealSDRMaxTasks)
			activeTasks = append(activeTasks, sdrTask)
		}
		if cfg.Subsystems.EnableSealSDRTrees {
			treesTask := lpseal.NewTreesTask(sp, db, slr, cfg.Subsystems.SealSDRTreesMaxTasks)
			finalizeTask := lpseal.NewFinalizeTask(cfg.Subsystems.FinalizeMaxTasks, sp, slr, db)
			activeTasks = append(activeTasks, treesTask, finalizeTask)
		}
		if cfg.Subsystems.EnableSendPrecommitMsg {
			precommitTask := lpseal.NewSubmitPrecommitTask(sp, db, full, sender, as, cfg.Fees.MaxPreCommitGasFee)
			activeTasks = append(activeTasks, precommitTask)
		}
		if cfg.Subsystems.EnablePoRepProof {
			porepTask := lpseal.NewPoRepTask(db, full, sp, slr, cfg.Subsystems.PoRepProofMaxTasks)
			activeTasks = append(activeTasks, porepTask)
			needProofParams = true
		}
		if cfg.Subsystems.EnableMoveStorage {
			moveStorageTask := lpseal.NewMoveStorageTask(sp, slr, db, cfg.Subsystems.MoveStorageMaxTasks)
			activeTasks = append(activeTasks, moveStorageTask)
		}
		if cfg.Subsystems.EnableSendCommitMsg {
			commitTask := lpseal.NewSubmitCommitTask(sp, db, full, sender, as, cfg.Fees.MaxCommitGasFee)
			activeTasks = append(activeTasks, commitTask)
		}
	}

	if needProofParams {
		for spt := range dependencies.ProofTypes {
			if err := modules.GetParams(true)(spt); err != nil {
				return nil, xerrors.Errorf("getting params: %w", err)
			}
		}
	}

	log.Infow("This lotus_provider instance handles",
		"miner_addresses", maddrs,
		"tasks", lo.Map(activeTasks, func(t harmonytask.TaskInterface, _ int) string { return t.TypeDetails().Name }))

	// harmony treats the first task as highest priority, so reverse the order
	// (we could have just appended to this list in the reverse order, but defining
	//  tasks in pipeline order is more intuitive)
	activeTasks = lo.Reverse(activeTasks)

	ht, err := harmonytask.New(db, activeTasks, dependencies.ListenAddr)
	if err != nil {
		return nil, err
	}

	if hasAnySealingTask {
		watcher, err := lpmessage.NewMessageWatcher(db, ht, chainSched, full)
		if err != nil {
			return nil, err
		}
		_ = watcher
	}

	if cfg.Subsystems.EnableWindowPost || hasAnySealingTask {
		go chainSched.Run(ctx)
	}

	return ht, nil
}
