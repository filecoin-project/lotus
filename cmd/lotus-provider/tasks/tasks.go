// Package tasks contains tasks that can be run by the lotus-provider command.
package tasks

import (
	"context"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/provider/lpseal"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"

	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/provider"
	"github.com/filecoin-project/lotus/provider/lpmessage"
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
		}

		if cfg.Subsystems.EnableWinningPost {
			winPoStTask := lpwinning.NewWinPostTask(cfg.Subsystems.WinningPostMaxTasks, db, lw, verif, full, maddrs)
			activeTasks = append(activeTasks, winPoStTask)
		}
	}

	hasAnySealingTask := cfg.Subsystems.EnableSealSDR
	{
		// Sealing

		var sp *lpseal.SealPoller
		var slr *lpffi.SealCalls
		if hasAnySealingTask {
			sp = lpseal.NewPoller(db, full)
			go sp.RunPoller(ctx)

			slr = lpffi.NewSealCalls(stor, lstor, si)
		}

		if cfg.Subsystems.EnableSealSDR {
			sdrTask := lpseal.NewSDRTask(full, db, sp, slr, cfg.Subsystems.SealSDRMaxTasks)
			activeTasks = append(activeTasks, sdrTask)
		}
		if cfg.Subsystems.EnableSealSDRTrees {
			treesTask := lpseal.NewTreesTask(sp, db, slr, cfg.Subsystems.SealSDRTreesMaxTasks)
			activeTasks = append(activeTasks, treesTask)
		}
		if cfg.Subsystems.EnableSendPrecommitMsg {
			precommitTask := lpseal.NewSubmitPrecommitTask(sp, db, full, sender, as, cfg.Fees.MaxPreCommitGasFee)
			activeTasks = append(activeTasks, precommitTask)
		}
	}
	log.Infow("This lotus_provider instance handles",
		"miner_addresses", maddrs,
		"tasks", lo.Map(activeTasks, func(t harmonytask.TaskInterface, _ int) string { return t.TypeDetails().Name }))

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
