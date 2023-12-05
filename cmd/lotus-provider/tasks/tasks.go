// Package tasks contains tasks that can be run by the lotus-provider command.
package tasks

import (
	"context"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/provider"
	"github.com/filecoin-project/lotus/provider/lpmessage"
	"github.com/filecoin-project/lotus/provider/lpwinning"
	"github.com/samber/lo"
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
	si := dependencies.Si
	var activeTasks []harmonytask.TaskInterface

	sender, sendTask := lpmessage.NewSender(full, full, db)
	activeTasks = append(activeTasks, sendTask)

	///////////////////////////////////////////////////////////////////////
	///// Task Selection
	///////////////////////////////////////////////////////////////////////
	{

		if cfg.Subsystems.EnableWindowPost {
			wdPostTask, wdPoStSubmitTask, derlareRecoverTask, err := provider.WindowPostScheduler(ctx, cfg.Fees, cfg.Proving, full, verif, lw, sender,
				as, maddrs, db, stor, si, cfg.Subsystems.WindowPostMaxTasks)
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
	log.Infow("This lotus_provider instance handles",
		"miner_addresses", maddrs,
		"tasks", lo.Map(activeTasks, func(t harmonytask.TaskInterface, _ int) string { return t.TypeDetails().Name }))

	return harmonytask.New(db, activeTasks, dependencies.ListenAddr)
}
