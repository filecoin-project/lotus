// Package tasks contains tasks that can be run by the curio command.
package tasks

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	curio "github.com/filecoin-project/lotus/curiosrc"
	"github.com/filecoin-project/lotus/curiosrc/chainsched"
	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/curiosrc/message"
	"github.com/filecoin-project/lotus/curiosrc/piece"
	"github.com/filecoin-project/lotus/curiosrc/seal"
	"github.com/filecoin-project/lotus/curiosrc/winning"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/lazy"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/modules"
)

var log = logging.Logger("curio/deps")

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

	sender, sendTask := message.NewSender(full, full, db)
	activeTasks = append(activeTasks, sendTask)

	chainSched := chainsched.New(full)

	var needProofParams bool

	///////////////////////////////////////////////////////////////////////
	///// Task Selection
	///////////////////////////////////////////////////////////////////////
	{
		// PoSt

		if cfg.Subsystems.EnableWindowPost {
			wdPostTask, wdPoStSubmitTask, derlareRecoverTask, err := curio.WindowPostScheduler(
				ctx, cfg.Fees, cfg.Proving, full, verif, lw, sender, chainSched,
				as, maddrs, db, stor, si, cfg.Subsystems.WindowPostMaxTasks)

			if err != nil {
				return nil, err
			}
			activeTasks = append(activeTasks, wdPostTask, wdPoStSubmitTask, derlareRecoverTask)
			needProofParams = true
		}

		if cfg.Subsystems.EnableWinningPost {
			winPoStTask := winning.NewWinPostTask(cfg.Subsystems.WinningPostMaxTasks, db, lw, verif, full, maddrs)
			activeTasks = append(activeTasks, winPoStTask)
			needProofParams = true
		}
	}

	slrLazy := lazy.MakeLazy(func() (*ffi.SealCalls, error) {
		return ffi.NewSealCalls(stor, lstor, si), nil
	})

	{
		// Piece handling
		if cfg.Subsystems.EnableParkPiece {
			parkPieceTask := piece.NewParkPieceTask(db, must.One(slrLazy.Val()), cfg.Subsystems.ParkPieceMaxTasks)
			cleanupPieceTask := piece.NewCleanupPieceTask(db, must.One(slrLazy.Val()), 0)
			activeTasks = append(activeTasks, parkPieceTask, cleanupPieceTask)
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

		var sp *seal.SealPoller
		var slr *ffi.SealCalls
		if hasAnySealingTask {
			sp = seal.NewPoller(db, full)
			go sp.RunPoller(ctx)

			slr = must.One(slrLazy.Val())
		}

		// NOTE: Tasks with the LEAST priority are at the top
		if cfg.Subsystems.EnableSealSDR {
			sdrTask := seal.NewSDRTask(full, db, sp, slr, cfg.Subsystems.SealSDRMaxTasks)
			activeTasks = append(activeTasks, sdrTask)
		}
		if cfg.Subsystems.EnableSealSDRTrees {
			treesTask := seal.NewTreesTask(sp, db, slr, cfg.Subsystems.SealSDRTreesMaxTasks)
			finalizeTask := seal.NewFinalizeTask(cfg.Subsystems.FinalizeMaxTasks, sp, slr, db)
			activeTasks = append(activeTasks, treesTask, finalizeTask)
		}
		if cfg.Subsystems.EnableSendPrecommitMsg {
			precommitTask := seal.NewSubmitPrecommitTask(sp, db, full, sender, as, cfg.Fees.MaxPreCommitGasFee)
			activeTasks = append(activeTasks, precommitTask)
		}
		if cfg.Subsystems.EnablePoRepProof {
			porepTask := seal.NewPoRepTask(db, full, sp, slr, cfg.Subsystems.PoRepProofMaxTasks)
			activeTasks = append(activeTasks, porepTask)
			needProofParams = true
		}
		if cfg.Subsystems.EnableMoveStorage {
			moveStorageTask := seal.NewMoveStorageTask(sp, slr, db, cfg.Subsystems.MoveStorageMaxTasks)
			activeTasks = append(activeTasks, moveStorageTask)
		}
		if cfg.Subsystems.EnableSendCommitMsg {
			commitTask := seal.NewSubmitCommitTask(sp, db, full, sender, as, cfg.Fees.MaxCommitGasFee)
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
		watcher, err := message.NewMessageWatcher(db, ht, chainSched, full)
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
