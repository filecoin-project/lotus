// Package tasks contains tasks that can be run by the curio command.
package tasks

import (
	"context"
	"sort"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"
	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	curio "github.com/filecoin-project/lotus/curiosrc"
	"github.com/filecoin-project/lotus/curiosrc/alertmanager"
	"github.com/filecoin-project/lotus/curiosrc/chainsched"
	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/curiosrc/gc"
	"github.com/filecoin-project/lotus/curiosrc/message"
	"github.com/filecoin-project/lotus/curiosrc/piece"
	"github.com/filecoin-project/lotus/curiosrc/seal"
	"github.com/filecoin-project/lotus/curiosrc/winning"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/lazy"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("curio/deps")

func StartTasks(ctx context.Context, dependencies *deps.Deps) (*harmonytask.TaskEngine, error) {
	cfg := dependencies.Cfg
	db := dependencies.DB
	full := dependencies.Full
	verif := dependencies.Verif
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
				ctx, cfg.Fees, cfg.Proving, full, verif, sender, chainSched,
				as, maddrs, db, stor, si, cfg.Subsystems.WindowPostMaxTasks)

			if err != nil {
				return nil, err
			}
			activeTasks = append(activeTasks, wdPostTask, wdPoStSubmitTask, derlareRecoverTask)
			needProofParams = true
		}

		if cfg.Subsystems.EnableWinningPost {
			pl := dependencies.LocalStore
			winPoStTask := winning.NewWinPostTask(cfg.Subsystems.WinningPostMaxTasks, db, pl, verif, full, maddrs)
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
			parkPieceTask, err := piece.NewParkPieceTask(db, must.One(slrLazy.Val()), cfg.Subsystems.ParkPieceMaxTasks)
			if err != nil {
				return nil, err
			}
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
			treeDTask := seal.NewTreeDTask(sp, db, slr, cfg.Subsystems.SealSDRTreesMaxTasks)
			treeRCTask := seal.NewTreeRCTask(sp, db, slr, cfg.Subsystems.SealSDRTreesMaxTasks)
			finalizeTask := seal.NewFinalizeTask(cfg.Subsystems.FinalizeMaxTasks, sp, slr, db)
			activeTasks = append(activeTasks, treeDTask, treeRCTask, finalizeTask)
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
			commitTask := seal.NewSubmitCommitTask(sp, db, full, sender, as, cfg)
			activeTasks = append(activeTasks, commitTask)
		}
	}

	if hasAnySealingTask {
		// Sealing nodes maintain storage index when bored
		storageEndpointGcTask := gc.NewStorageEndpointGC(si, stor, db)
		activeTasks = append(activeTasks, storageEndpointGcTask)
	}

	amTask := alertmanager.NewAlertTask(full, db, cfg.Alerting)
	activeTasks = append(activeTasks, amTask)

	if needProofParams {
		for spt := range dependencies.ProofTypes {
			if err := modules.GetParams(true)(spt); err != nil {
				return nil, xerrors.Errorf("getting params: %w", err)
			}
		}
	}

	minerAddresses := make([]string, 0, len(maddrs))
	for k := range maddrs {
		minerAddresses = append(minerAddresses, address.Address(k).String())
	}

	log.Infow("This Curio instance handles",
		"miner_addresses", minerAddresses,
		"tasks", lo.Map(activeTasks, func(t harmonytask.TaskInterface, _ int) string { return t.TypeDetails().Name }))

	// harmony treats the first task as highest priority, so reverse the order
	// (we could have just appended to this list in the reverse order, but defining
	//  tasks in pipeline order is more intuitive)
	activeTasks = lo.Reverse(activeTasks)

	ht, err := harmonytask.New(db, activeTasks, dependencies.ListenAddr)
	if err != nil {
		return nil, err
	}
	go machineDetails(dependencies, activeTasks, ht.ResourcesAvailable().MachineID)

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

func machineDetails(deps *deps.Deps, activeTasks []harmonytask.TaskInterface, machineID int) {
	taskNames := lo.Map(activeTasks, func(item harmonytask.TaskInterface, _ int) string {
		return item.TypeDetails().Name
	})

	miners := lo.Map(maps.Keys(deps.Maddrs), func(item dtypes.MinerAddress, _ int) string {
		return address.Address(item).String()
	})
	sort.Strings(miners)

	_, err := deps.DB.Exec(context.Background(), `INSERT INTO harmony_machine_details 
		(tasks, layers, startup_time, miners, machine_id) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (machine_id) DO UPDATE SET tasks=$1, layers=$2, startup_time=$3, miners=$4`,
		strings.Join(taskNames, ","), strings.Join(deps.Layers, ","),
		time.Now(), strings.Join(miners, ","), machineID)

	if err != nil {
		log.Errorf("failed to update machine details: %s", err)
		return
	}

	// maybePostWarning
	if !lo.Contains(taskNames, "WdPost") && !lo.Contains(taskNames, "WinPost") {
		// Maybe we aren't running a PoSt for these miners?
		var allMachines []struct {
			MachineID int    `db:"machine_id"`
			Miners    string `db:"miners"`
			Tasks     string `db:"tasks"`
		}
		err := deps.DB.Select(context.Background(), &allMachines, `SELECT machine_id, miners, tasks FROM harmony_machine_details`)
		if err != nil {
			log.Errorf("failed to get machine details: %s", err)
			return
		}

		for _, miner := range miners {
			var myPostIsHandled bool
			for _, m := range allMachines {
				if !lo.Contains(strings.Split(m.Miners, ","), miner) {
					continue
				}
				if lo.Contains(strings.Split(m.Tasks, ","), "WdPost") && lo.Contains(strings.Split(m.Tasks, ","), "WinPost") {
					myPostIsHandled = true
					break
				}
			}
			if !myPostIsHandled {
				log.Errorf("No PoSt tasks are running for miner %s. Start handling PoSts immediately with:\n\tcurio run --layers=\"post\" ", miner)
			}
		}
	}
}
