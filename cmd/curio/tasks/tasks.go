// Package tasks contains tasks that can be run by the curio command.
package tasks

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	curio "github.com/filecoin-project/lotus/curiosrc"
	"github.com/filecoin-project/lotus/curiosrc/message"
	"github.com/filecoin-project/lotus/curiosrc/winning"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
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
	si := dependencies.Si
	var activeTasks []harmonytask.TaskInterface

	sender, sendTask := message.NewSender(full, full, db)
	activeTasks = append(activeTasks, sendTask)

	///////////////////////////////////////////////////////////////////////
	///// Task Selection
	///////////////////////////////////////////////////////////////////////
	{

		if cfg.Subsystems.EnableWindowPost {
			wdPostTask, wdPoStSubmitTask, derlareRecoverTask, err := curio.WindowPostScheduler(ctx, cfg.Fees, cfg.Proving, full, verif, lw, sender,
				as, maddrs, db, stor, si, cfg.Subsystems.WindowPostMaxTasks)
			if err != nil {
				return nil, err
			}
			activeTasks = append(activeTasks, wdPostTask, wdPoStSubmitTask, derlareRecoverTask)
		}

		if cfg.Subsystems.EnableWinningPost {
			winPoStTask := winning.NewWinPostTask(cfg.Subsystems.WinningPostMaxTasks, db, lw, verif, full, maddrs)
			activeTasks = append(activeTasks, winPoStTask)
		}
	}
	log.Infow("This Curio instance handles",
		"miner_addresses", maddrs,
		"tasks", lo.Map(activeTasks, func(t harmonytask.TaskInterface, _ int) string { return t.TypeDetails().Name }))

	return harmonytask.New(db, activeTasks, dependencies.ListenAddr)
}
