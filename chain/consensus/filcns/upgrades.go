package filcns

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/docker/go-units"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/term"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	nv18 "github.com/filecoin-project/go-state-types/builtin/v10/migration"
	init11 "github.com/filecoin-project/go-state-types/builtin/v11/init"
	nv19 "github.com/filecoin-project/go-state-types/builtin/v11/migration"
	system11 "github.com/filecoin-project/go-state-types/builtin/v11/system"
	init12 "github.com/filecoin-project/go-state-types/builtin/v12/init"
	nv21 "github.com/filecoin-project/go-state-types/builtin/v12/migration"
	system12 "github.com/filecoin-project/go-state-types/builtin/v12/system"
	nv22 "github.com/filecoin-project/go-state-types/builtin/v13/migration"
	nv23 "github.com/filecoin-project/go-state-types/builtin/v14/migration"
	nv24 "github.com/filecoin-project/go-state-types/builtin/v15/migration"
	nv25 "github.com/filecoin-project/go-state-types/builtin/v16/migration"
	nv27 "github.com/filecoin-project/go-state-types/builtin/v17/migration"
	nv28 "github.com/filecoin-project/go-state-types/builtin/v18/migration"
	nv17 "github.com/filecoin-project/go-state-types/builtin/v9/migration"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/migration"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-state-types/rt"
	gstStore "github.com/filecoin-project/go-state-types/store"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	multisig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	power0 "github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/migration/nv3"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/filecoin-project/specs-actors/v2/actors/migration/nv4"
	"github.com/filecoin-project/specs-actors/v2/actors/migration/nv7"
	"github.com/filecoin-project/specs-actors/v3/actors/migration/nv10"
	"github.com/filecoin-project/specs-actors/v4/actors/migration/nv12"
	"github.com/filecoin-project/specs-actors/v5/actors/migration/nv13"
	"github.com/filecoin-project/specs-actors/v6/actors/migration/nv14"
	"github.com/filecoin-project/specs-actors/v7/actors/migration/nv15"
	"github.com/filecoin-project/specs-actors/v8/actors/migration/nv16"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/system"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node/bundle"
)

//go:embed UpgradeSplash.txt
var upgradeSplash string

var (
	MigrationMaxWorkerCount    int
	EnvMigrationMaxWorkerCount = "LOTUS_MIGRATION_MAX_WORKER_COUNT"
)

func init() {
	// the default calculation used for migration worker count
	MigrationMaxWorkerCount = runtime.NumCPU()
	// check if an alternative value was request by environment
	if mwcs := os.Getenv(EnvMigrationMaxWorkerCount); mwcs != "" {
		mwc, err := strconv.ParseInt(mwcs, 10, 32)
		if err != nil {
			log.Warnf("invalid value for %s (%s) defaulting to %d: %s", EnvMigrationMaxWorkerCount, mwcs, MigrationMaxWorkerCount, err)
			return
		}
		// use value from environment
		log.Infof("migration worker count set from %s (%d)", EnvMigrationMaxWorkerCount, mwc)
		MigrationMaxWorkerCount = int(mwc)
		return
	}
	log.Infof("migration worker count: %d", MigrationMaxWorkerCount)
}

func DefaultUpgradeSchedule() stmgr.UpgradeSchedule {
	var us stmgr.UpgradeSchedule

	updates := []stmgr.Upgrade{{
		Height:    buildconstants.UpgradeBreezeHeight,
		Network:   network.Version1,
		Migration: UpgradeFaucetBurnRecovery,
	}, {
		Height:    buildconstants.UpgradeSmokeHeight,
		Network:   network.Version2,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeIgnitionHeight,
		Network:   network.Version3,
		Migration: UpgradeIgnition,
	}, {
		Height:    buildconstants.UpgradeRefuelHeight,
		Network:   network.Version3,
		Migration: UpgradeRefuel,
	}, {
		Height:    buildconstants.UpgradeAssemblyHeight,
		Network:   network.Version4,
		Expensive: true,
		Migration: UpgradeActorsV2,
	}, {
		Height:    buildconstants.UpgradeTapeHeight,
		Network:   network.Version5,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeLiftoffHeight,
		Network:   network.Version5,
		Migration: UpgradeLiftoff,
	}, {
		Height:    buildconstants.UpgradeKumquatHeight,
		Network:   network.Version6,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeCalicoHeight,
		Network:   network.Version7,
		Migration: UpgradeCalico,
	}, {
		Height:    buildconstants.UpgradePersianHeight,
		Network:   network.Version8,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeOrangeHeight,
		Network:   network.Version9,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeTrustHeight,
		Network:   network.Version10,
		Migration: UpgradeActorsV3,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV3,
			StartWithin:     120,
			DontStartWithin: 60,
			StopWithin:      35,
		}, {
			PreMigration:    PreUpgradeActorsV3,
			StartWithin:     30,
			DontStartWithin: 15,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeNorwegianHeight,
		Network:   network.Version11,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeTurboHeight,
		Network:   network.Version12,
		Migration: UpgradeActorsV4,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV4,
			StartWithin:     120,
			DontStartWithin: 60,
			StopWithin:      35,
		}, {
			PreMigration:    PreUpgradeActorsV4,
			StartWithin:     30,
			DontStartWithin: 15,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeHyperdriveHeight,
		Network:   network.Version13,
		Migration: UpgradeActorsV5,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV5,
			StartWithin:     120,
			DontStartWithin: 60,
			StopWithin:      35,
		}, {
			PreMigration:    PreUpgradeActorsV5,
			StartWithin:     30,
			DontStartWithin: 15,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeChocolateHeight,
		Network:   network.Version14,
		Migration: UpgradeActorsV6,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV6,
			StartWithin:     120,
			DontStartWithin: 60,
			StopWithin:      35,
		}, {
			PreMigration:    PreUpgradeActorsV6,
			StartWithin:     30,
			DontStartWithin: 15,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeOhSnapHeight,
		Network:   network.Version15,
		Migration: UpgradeActorsV7,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV7,
			StartWithin:     180,
			DontStartWithin: 60,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeSkyrHeight,
		Network:   network.Version16,
		Migration: UpgradeActorsV8,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV8,
			StartWithin:     180,
			DontStartWithin: 60,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeSharkHeight,
		Network:   network.Version17,
		Migration: UpgradeActorsV9,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV9,
			StartWithin:     240,
			DontStartWithin: 60,
			StopWithin:      20,
		}, {
			PreMigration:    PreUpgradeActorsV9,
			StartWithin:     15,
			DontStartWithin: 10,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeHyggeHeight,
		Network:   network.Version18,
		Migration: UpgradeActorsV10,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV10,
			StartWithin:     60,
			DontStartWithin: 10,
			StopWithin:      5,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeLightningHeight,
		Network:   network.Version19,
		Migration: UpgradeActorsV11,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV11,
			StartWithin:     120,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeThunderHeight,
		Network:   network.Version20,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeWatermelonHeight,
		Network:   network.Version21,
		Migration: UpgradeActorsV12,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV12,
			StartWithin:     180,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeWatermelonFixHeight,
		Network:   network.Version21,
		Migration: buildUpgradeActorsV12MinerFix(calibnetv12BuggyMinerCID1, calibnetv12BuggyManifestCID2),
	}, {
		Height:    buildconstants.UpgradeWatermelonFix2Height,
		Network:   network.Version21,
		Migration: buildUpgradeActorsV12MinerFix(calibnetv12BuggyMinerCID2, calibnetv12CorrectManifestCID1),
	}, {
		Height:    buildconstants.UpgradeDragonHeight,
		Network:   network.Version22,
		Migration: UpgradeActorsV13,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV13,
			StartWithin:     120,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeCalibrationDragonFixHeight,
		Network:   network.Version22,
		Migration: upgradeActorsV13VerifregFix(calibnetv13BuggyVerifregCID1, calibnetv13CorrectManifestCID1),
	}, {
		Height:    buildconstants.UpgradeWaffleHeight,
		Network:   network.Version23,
		Migration: UpgradeActorsV14,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV14,
			StartWithin:     120,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeTuktukHeight,
		Network:   network.Version24,
		Migration: UpgradeActorsV15,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV15,
			StartWithin:     120,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeTeepHeight,
		Network:   network.Version25,
		Migration: UpgradeActorsV16,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV16,
			StartWithin:     120,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeTockHeight,
		Network:   network.Version26,
		Migration: nil,
	}, {
		Height:    buildconstants.UpgradeTockFixHeight,
		Network:   network.Version26,
		Migration: UpgradeActorsV16Fix,
	}, {
		Height:    buildconstants.UpgradeGoldenWeekHeight,
		Network:   network.Version27,
		Migration: UpgradeActorsV17,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV17,
			StartWithin:     120,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	}, {
		Height:    buildconstants.UpgradeXxHeight,
		Network:   network.Version28,
		Migration: UpgradeActorsV18,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV18,
			StartWithin:     120,
			DontStartWithin: 15,
			StopWithin:      10,
		}},
		Expensive: true,
	},
	}

	for _, u := range updates {
		if u.Height < 0 {
			// upgrade disabled
			continue
		}
		us = append(us, u)
	}
	return us
}

func UpgradeFaucetBurnRecovery(ctx context.Context, sm *stmgr.StateManager, _ stmgr.MigrationCache, em stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Some initial parameters
	FundsForMiners := types.FromFil(1_000_000)
	LookbackEpoch := abi.ChainEpoch(32000)
	AccountCap := types.FromFil(0)
	BaseMinerBalance := types.FromFil(20)
	DesiredReimbursementBalance := types.FromFil(5_000_000)

	isSystemAccount := func(addr address.Address) (bool, error) {
		id, err := address.IDFromAddress(addr)
		if err != nil {
			return false, xerrors.Errorf("id address: %w", err)
		}

		if id < 1000 {
			return true, nil
		}
		return false, nil
	}

	minerFundsAlloc := func(pow, tpow abi.StoragePower) abi.TokenAmount {
		return types.BigDiv(types.BigMul(pow, FundsForMiners), tpow)
	}

	// Grab lookback state for account checks
	lbts, err := sm.ChainStore().GetTipsetByHeight(ctx, LookbackEpoch, ts, false)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get tipset at lookback height: %w", err)
	}

	lbtree, err := sm.ParentState(lbts)
	if err != nil {
		return cid.Undef, xerrors.Errorf("loading state tree failed: %w", err)
	}

	tree, err := sm.StateTree(root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
	}

	type transfer struct {
		From address.Address
		To   address.Address
		Amt  abi.TokenAmount
	}

	var transfers []transfer
	subcalls := make([]types.ExecutionTrace, 0)
	transferCb := func(trace types.ExecutionTrace) {
		subcalls = append(subcalls, trace)
	}

	// Take all excess funds away, put them into the reserve account
	err = tree.ForEach(func(addr address.Address, act *types.Actor) error {
		switch act.Code {
		case builtin0.AccountActorCodeID, builtin0.MultisigActorCodeID, builtin0.PaymentChannelActorCodeID:
			sysAcc, err := isSystemAccount(addr)
			if err != nil {
				return xerrors.Errorf("checking system account: %w", err)
			}

			if !sysAcc {
				transfers = append(transfers, transfer{
					From: addr,
					To:   builtin.ReserveAddress,
					Amt:  act.Balance,
				})
			}
		case builtin0.StorageMinerActorCodeID:
			var st miner0.State
			if err := sm.ChainStore().ActorStore(ctx).Get(ctx, act.Head, &st); err != nil {
				return xerrors.Errorf("failed to load miner state: %w", err)
			}

			var available abi.TokenAmount
			{
				defer func() {
					if err := recover(); err != nil {
						log.Warnf("Get available balance failed (%s, %s, %s): %s", addr, act.Head, act.Balance, err)
					}
					available = abi.NewTokenAmount(0)
				}()
				// this panics if the miner doesn't have enough funds to cover their locked pledge
				available = st.GetAvailableBalance(act.Balance)
			}

			if !available.IsZero() {
				transfers = append(transfers, transfer{
					From: addr,
					To:   builtin.ReserveAddress,
					Amt:  available,
				})
			}
		}
		return nil
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("foreach over state tree failed: %w", err)
	}

	// Execute transfers from previous step
	for _, t := range transfers {
		if err := stmgr.DoTransfer(tree, t.From, t.To, t.Amt, transferCb); err != nil {
			return cid.Undef, xerrors.Errorf("transfer %s %s->%s failed: %w", t.Amt, t.From, t.To, err)
		}
	}

	// pull up power table to give miners back some funds proportional to their power
	var ps power0.State
	powAct, err := tree.GetActor(builtin0.StoragePowerActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to load power actor: %w", err)
	}

	cst := cbor.NewCborStore(sm.ChainStore().StateBlockstore())
	if err := cst.Get(ctx, powAct.Head, &ps); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get power actor state: %w", err)
	}

	totalPower := ps.TotalBytesCommitted

	var transfersBack []transfer
	// Now, we return some funds to places where they are needed
	err = tree.ForEach(func(addr address.Address, act *types.Actor) error {
		lbact, err := lbtree.GetActor(addr)
		if err != nil {
			if !errors.Is(err, types.ErrActorNotFound) {
				return xerrors.Errorf("failed to get actor in lookback state")
			}
		}

		prevBalance := abi.NewTokenAmount(0)
		if lbact != nil {
			prevBalance = lbact.Balance
		}

		switch act.Code {
		case builtin0.AccountActorCodeID, builtin0.MultisigActorCodeID, builtin0.PaymentChannelActorCodeID:
			nbalance := big.Min(prevBalance, AccountCap)
			if nbalance.Sign() != 0 {
				transfersBack = append(transfersBack, transfer{
					From: builtin.ReserveAddress,
					To:   addr,
					Amt:  nbalance,
				})
			}
		case builtin0.StorageMinerActorCodeID:
			var st miner0.State
			if err := sm.ChainStore().ActorStore(ctx).Get(ctx, act.Head, &st); err != nil {
				return xerrors.Errorf("failed to load miner state: %w", err)
			}

			var minfo miner0.MinerInfo
			if err := cst.Get(ctx, st.Info, &minfo); err != nil {
				return xerrors.Errorf("failed to get miner info: %w", err)
			}

			sectorsArr, err := adt0.AsArray(sm.ChainStore().ActorStore(ctx), st.Sectors)
			if err != nil {
				return xerrors.Errorf("failed to load sectors array: %w", err)
			}

			slen := sectorsArr.Length()

			power := types.BigMul(types.NewInt(slen), types.NewInt(uint64(minfo.SectorSize)))

			mfunds := minerFundsAlloc(power, totalPower)
			transfersBack = append(transfersBack, transfer{
				From: builtin.ReserveAddress,
				To:   minfo.Worker,
				Amt:  mfunds,
			})

			// Now make sure to give each miner who had power at the lookback some FIL
			lbact, err := lbtree.GetActor(addr)
			if err == nil {
				var lbst miner0.State
				if err := sm.ChainStore().ActorStore(ctx).Get(ctx, lbact.Head, &lbst); err != nil {
					return xerrors.Errorf("failed to load miner state: %w", err)
				}

				lbsectors, err := adt0.AsArray(sm.ChainStore().ActorStore(ctx), lbst.Sectors)
				if err != nil {
					return xerrors.Errorf("failed to load lb sectors array: %w", err)
				}

				if lbsectors.Length() > 0 {
					transfersBack = append(transfersBack, transfer{
						From: builtin.ReserveAddress,
						To:   minfo.Worker,
						Amt:  BaseMinerBalance,
					})
				}

			} else {
				log.Warnf("failed to get miner in lookback state: %s", err)
			}
		}
		return nil
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("foreach over state tree failed: %w", err)
	}

	for _, t := range transfersBack {
		if err := stmgr.DoTransfer(tree, t.From, t.To, t.Amt, transferCb); err != nil {
			return cid.Undef, xerrors.Errorf("transfer %s %s->%s failed: %w", t.Amt, t.From, t.To, err)
		}
	}

	// transfer all burnt funds back to the reserve account
	burntAct, err := tree.GetActor(builtin0.BurntFundsActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to load burnt funds actor: %w", err)
	}
	if err := stmgr.DoTransfer(tree, builtin0.BurntFundsActorAddr, builtin.ReserveAddress, burntAct.Balance, transferCb); err != nil {
		return cid.Undef, xerrors.Errorf("failed to unburn funds: %w", err)
	}

	// Top up the reimbursement service
	reimbAddr, err := address.NewFromString("t0111")
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to parse reimbursement service address")
	}

	reimb, err := tree.GetActor(reimbAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to load reimbursement account actor: %w", err)
	}

	difference := types.BigSub(DesiredReimbursementBalance, reimb.Balance)
	if err := stmgr.DoTransfer(tree, builtin.ReserveAddress, reimbAddr, difference, transferCb); err != nil {
		return cid.Undef, xerrors.Errorf("failed to top up reimbursement account: %w", err)
	}

	// Now, a final sanity check to make sure the balances all check out
	total := abi.NewTokenAmount(0)
	err = tree.ForEach(func(addr address.Address, act *types.Actor) error {
		total = types.BigAdd(total, act.Balance)
		return nil
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("checking final state balance failed: %w", err)
	}

	exp := types.FromFil(buildconstants.FilBase)
	if !exp.Equals(total) {
		return cid.Undef, xerrors.Errorf("resultant state tree account balance was not correct: %s", total)
	}

	if em != nil {
		// record the transfer in execution traces

		fakeMsg := stmgr.MakeFakeMsg(builtin.SystemActorAddr, builtin.SystemActorAddr, big.Zero(), uint64(epoch))

		if err := em.MessageApplied(ctx, ts, fakeMsg.Cid(), fakeMsg, &vm.ApplyRet{
			MessageReceipt: *stmgr.MakeFakeRct(),
			ActorErr:       nil,
			ExecutionTrace: types.ExecutionTrace{
				Msg: types.MessageTrace{
					To:   fakeMsg.To,
					From: fakeMsg.From,
				},
				Subcalls: subcalls,
			},
			Duration: 0,
			GasCosts: nil,
		}, false); err != nil {
			return cid.Undef, xerrors.Errorf("recording transfers: %w", err)
		}
	}

	return tree.Flush(ctx)
}

func UpgradeIgnition(ctx context.Context, sm *stmgr.StateManager, _ stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	store := sm.ChainStore().ActorStore(ctx)

	if buildconstants.UpgradeLiftoffHeight <= epoch {
		return cid.Undef, xerrors.Errorf("liftoff height must be beyond ignition height")
	}

	nst, err := nv3.MigrateStateTree(ctx, store, root, epoch)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors state: %w", err)
	}

	tree, err := sm.StateTree(nst)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
	}

	err = stmgr.SetNetworkName(ctx, store, tree, "ignition")
	if err != nil {
		return cid.Undef, xerrors.Errorf("setting network name: %w", err)
	}

	split1, err := address.NewFromString("t0115")
	if err != nil {
		return cid.Undef, xerrors.Errorf("first split address: %w", err)
	}

	split2, err := address.NewFromString("t0116")
	if err != nil {
		return cid.Undef, xerrors.Errorf("second split address: %w", err)
	}

	err = resetGenesisMsigs0(ctx, sm, store, tree, buildconstants.UpgradeLiftoffHeight)
	if err != nil {
		return cid.Undef, xerrors.Errorf("resetting genesis msig start epochs: %w", err)
	}

	err = splitGenesisMultisig0(ctx, cb, split1, store, tree, 50, epoch, ts)
	if err != nil {
		return cid.Undef, xerrors.Errorf("splitting first msig: %w", err)
	}

	err = splitGenesisMultisig0(ctx, cb, split2, store, tree, 50, epoch, ts)
	if err != nil {
		return cid.Undef, xerrors.Errorf("splitting second msig: %w", err)
	}

	err = nv3.CheckStateTree(ctx, store, nst, epoch, builtin0.TotalFilecoin)
	if err != nil {
		return cid.Undef, xerrors.Errorf("sanity check after ignition upgrade failed: %w", err)
	}

	return tree.Flush(ctx)
}

func splitGenesisMultisig0(ctx context.Context, em stmgr.ExecMonitor, addr address.Address, store adt0.Store, tree *state.StateTree, portions uint64, epoch abi.ChainEpoch, ts *types.TipSet) error {
	if portions < 1 {
		return xerrors.Errorf("cannot split into 0 portions")
	}

	mact, err := tree.GetActor(addr)
	if err != nil {
		return xerrors.Errorf("getting msig actor: %w", err)
	}

	mst, err := multisig.Load(store, mact)
	if err != nil {
		return xerrors.Errorf("getting msig state: %w", err)
	}

	signers, err := mst.Signers()
	if err != nil {
		return xerrors.Errorf("getting msig signers: %w", err)
	}

	thresh, err := mst.Threshold()
	if err != nil {
		return xerrors.Errorf("getting msig threshold: %w", err)
	}

	ibal, err := mst.InitialBalance()
	if err != nil {
		return xerrors.Errorf("getting msig initial balance: %w", err)
	}

	se, err := mst.StartEpoch()
	if err != nil {
		return xerrors.Errorf("getting msig start epoch: %w", err)
	}

	ud, err := mst.UnlockDuration()
	if err != nil {
		return xerrors.Errorf("getting msig unlock duration: %w", err)
	}

	pending, err := adt0.MakeEmptyMap(store).Root()
	if err != nil {
		return xerrors.Errorf("failed to create empty map: %w", err)
	}

	newIbal := big.Div(ibal, types.NewInt(portions))
	newState := &multisig0.State{
		Signers:               signers,
		NumApprovalsThreshold: thresh,
		NextTxnID:             0,
		InitialBalance:        newIbal,
		StartEpoch:            se,
		UnlockDuration:        ud,
		PendingTxns:           pending,
	}

	scid, err := store.Put(ctx, newState)
	if err != nil {
		return xerrors.Errorf("storing new state: %w", err)
	}

	newActor := types.Actor{
		Code:    builtin0.MultisigActorCodeID,
		Head:    scid,
		Nonce:   0,
		Balance: big.Zero(),
	}

	i := uint64(0)
	subcalls := make([]types.ExecutionTrace, 0, portions)
	transferCb := func(trace types.ExecutionTrace) {
		subcalls = append(subcalls, trace)
	}

	for i < portions {
		keyAddr, err := stmgr.MakeKeyAddr(addr, i)
		if err != nil {
			return xerrors.Errorf("creating key address: %w", err)
		}

		idAddr, err := tree.RegisterNewAddress(keyAddr)
		if err != nil {
			return xerrors.Errorf("registering new address: %w", err)
		}

		err = tree.SetActor(idAddr, &newActor)
		if err != nil {
			return xerrors.Errorf("setting new msig actor state: %w", err)
		}

		if err := stmgr.DoTransfer(tree, addr, idAddr, newIbal, transferCb); err != nil {
			return xerrors.Errorf("transferring split msig balance: %w", err)
		}

		i++
	}

	if em != nil {
		// record the transfer in execution traces

		fakeMsg := stmgr.MakeFakeMsg(builtin.SystemActorAddr, addr, big.Zero(), uint64(epoch))

		if err := em.MessageApplied(ctx, ts, fakeMsg.Cid(), fakeMsg, &vm.ApplyRet{
			MessageReceipt: *stmgr.MakeFakeRct(),
			ActorErr:       nil,
			ExecutionTrace: types.ExecutionTrace{
				Msg: types.MessageTrace{
					From: fakeMsg.From,
					To:   fakeMsg.To,
				},
				Subcalls: subcalls,
			},
			Duration: 0,
			GasCosts: nil,
		}, false); err != nil {
			return xerrors.Errorf("recording transfers: %w", err)
		}
	}

	return nil
}

// TODO: After the Liftoff epoch, refactor this to use resetMultisigVesting
func resetGenesisMsigs0(ctx context.Context, sm *stmgr.StateManager, store adt0.Store, tree *state.StateTree, startEpoch abi.ChainEpoch) error {
	gb, err := sm.ChainStore().GetGenesis(ctx)
	if err != nil {
		return xerrors.Errorf("getting genesis block: %w", err)
	}

	gts, err := types.NewTipSet([]*types.BlockHeader{gb})
	if err != nil {
		return xerrors.Errorf("getting genesis tipset: %w", err)
	}

	cst := cbor.NewCborStore(sm.ChainStore().StateBlockstore())
	genesisTree, err := state.LoadStateTree(cst, gts.ParentState())
	if err != nil {
		return xerrors.Errorf("loading state tree: %w", err)
	}

	err = genesisTree.ForEach(func(addr address.Address, genesisActor *types.Actor) error {
		if genesisActor.Code == builtin0.MultisigActorCodeID {
			currActor, err := tree.GetActor(addr)
			if err != nil {
				return xerrors.Errorf("loading actor: %w", err)
			}

			var currState multisig0.State
			if err := store.Get(ctx, currActor.Head, &currState); err != nil {
				return xerrors.Errorf("reading multisig state: %w", err)
			}

			currState.StartEpoch = startEpoch

			currActor.Head, err = store.Put(ctx, &currState)
			if err != nil {
				return xerrors.Errorf("writing new multisig state: %w", err)
			}

			if err := tree.SetActor(addr, currActor); err != nil {
				return xerrors.Errorf("setting multisig actor: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		return xerrors.Errorf("iterating over genesis actors: %w", err)
	}

	return nil
}

func resetMultisigVesting0(ctx context.Context, store adt0.Store, tree *state.StateTree, addr address.Address, startEpoch abi.ChainEpoch, duration abi.ChainEpoch, balance abi.TokenAmount) error {
	act, err := tree.GetActor(addr)
	if err != nil {
		return xerrors.Errorf("getting actor: %w", err)
	}

	if !builtin.IsMultisigActor(act.Code) {
		return xerrors.Errorf("actor wasn't msig: %w", err)
	}

	var msigState multisig0.State
	if err := store.Get(ctx, act.Head, &msigState); err != nil {
		return xerrors.Errorf("reading multisig state: %w", err)
	}

	msigState.StartEpoch = startEpoch
	msigState.UnlockDuration = duration
	msigState.InitialBalance = balance

	act.Head, err = store.Put(ctx, &msigState)
	if err != nil {
		return xerrors.Errorf("writing new multisig state: %w", err)
	}

	if err := tree.SetActor(addr, act); err != nil {
		return xerrors.Errorf("setting multisig actor: %w", err)
	}

	return nil
}

func UpgradeRefuel(ctx context.Context, sm *stmgr.StateManager, _ stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {

	store := sm.ChainStore().ActorStore(ctx)
	tree, err := sm.StateTree(root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
	}

	err = resetMultisigVesting0(ctx, store, tree, builtin.SaftAddress, 0, 0, big.Zero())
	if err != nil {
		return cid.Undef, xerrors.Errorf("tweaking msig vesting: %w", err)
	}

	err = resetMultisigVesting0(ctx, store, tree, builtin.ReserveAddress, 0, 0, big.Zero())
	if err != nil {
		return cid.Undef, xerrors.Errorf("tweaking msig vesting: %w", err)
	}

	err = resetMultisigVesting0(ctx, store, tree, builtin.RootVerifierAddress, 0, 0, big.Zero())
	if err != nil {
		return cid.Undef, xerrors.Errorf("tweaking msig vesting: %w", err)
	}

	return tree.Flush(ctx)
}

func UpgradeActorsV2(ctx context.Context, sm *stmgr.StateManager, _ stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)

	info, err := store.Put(ctx, new(types.StateInfo0))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create new state info for actors v2: %w", err)
	}

	newHamtRoot, err := nv4.MigrateStateTree(ctx, store, root, epoch, nv4.DefaultConfig())
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v2: %w", err)
	}

	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion1,
		Actors:  newHamtRoot,
		Info:    info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// perform some basic sanity checks to make sure everything still works.
	if newSm, err := state.LoadStateTree(store, newRoot); err != nil {
		return cid.Undef, xerrors.Errorf("state tree sanity load failed: %w", err)
	} else if newRoot2, err := newSm.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("state tree sanity flush failed: %w", err)
	} else if newRoot2 != newRoot {
		return cid.Undef, xerrors.Errorf("state-root mismatch: %s != %s", newRoot, newRoot2)
	} else if _, err := newSm.GetActor(builtin0.InitActorAddr); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load init actor after upgrade: %w", err)
	}

	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

func UpgradeLiftoff(ctx context.Context, sm *stmgr.StateManager, _ stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	tree, err := sm.StateTree(root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
	}

	err = stmgr.SetNetworkName(ctx, sm.ChainStore().ActorStore(ctx), tree, "mainnet")
	if err != nil {
		return cid.Undef, xerrors.Errorf("setting network name: %w", err)
	}

	return tree.Flush(ctx)
}

func UpgradeCalico(ctx context.Context, sm *stmgr.StateManager, _ stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	if buildconstants.BuildType != buildconstants.BuildMainnet {
		return root, nil
	}

	store := sm.ChainStore().ActorStore(ctx)
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion1 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 1 for calico upgrade, got %d",
			stateRoot.Version,
		)
	}

	newHamtRoot, err := nv7.MigrateStateTree(ctx, store, stateRoot.Actors, epoch, nv7.DefaultConfig())
	if err != nil {
		return cid.Undef, xerrors.Errorf("running nv7 migration: %w", err)
	}

	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: stateRoot.Version,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// perform some basic sanity checks to make sure everything still works.
	if newSm, err := state.LoadStateTree(store, newRoot); err != nil {
		return cid.Undef, xerrors.Errorf("state tree sanity load failed: %w", err)
	} else if newRoot2, err := newSm.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("state tree sanity flush failed: %w", err)
	} else if newRoot2 != newRoot {
		return cid.Undef, xerrors.Errorf("state-root mismatch: %s != %s", newRoot, newRoot2)
	} else if _, err := newSm.GetActor(builtin0.InitActorAddr); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load init actor after upgrade: %w", err)
	}

	return newRoot, nil
}

func UpgradeActorsV3(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := nv10.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}
	newRoot, err := upgradeActorsV3Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v3 state: %w", err)
	}

	tree, err := sm.StateTree(newRoot)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
	}

	if buildconstants.BuildType == buildconstants.BuildMainnet {
		err := stmgr.TerminateActor(ctx, tree, buildconstants.ZeroAddress, cb, epoch, ts)
		if err != nil && !errors.Is(err, types.ErrActorNotFound) {
			return cid.Undef, xerrors.Errorf("deleting zero bls actor: %w", err)
		}

		newRoot, err = tree.Flush(ctx)
		if err != nil {
			return cid.Undef, xerrors.Errorf("flushing state tree: %w", err)
		}
	}

	return newRoot, nil
}

func PreUpgradeActorsV3(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}
	config := nv10.Config{MaxWorkers: uint(workerCount)}
	_, err := upgradeActorsV3Common(ctx, sm, cache, root, epoch, ts, config)
	return err
}

func upgradeActorsV3Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config nv10.Config,
) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion1 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 1 for actors v3 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// Perform the migration
	newHamtRoot, err := nv10.MigrateStateTree(ctx, store, stateRoot.Actors, epoch, config, migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v3: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion2,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persist the new tree.

	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

func UpgradeActorsV4(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := nv12.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}

	newRoot, err := upgradeActorsV4Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v4 state: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV4(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}
	config := nv12.Config{MaxWorkers: uint(workerCount)}
	_, err := upgradeActorsV4Common(ctx, sm, cache, root, epoch, ts, config)
	return err
}

func upgradeActorsV4Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config nv12.Config,
) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion2 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 2 for actors v4 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// Perform the migration
	newHamtRoot, err := nv12.MigrateStateTree(ctx, store, stateRoot.Actors, epoch, config, migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v4: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion3,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persist the new tree.

	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

func UpgradeActorsV5(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := nv13.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}

	newRoot, err := upgradeActorsV5Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v5 state: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV5(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}
	config := nv13.Config{MaxWorkers: uint(workerCount)}
	_, err := upgradeActorsV5Common(ctx, sm, cache, root, epoch, ts, config)
	return err
}

func upgradeActorsV5Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config nv13.Config,
) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion3 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 3 for actors v5 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// Perform the migration
	newHamtRoot, err := nv13.MigrateStateTree(ctx, store, stateRoot.Actors, epoch, config, migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v5: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion4,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persist the new tree.

	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

func UpgradeActorsV6(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := nv14.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}

	newRoot, err := upgradeActorsV6Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v5 state: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV6(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}
	config := nv14.Config{MaxWorkers: uint(workerCount)}
	_, err := upgradeActorsV6Common(ctx, sm, cache, root, epoch, ts, config)
	return err
}

func upgradeActorsV6Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config nv14.Config,
) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion4 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 4 for actors v6 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// Perform the migration
	newHamtRoot, err := nv14.MigrateStateTree(ctx, store, stateRoot.Actors, epoch, config, migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v6: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion4,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persist the new tree.

	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

func UpgradeActorsV7(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := nv15.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}

	newRoot, err := upgradeActorsV7Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v6 state: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV7(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := nv15.Config{MaxWorkers: uint(workerCount),
		ProgressLogPeriod: time.Minute * 5}

	_, err = upgradeActorsV7Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func upgradeActorsV7Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config nv15.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	// TODO: pretty sure we'd achieve nothing by doing this, confirm in review
	//buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), writeStore)
	store := store.ActorStore(ctx, writeStore)
	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion4 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 4 for actors v7 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// Perform the migration
	newHamtRoot, err := nv15.MigrateStateTree(ctx, store, stateRoot.Actors, epoch, config, migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v7: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion4,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

func UpgradeActorsV8(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := nv16.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}

	newRoot, err := upgradeActorsV8Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v7 state: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV8(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := nv16.Config{MaxWorkers: uint(workerCount),
		ProgressLogPeriod: time.Minute * 5}

	_, err = upgradeActorsV8Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func upgradeActorsV8Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config nv16.Config,
) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)

	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, buf, actorstypes.Version8); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion4 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 4 for actors v8 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version8)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v8 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv16.MigrateStateTree(ctx, store, manifest, stateRoot.Actors, epoch, config, migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v8: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion4,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persist the new tree.

	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

func UpgradeActorsV9(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := nv17.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}

	newRoot, err := upgradeActorsV9Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v8 state: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV9(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid,
	epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := nv17.Config{MaxWorkers: uint(workerCount),
		ProgressLogPeriod: time.Minute * 5}

	_, err = upgradeActorsV9Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func upgradeActorsV9Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config nv17.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	store := store.ActorStore(ctx, writeStore)

	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, sm.ChainStore().StateBlockstore(), actorstypes.Version9); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion4 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 4 for actors v9 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version9)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v9 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv17.MigrateStateTree(ctx, store, manifest, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v9: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion4,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV10(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: time.Minute * 5,
	}

	_, err = upgradeActorsV10Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV10(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 3.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}

	newRoot, err := upgradeActorsV10Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v10 state: %w", err)
	}

	return newRoot, nil
}

func upgradeActorsV10Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)

	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, sm.ChainStore().StateBlockstore(), actorstypes.Version10); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion4 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 4 for actors v10 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version10)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v10 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv18.MigrateStateTree(ctx, store, manifest, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v10: %w", err)
	}

	// Persist the result.
	newRoot, err := store.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persist the new tree.

	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

func PreUpgradeActorsV11(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: time.Minute * 5,
	}

	_, err = upgradeActorsV11Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV11(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}
	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}
	newRoot, err := upgradeActorsV11Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v11 state: %w", err)
	}
	return newRoot, nil
}

func upgradeActorsV11Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)
	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version11); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v11 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version11)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v11 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv19.MigrateStateTree(ctx, adtStore, manifest, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v11: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV12(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: time.Minute * 5,
	}

	_, err = upgradeActorsV12Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV12(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}
	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}
	newRoot, err := upgradeActorsV12Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v12 state: %w", err)
	}
	return newRoot, nil
}

var (
	calibnetv12BuggyMinerCID1 = cid.MustParse("bafk2bzacecnh2ouohmonvebq7uughh4h3ppmg4cjsk74dzxlbbtlcij4xbzxq")
	calibnetv12BuggyMinerCID2 = cid.MustParse("bafk2bzaced7emkbbnrewv5uvrokxpf5tlm4jslu2jsv77ofw2yqdglg657uie")

	calibnetv12BuggyBundleSuffix1 = "calibrationnet-12-rc1"
	calibnetv12BuggyBundleSuffix2 = "calibrationnet-12-rc2"

	calibnetv12BuggyManifestCID1   = cid.MustParse("bafy2bzacedrunxfqta5skb7q7x32lnp4efz2oq7fn226ffm7fu5iqs62jkmvs")
	calibnetv12BuggyManifestCID2   = cid.MustParse("bafy2bzacebl4w5ptfvuw6746w7ev562idkbf5ppq72e6zub22435ws2rukzru")
	calibnetv12CorrectManifestCID1 = cid.MustParse("bafy2bzacednzb3pkrfnbfhmoqtb3bc6dgvxszpqklf3qcc7qzcage4ewzxsca")

	calibnetv13BuggyVerifregCID1 = cid.MustParse("bafk2bzacednskl3bykz5qpo54z2j2p4q44t5of4ktd6vs6ymmg2zebsbxazkm")

	calibnetv13BuggyBundleSuffix1 = "calibrationnet-13-rc3"

	calibnetv13BuggyManifestCID1   = cid.MustParse("bafy2bzacea4firkyvt2zzdwqjrws5pyeluaesh6uaid246tommayr4337xpmi")
	calibnetv13CorrectManifestCID1 = cid.MustParse("bafy2bzacect4ktyujrwp6mjlsitnpvuw2pbuppz6w52sfljyo4agjevzm75qs")

	// Some v16.0.0 bundles are included in the v16 bundles tarball along with the v16.0.1 bundles.
	// But the v16.0.0 ones have a -v16.0.0.car suffix instead of just .car.
	v1600BundleSuffix = "v16.0.0"
)

func upgradeActorsV12Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)
	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version12); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v12 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// check whether or not this is a calibnet upgrade
	// we do this because calibnet upgraded to a "wrong" actors bundle, which was then corrected
	// we thus upgrade to calibrationnet-buggy in this upgrade
	actorsIn, err := state.LoadStateTree(adtStore, root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("loading state tree: %w", err)
	}

	initActor, err := actorsIn.GetActor(builtin.InitActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
	}

	var initState init11.State
	if err := adtStore.Get(ctx, initActor.Head, &initState); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
	}

	var manifestCid cid.Cid
	if initState.NetworkName == "calibrationnet" {
		embedded, ok := build.GetEmbeddedBuiltinActorsBundle(actorstypes.Version12, calibnetv12BuggyBundleSuffix1)
		if !ok {
			return cid.Undef, xerrors.Errorf("didn't find buggy calibrationnet bundle")
		}

		var err error
		manifestCid, err = bundle.LoadBundle(ctx, writeStore, bytes.NewReader(embedded))
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to load buggy calibnet bundle: %w", err)
		}

		if manifestCid != calibnetv12BuggyManifestCID1 {
			return cid.Undef, xerrors.Errorf("didn't find expected buggy calibnet bundle manifest: %s != %s", manifestCid, calibnetv12BuggyManifestCID1)
		}
	} else {
		ok := false
		manifestCid, ok = actors.GetManifest(actorstypes.Version12)
		if !ok {
			return cid.Undef, xerrors.Errorf("no manifest CID for v12 upgrade")
		}
	}

	// Perform the migration
	newHamtRoot, err := nv21.MigrateStateTree(ctx, adtStore, manifestCid, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v12: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

// ////////////////////
func buildUpgradeActorsV12MinerFix(oldBuggyMinerCID, newManifestCID cid.Cid) func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	return func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
		stateStore := sm.ChainStore().StateBlockstore()
		adtStore := store.ActorStore(ctx, stateStore)

		// ensure that the manifest is loaded in the blockstore

		// this loads the "correct" bundle for UpgradeWatermelonFix2Height
		if err := bundle.LoadBundles(ctx, stateStore, actorstypes.Version12); err != nil {
			return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
		}

		// this loads the second buggy bundle, for UpgradeWatermelonFixHeight
		embedded, ok := build.GetEmbeddedBuiltinActorsBundle(actorstypes.Version12, calibnetv12BuggyBundleSuffix2)
		if !ok {
			return cid.Undef, xerrors.Errorf("didn't find buggy calibrationnet bundle")
		}

		_, err := bundle.LoadBundle(ctx, stateStore, bytes.NewReader(embedded))
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to load buggy calibnet bundle: %w", err)
		}

		// now confirm we have the one we're migrating to
		if haveManifest, err := stateStore.Has(ctx, newManifestCID); err != nil {
			return cid.Undef, xerrors.Errorf("blockstore error when loading manifest %s: %w", newManifestCID, err)
		} else if !haveManifest {
			return cid.Undef, xerrors.Errorf("missing new manifest %s in blockstore", newManifestCID)
		}

		// Load input state tree
		actorsIn, err := state.LoadStateTree(adtStore, root)
		if err != nil {
			return cid.Undef, xerrors.Errorf("loading state tree: %w", err)
		}

		// load old manifest data
		systemActor, err := actorsIn.GetActor(builtin.SystemActorAddr)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
		}

		var systemState system11.State
		if err := adtStore.Get(ctx, systemActor.Head, &systemState); err != nil {
			return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
		}

		var oldManifestData manifest.ManifestData
		if err := adtStore.Get(ctx, systemState.BuiltinActors, &oldManifestData); err != nil {
			return cid.Undef, xerrors.Errorf("failed to get old manifest data: %w", err)
		}

		// load new manifest
		var newManifest manifest.Manifest
		if err := adtStore.Get(ctx, newManifestCID, &newManifest); err != nil {
			return cid.Undef, xerrors.Errorf("error reading actor manifest: %w", err)
		}

		if err := newManifest.Load(ctx, adtStore); err != nil {
			return cid.Undef, xerrors.Errorf("error loading actor manifest: %w", err)
		}

		// build the CID mapping
		codeMapping := make(map[cid.Cid]cid.Cid, len(oldManifestData.Entries))
		for _, oldEntry := range oldManifestData.Entries {
			newCID, ok := newManifest.Get(oldEntry.Name)
			if !ok {
				return cid.Undef, xerrors.Errorf("missing manifest entry for %s", oldEntry.Name)
			}

			// Note: we expect newCID to be the same as oldEntry.Code for all actors except the miner actor
			codeMapping[oldEntry.Code] = newCID
		}

		// Create empty actorsOut

		actorsOut, err := state.NewStateTree(adtStore, actorsIn.Version())
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to create new tree: %w", err)
		}

		// Perform the migration
		err = actorsIn.ForEach(func(a address.Address, actor *types.Actor) error {
			newCid, ok := codeMapping[actor.Code]
			if !ok {
				return xerrors.Errorf("didn't find mapping for %s", actor.Code)
			}

			return actorsOut.SetActor(a, &types.ActorV5{
				Code:             newCid,
				Head:             actor.Head,
				Nonce:            actor.Nonce,
				Balance:          actor.Balance,
				DelegatedAddress: actor.DelegatedAddress,
			})
		})
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to perform migration: %w", err)
		}

		systemState.BuiltinActors = newManifest.Data
		newSystemHead, err := adtStore.Put(ctx, &systemState)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to put new system state: %w", err)
		}

		systemActor.Head = newSystemHead
		if err = actorsOut.SetActor(builtin.SystemActorAddr, systemActor); err != nil {
			return cid.Undef, xerrors.Errorf("failed to put new system actor: %w", err)
		}

		// Sanity checking

		err = actorsIn.ForEach(func(a address.Address, inActor *types.Actor) error {
			outActor, err := actorsOut.GetActor(a)
			if err != nil {
				return xerrors.Errorf("failed to get actor in outTree: %w", err)
			}

			if inActor.Nonce != outActor.Nonce {
				return xerrors.Errorf("mismatched nonce for actor %s", a)
			}

			if !inActor.Balance.Equals(outActor.Balance) {
				return xerrors.Errorf("mismatched balance for actor %s: %d != %d", a, inActor.Balance, outActor.Balance)
			}

			if inActor.DelegatedAddress != outActor.DelegatedAddress && inActor.DelegatedAddress.String() != outActor.DelegatedAddress.String() {
				return xerrors.Errorf("mismatched address for actor %s: %s != %s", a, inActor.DelegatedAddress, outActor.DelegatedAddress)
			}

			if inActor.Head != outActor.Head && a != builtin.SystemActorAddr {
				return xerrors.Errorf("mismatched head for actor %s", a)
			}

			// Actor Codes are only expected to change for the miner actor
			if inActor.Code != oldBuggyMinerCID && inActor.Code != outActor.Code {
				return xerrors.Errorf("unexpected change in code for actor %s", a)
			}

			return nil
		})
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to sanity check migration: %w", err)
		}

		// Persist the result.
		newRoot, err := actorsOut.Flush(ctx)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
		}

		return newRoot, nil
	}
}

func PreUpgradeActorsV13(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: time.Minute * 5,
		UpgradeEpoch:      buildconstants.UpgradeDragonHeight,
	}

	_, err = upgradeActorsV13Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV13(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}
	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
		UpgradeEpoch:      buildconstants.UpgradeDragonHeight,
	}
	newRoot, err := upgradeActorsV13Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v13 state: %w", err)
	}
	return newRoot, nil
}

func upgradeActorsV13Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)
	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version13); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v13 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// check whether or not this is a calibnet upgrade
	// we do this because calibnet upgraded to a "wrong" actors bundle, which was then corrected
	// we thus upgrade to calibrationnet-buggy in this upgrade
	actorsIn, err := state.LoadStateTree(adtStore, root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("loading state tree: %w", err)
	}

	initActor, err := actorsIn.GetActor(builtin.InitActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
	}

	var initState init12.State
	if err := adtStore.Get(ctx, initActor.Head, &initState); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
	}

	var manifestCid cid.Cid
	if initState.NetworkName == "calibrationnet" {
		embedded, ok := build.GetEmbeddedBuiltinActorsBundle(actorstypes.Version13, calibnetv13BuggyBundleSuffix1)
		if !ok {
			return cid.Undef, xerrors.Errorf("didn't find buggy calibrationnet bundle")
		}

		var err error
		manifestCid, err = bundle.LoadBundle(ctx, writeStore, bytes.NewReader(embedded))
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to load buggy calibnet bundle: %w", err)
		}

		if manifestCid != calibnetv13BuggyManifestCID1 {
			return cid.Undef, xerrors.Errorf("didn't find expected buggy calibnet bundle manifest: %s != %s", manifestCid, calibnetv12BuggyManifestCID1)
		}
	} else {
		ok := false
		manifestCid, ok = actors.GetManifest(actorstypes.Version13)
		if !ok {
			return cid.Undef, xerrors.Errorf("no manifest CID for v13 upgrade")
		}
	}

	// Perform the migration
	newHamtRoot, err := nv22.MigrateStateTree(ctx, adtStore, manifestCid, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v13: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

// ////////////////////
func upgradeActorsV13VerifregFix(oldBuggyVerifregCID, newManifestCID cid.Cid) func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	return func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
		stateStore := sm.ChainStore().StateBlockstore()
		adtStore := store.ActorStore(ctx, stateStore)

		// ensure that the manifest is loaded in the blockstore

		// this loads the "correct" bundle for UpgradeCalibrationDragonFixHeight
		if err := bundle.LoadBundles(ctx, stateStore, actorstypes.Version13); err != nil {
			return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
		}

		// this loads the buggy bundle, for UpgradeDragonHeight
		embedded, ok := build.GetEmbeddedBuiltinActorsBundle(actorstypes.Version13, calibnetv13BuggyBundleSuffix1)
		if !ok {
			return cid.Undef, xerrors.Errorf("didn't find buggy calibrationnet bundle")
		}

		_, err := bundle.LoadBundle(ctx, stateStore, bytes.NewReader(embedded))
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to load buggy calibnet bundle: %w", err)
		}

		// now confirm we have the one we're migrating to
		if haveManifest, err := stateStore.Has(ctx, newManifestCID); err != nil {
			return cid.Undef, xerrors.Errorf("blockstore error when loading manifest %s: %w", newManifestCID, err)
		} else if !haveManifest {
			return cid.Undef, xerrors.Errorf("missing new manifest %s in blockstore", newManifestCID)
		}

		// Load input state tree
		actorsIn, err := state.LoadStateTree(adtStore, root)
		if err != nil {
			return cid.Undef, xerrors.Errorf("loading state tree: %w", err)
		}

		// load old manifest data
		systemActor, err := actorsIn.GetActor(builtin.SystemActorAddr)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
		}

		var systemState system12.State
		if err := adtStore.Get(ctx, systemActor.Head, &systemState); err != nil {
			return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
		}

		var oldManifestData manifest.ManifestData
		if err := adtStore.Get(ctx, systemState.BuiltinActors, &oldManifestData); err != nil {
			return cid.Undef, xerrors.Errorf("failed to get old manifest data: %w", err)
		}

		// load new manifest
		var newManifest manifest.Manifest
		if err := adtStore.Get(ctx, newManifestCID, &newManifest); err != nil {
			return cid.Undef, xerrors.Errorf("error reading actor manifest: %w", err)
		}

		if err := newManifest.Load(ctx, adtStore); err != nil {
			return cid.Undef, xerrors.Errorf("error loading actor manifest: %w", err)
		}

		// build the CID mapping
		codeMapping := make(map[cid.Cid]cid.Cid, len(oldManifestData.Entries))
		for _, oldEntry := range oldManifestData.Entries {
			newCID, ok := newManifest.Get(oldEntry.Name)
			if !ok {
				return cid.Undef, xerrors.Errorf("missing manifest entry for %s", oldEntry.Name)
			}

			// Note: we expect newCID to be the same as oldEntry.Code for all actors except the verifreg actor
			codeMapping[oldEntry.Code] = newCID
		}

		// Create empty actorsOut

		actorsOut, err := state.NewStateTree(adtStore, actorsIn.Version())
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to create new tree: %w", err)
		}

		// Perform the migration
		err = actorsIn.ForEach(func(a address.Address, actor *types.Actor) error {
			newCid, ok := codeMapping[actor.Code]
			if !ok {
				return xerrors.Errorf("didn't find mapping for %s", actor.Code)
			}

			return actorsOut.SetActor(a, &types.ActorV5{
				Code:             newCid,
				Head:             actor.Head,
				Nonce:            actor.Nonce,
				Balance:          actor.Balance,
				DelegatedAddress: actor.DelegatedAddress,
			})
		})
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to perform migration: %w", err)
		}

		systemState.BuiltinActors = newManifest.Data
		newSystemHead, err := adtStore.Put(ctx, &systemState)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to put new system state: %w", err)
		}

		systemActor.Head = newSystemHead
		if err = actorsOut.SetActor(builtin.SystemActorAddr, systemActor); err != nil {
			return cid.Undef, xerrors.Errorf("failed to put new system actor: %w", err)
		}

		// Sanity checking

		err = actorsIn.ForEach(func(a address.Address, inActor *types.Actor) error {
			outActor, err := actorsOut.GetActor(a)
			if err != nil {
				return xerrors.Errorf("failed to get actor in outTree: %w", err)
			}

			if inActor.Nonce != outActor.Nonce {
				return xerrors.Errorf("mismatched nonce for actor %s", a)
			}

			if !inActor.Balance.Equals(outActor.Balance) {
				return xerrors.Errorf("mismatched balance for actor %s: %d != %d", a, inActor.Balance, outActor.Balance)
			}

			if inActor.DelegatedAddress != outActor.DelegatedAddress && inActor.DelegatedAddress.String() != outActor.DelegatedAddress.String() {
				return xerrors.Errorf("mismatched address for actor %s: %s != %s", a, inActor.DelegatedAddress, outActor.DelegatedAddress)
			}

			if inActor.Head != outActor.Head && a != builtin.SystemActorAddr {
				return xerrors.Errorf("mismatched head for actor %s", a)
			}

			// Actor Codes are only expected to change for the verifreg actor
			if inActor.Code != oldBuggyVerifregCID && inActor.Code != outActor.Code {
				return xerrors.Errorf("unexpected change in code for actor %s", a)
			}

			return nil
		})
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to sanity check migration: %w", err)
		}

		// Persist the result.
		newRoot, err := actorsOut.Flush(ctx)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
		}

		return newRoot, nil
	}
}

func PreUpgradeActorsV14(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: time.Minute * 5,
	}

	_, err = upgradeActorsV14Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV14(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}
	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}
	newRoot, err := upgradeActorsV14Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v14 state: %w", err)
	}

	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Print(upgradeSplash)
	}

	return newRoot, nil
}

func upgradeActorsV14Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)
	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version14); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v14 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version14)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v14 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv23.MigrateStateTree(ctx, adtStore, manifest, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v14: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

// PreUpgradeActorsV15 runs the premigration for v15 actors. Note that this migration contains no
// cached migrators, so the only purpose of running a premigration is to prime the blockstore with
// IPLD blocks that would be created during the migration, to reduce the amount of work that needs
// to be done during the actual migration since block Puts become simple Has operations. But the
// same amount of migration work will need to be done otherwise.
func PreUpgradeActorsV15(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	_, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: time.Minute * 5,
	}

	_, err = upgradeActorsV15Common(ctx, sm, cache, lbRoot, epoch, config)
	return err
}

func UpgradeActorsV15(
	ctx context.Context,
	sm *stmgr.StateManager,
	cache stmgr.MigrationCache,
	cb stmgr.ExecMonitor,
	root cid.Cid,
	epoch abi.ChainEpoch,
	ts *types.TipSet,
) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}
	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: 10 * time.Second,
	}
	newRoot, err := upgradeActorsV15Common(ctx, sm, cache, root, epoch, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v15 state: %w", err)
	}
	return newRoot, nil
}

func upgradeActorsV15Common(
	ctx context.Context,
	sm *stmgr.StateManager,
	cache stmgr.MigrationCache,
	root cid.Cid,
	epoch abi.ChainEpoch,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)
	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version15); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v15 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version15)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v15 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv24.MigrateStateTree(
		ctx,
		adtStore,
		manifest,
		stateRoot.Actors,
		epoch,
		// two FIP-0081 constants for this migration only
		int64(buildconstants.UpgradeTuktukHeight),           // powerRampStartEpoch
		buildconstants.UpgradeTuktukPowerRampDurationEpochs, // powerRampDurationEpochs
		config,
		migrationLogger{},
		cache,
	)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v15: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV16(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	logPeriod, err := getMigrationProgressLogPeriod()
	if err != nil {
		return xerrors.Errorf("error getting progress log period: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: logPeriod,
	}

	_, err = upgradeActorsV16Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV16(
	ctx context.Context,
	sm *stmgr.StateManager,
	cache stmgr.MigrationCache,
	cb stmgr.ExecMonitor,
	root cid.Cid,
	epoch abi.ChainEpoch,
	ts *types.TipSet,
) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	logPeriod, err := getMigrationProgressLogPeriod()
	if err != nil {
		return cid.Undef, xerrors.Errorf("error getting progress log period: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: logPeriod,
	}

	newRoot, err := upgradeActorsV16Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v16 state: %w", err)
	}
	return newRoot, nil
}

func upgradeActorsV16Common(
	ctx context.Context,
	sm *stmgr.StateManager,
	cache stmgr.MigrationCache,
	root cid.Cid,
	epoch abi.ChainEpoch,
	ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)

	manifest, ok := actors.GetManifest(actorstypes.Version16)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v16 upgrade")
	}

	if buildconstants.UpgradeTockFixHeight > 0 {
		// If there is a UpgradeTockFixHeight height set, then we are expected to load v16.0.0 here and
		// then UpgradeTockFixHeight will take care of setting the actors to v16.0.1. If it's not set
		// then there's nothing to fix and we'll proceed as normal.

		var initState init12.State
		if actorsIn, err := state.LoadStateTree(adtStore, root); err != nil {
			return cid.Undef, xerrors.Errorf("loading state tree: %w", err)
		} else if initActor, err := actorsIn.GetActor(builtin.InitActorAddr); err != nil {
			return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
		} else if err := adtStore.Get(ctx, initActor.Head, &initState); err != nil {
			return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
		}

		// The v16.0.0 bundle is embedded in the v16 tarball, we just need to load it with the right
		// name (builtin-actors-<network>-v16.0.0.car).
		embedded, ok := build.GetEmbeddedBuiltinActorsBundle(actorstypes.Version16, fmt.Sprintf("%s-%s", initState.NetworkName, v1600BundleSuffix))
		if !ok {
			return cid.Undef, xerrors.Errorf("didn't find v16.0.0 %s bundle with suffix %s", initState.NetworkName, v1600BundleSuffix)
		}

		var err error
		manifest, err = bundle.LoadBundle(ctx, writeStore, bytes.NewReader(embedded))
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to load buggy calibnet bundle: %w", err)
		}

		// Sanity check that we loaded what we were supposed to
		if metadata := build.BuggyBuiltinActorsMetadataForNetwork(initState.NetworkName, actorstypes.Version16); metadata == nil {
			return cid.Undef, xerrors.Errorf("didn't find expected v16.0.0 bundle metadata for %s", initState.NetworkName)
		} else if manifest != metadata.ManifestCid {
			return cid.Undef, xerrors.Errorf("didn't load expected v16.0.0 bundle manifest: %s != %s", manifest, metadata.ManifestCid)
		}
	} else {
		// ensure that the manifest is loaded in the blockstore
		if err := bundle.LoadBundles(ctx, sm.ChainStore().StateBlockstore(), actorstypes.Version16); err != nil {
			return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
		}
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v16 upgrade, got %d",
			stateRoot.Version,
		)
	}

	// Perform the migration
	newHamtRoot, err := nv25.MigrateStateTree(ctx, adtStore, manifest, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v16: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

// UpgradeActorsV16Fix should _not_ be used on mainnet. It performs an upgrade to v16.0.1 on top of
// the existing v16.0.0 that was already deployed.
// The actual mainnet upgrade is performed with UpgradeActorsV16 with the v16.0.1 bundle.
// This upgrade performs an inefficient form of the migration that go-state-types normally performs.
func UpgradeActorsV16Fix(
	ctx context.Context,
	sm *stmgr.StateManager,
	cache stmgr.MigrationCache,
	cb stmgr.ExecMonitor,
	root cid.Cid,
	epoch abi.ChainEpoch,
	ts *types.TipSet,
) (cid.Cid, error) {
	stateStore := sm.ChainStore().StateBlockstore()
	adtStore := store.ActorStore(ctx, stateStore)

	// Get the real v16 manifest, which should be for v16.0.1
	manifestCid, ok := actors.GetManifest(actorstypes.Version16)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v16 upgrade")
	}
	// ensure that the manifest is loaded in the blockstore, it will load the correct v16.0.1 bundle
	if err := bundle.LoadBundles(ctx, stateStore, actorstypes.Version16); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load input state tree
	actorsIn, err := state.LoadStateTree(adtStore, root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("loading state tree: %w", err)
	}

	// load old manifest data
	oldSystemActor, err := actorsIn.GetActor(builtin.SystemActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
	}
	// load old manifest data directly from the system actor
	var oldManifestData manifest.ManifestData
	var oldSystemState system12.State
	if err := adtStore.Get(ctx, oldSystemActor.Head, &oldSystemState); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
	} else if err := adtStore.Get(ctx, oldSystemState.BuiltinActors, &oldManifestData); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get old manifest data: %w", err)
	}

	// load new manifest from the blockstore
	var newManifest manifest.Manifest
	if err := adtStore.Get(ctx, manifestCid, &newManifest); err != nil {
		return cid.Undef, xerrors.Errorf("error reading actor manifest: %w", err)
	} else if err := newManifest.Load(ctx, adtStore); err != nil {
		return cid.Undef, xerrors.Errorf("error loading actor manifest: %w", err)
	}

	// build an actor CID mapping
	codeMapping := make(map[cid.Cid]cid.Cid, len(oldManifestData.Entries))
	for _, oldEntry := range oldManifestData.Entries {
		newCID, ok := newManifest.Get(oldEntry.Name)
		if !ok {
			return cid.Undef, xerrors.Errorf("missing manifest entry for %s", oldEntry.Name)
		}
		codeMapping[oldEntry.Code] = newCID
	}

	// Create empty state tree
	actorsOut, err := state.NewStateTree(adtStore, actorsIn.Version())
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create new tree: %w", err)
	}

	// Perform the migration
	err = actorsIn.ForEach(func(a address.Address, actor *types.Actor) error {
		newCid, ok := codeMapping[actor.Code]
		if !ok {
			return xerrors.Errorf("didn't find mapping for %s", actor.Code)
		}

		return actorsOut.SetActor(a, &types.ActorV5{
			Code:             newCid,
			Head:             actor.Head,
			Nonce:            actor.Nonce,
			Balance:          actor.Balance,
			DelegatedAddress: actor.DelegatedAddress,
		})
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to perform migration: %w", err)
	}

	// Setup the system actor with the new manifest, fetching it from actorsOut where it's already
	// had its code CID changed, changing the manifest, then writing back to actorsOut
	newSystemActor, err := actorsOut.GetActor(builtin.SystemActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
	}
	var newSystemState system12.State
	if err := adtStore.Get(ctx, newSystemActor.Head, &newSystemState); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
	} else if err := adtStore.Get(ctx, newSystemState.BuiltinActors, &oldManifestData); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get old manifest data: %w", err)
	}
	newSystemState.BuiltinActors = newManifest.Data
	newSystemHead, err := adtStore.Put(ctx, &newSystemState)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to put new system state: %w", err)
	}
	newSystemActor.Head = newSystemHead
	if err = actorsOut.SetActor(builtin.SystemActorAddr, newSystemActor); err != nil {
		return cid.Undef, xerrors.Errorf("failed to put new system actor: %w", err)
	}

	// Sanity check that the migration worked by re-iterating over the old tree and checking
	// against the new tree's actors.

	// initState for networkName
	var initState init12.State
	if actorsIn, err := state.LoadStateTree(adtStore, root); err != nil {
		return cid.Undef, xerrors.Errorf("loading state tree: %w", err)
	} else if initActor, err := actorsIn.GetActor(builtin.InitActorAddr); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor: %w", err)
	} else if err := adtStore.Get(ctx, initActor.Head, &initState); err != nil {
		return cid.Undef, xerrors.Errorf("failed to get system actor state: %w", err)
	}

	v1600metadata := build.BuggyBuiltinActorsMetadataForNetwork(initState.NetworkName, actorstypes.Version16)
	if v1600metadata == nil {
		return cid.Undef, xerrors.Errorf("expected v16.0.0 metadata for network %s", initState.NetworkName)
	}

	err = actorsIn.ForEach(func(a address.Address, inActor *types.Actor) error {
		outActor, err := actorsOut.GetActor(a)
		if err != nil {
			return xerrors.Errorf("failed to get actor in outTree: %w", err)
		}

		if inActor.Nonce != outActor.Nonce {
			return xerrors.Errorf("mismatched nonce for actor %s", a)
		} else if !inActor.Balance.Equals(outActor.Balance) {
			return xerrors.Errorf("mismatched balance for actor %s: %d != %d", a, inActor.Balance, outActor.Balance)
		} else if inActor.DelegatedAddress != outActor.DelegatedAddress && inActor.DelegatedAddress.String() != outActor.DelegatedAddress.String() {
			return xerrors.Errorf("mismatched address for actor %s: %s != %s", a, inActor.DelegatedAddress, outActor.DelegatedAddress)
		} else if inActor.Head != outActor.Head && a != builtin.SystemActorAddr {
			return xerrors.Errorf("mismatched head for actor %s", a)
		}

		/*
			TODO: This code block was skipped while preparing the nv27 network skeleton:
				https://github.com/filecoin-project/lotus/pull/13125
			The problem encountered here is that the initial v17 actors bundle for the nv27 skeleton was
			identical to the v16 bundle (a normal part of skeleton setup), so calls to
			actors.GetActorMetaByCode for v16 CIDs would return 17 as the version, and the second
			assertion below here fails. This ought to be solved when a new actors bundle is introduced and
			this block can be re-enabled. However, this is also not critical code, and was only used on
			calibnet, so 🤷.

			// Check that the actor code has changed to the new expected value; work backward by getting the
			// actor name from the new code CID.
			if actorName, version, ok := actors.GetActorMetaByCode(outActor.Code); !ok {
				return xerrors.Errorf("failed to get actor meta for code %s", outActor.Code)
			} else if version != actorstypes.Version16 {
				return xerrors.Errorf("unexpected actor version for %s: %d", actorName, version)
			} else if oldCode, ok := v1600metadata.Actors[actorName]; !ok {
				return xerrors.Errorf("missing actor %s in v16.0.0 metadata", actorName)
			} else if oldCode != inActor.Code {
				return xerrors.Errorf("unexpected actor code for %s: %s != %s", actorName, oldCode, outActor.Code)
			}
		*/

		return nil
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to sanity check migration: %w", err)
	}

	// Persist the result.
	newRoot, err := actorsOut.Flush(ctx)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV17(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	logPeriod, err := getMigrationProgressLogPeriod()
	if err != nil {
		return xerrors.Errorf("error getting progress log period: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: logPeriod,
	}

	_, err = upgradeActorsV17Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV17(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	logPeriod, err := getMigrationProgressLogPeriod()
	if err != nil {
		return cid.Undef, xerrors.Errorf("error getting progress log period: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: logPeriod,
	}
	newRoot, err := upgradeActorsV17Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v17 state: %w", err)
	}
	return newRoot, nil
}

func upgradeActorsV17Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)
	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version17); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v17 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version17)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v17 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv27.MigrateStateTree(ctx, adtStore, manifest, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v17: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

func PreUpgradeActorsV18(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
	// Use half the CPUs for pre-migration, but leave at least 3.
	workerCount := MigrationMaxWorkerCount
	if workerCount <= 4 {
		workerCount = 1
	} else {
		workerCount /= 2
	}

	lbts, lbRoot, err := stmgr.GetLookbackTipSetForRound(ctx, sm, ts, epoch)
	if err != nil {
		return xerrors.Errorf("error getting lookback ts for premigration: %w", err)
	}

	logPeriod, err := getMigrationProgressLogPeriod()
	if err != nil {
		return xerrors.Errorf("error getting progress log period: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		ProgressLogPeriod: logPeriod,
	}

	_, err = upgradeActorsV18Common(ctx, sm, cache, lbRoot, epoch, lbts, config)
	return err
}

func UpgradeActorsV18(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	// Use all the CPUs except 2.
	workerCount := MigrationMaxWorkerCount - 3
	if workerCount <= 0 {
		workerCount = 1
	}

	logPeriod, err := getMigrationProgressLogPeriod()
	if err != nil {
		return cid.Undef, xerrors.Errorf("error getting progress log period: %w", err)
	}

	config := migration.Config{
		MaxWorkers:        uint(workerCount),
		JobQueueSize:      1000,
		ResultQueueSize:   100,
		ProgressLogPeriod: logPeriod,
	}
	newRoot, err := upgradeActorsV18Common(ctx, sm, cache, root, epoch, ts, config)
	if err != nil {
		return cid.Undef, xerrors.Errorf("migrating actors v18 state: %w", err)
	}
	return newRoot, nil
}

func upgradeActorsV18Common(
	ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
	root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet,
	config migration.Config,
) (cid.Cid, error) {
	writeStore := blockstore.NewAutobatch(ctx, sm.ChainStore().StateBlockstore(), units.GiB/4)
	adtStore := store.ActorStore(ctx, writeStore)
	// ensure that the manifest is loaded in the blockstore
	if err := bundle.LoadBundles(ctx, writeStore, actorstypes.Version18); err != nil {
		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
	}

	// Load the state root.
	var stateRoot types.StateRoot
	if err := adtStore.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != types.StateTreeVersion5 {
		return cid.Undef, xerrors.Errorf(
			"expected state root version 5 for actors v18 upgrade, got %d",
			stateRoot.Version,
		)
	}

	manifest, ok := actors.GetManifest(actorstypes.Version18)
	if !ok {
		return cid.Undef, xerrors.Errorf("no manifest CID for v18 upgrade")
	}

	// Perform the migration
	newHamtRoot, err := nv28.MigrateStateTree(ctx, adtStore, manifest, stateRoot.Actors, epoch, config,
		migrationLogger{}, cache)
	if err != nil {
		return cid.Undef, xerrors.Errorf("upgrading to actors v18: %w", err)
	}

	// Persist the result.
	newRoot, err := adtStore.Put(ctx, &types.StateRoot{
		Version: types.StateTreeVersion5,
		Actors:  newHamtRoot,
		Info:    stateRoot.Info,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to persist new state root: %w", err)
	}

	// Persists the new tree and shuts down the flush worker
	if err := writeStore.Flush(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore flush failed: %w", err)
	}

	if err := writeStore.Shutdown(ctx); err != nil {
		return cid.Undef, xerrors.Errorf("writeStore shutdown failed: %w", err)
	}

	return newRoot, nil
}

////////////////////

// Example upgrade function if upgrade requires only code changes
//func UpgradeActorsV9(ctx context.Context, sm *stmgr.StateManager, _ stmgr.MigrationCache, _ stmgr.ExecMonitor, root cid.Cid, _ abi.ChainEpoch, _ *types.TipSet) (cid.Cid, error) {
//	buf := blockstore.NewTieredBstore(sm.ChainStore().StateBlockstore(), blockstore.NewMemorySync())
//
//	av := actors.Version9
//	// This may change for upgrade
//	newStateTreeVersion := types.StateTreeVersion4
//
//	// ensure that the manifest is loaded in the blockstore
//	if err := bundle.FetchAndLoadBundles(ctx, buf, map[actors.Version]build.Bundle{
//		av: build.BuiltinActorReleases[av],
//	}); err != nil {
//		return cid.Undef, xerrors.Errorf("failed to load manifest bundle: %w", err)
//	}
//
//	newActorsManifestCid, ok := actors.GetManifest(av)
//	if !ok {
//		return cid.Undef, xerrors.Errorf("no manifest CID for v8 upgrade")
//	}
//
//	bstore := sm.ChainStore().StateBlockstore()
//	return LiteMigration(ctx, bstore, newActorsManifestCid, root, av, types.StateTreeVersion4, newStateTreeVersion)
//}

func LiteMigration(ctx context.Context, bstore blockstore.Blockstore, newActorsManifestCid cid.Cid, root cid.Cid, oldAv actorstypes.Version, newAv actorstypes.Version, oldStateTreeVersion types.StateTreeVersion, newStateTreeVersion types.StateTreeVersion) (cid.Cid, error) {
	buf := blockstore.NewTieredBstore(bstore, blockstore.NewMemorySync())
	store := store.ActorStore(ctx, buf)
	adtStore := gstStore.WrapStore(ctx, store)

	// Load the state root.
	var stateRoot types.StateRoot
	if err := store.Get(ctx, root, &stateRoot); err != nil {
		return cid.Undef, xerrors.Errorf("failed to decode state root: %w", err)
	}

	if stateRoot.Version != oldStateTreeVersion {
		return cid.Undef, xerrors.Errorf(
			"expected state tree version %d for actors code upgrade, got %d",
			oldStateTreeVersion,
			stateRoot.Version,
		)
	}

	st, err := state.LoadStateTree(store, root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to load state tree: %w", err)
	}

	oldManifestData, err := stmgr.GetManifestData(ctx, st)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error loading old actor manifest: %w", err)
	}

	// load new manifest
	newManifest, err := actors.LoadManifest(ctx, newActorsManifestCid, store)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error loading new manifest: %w", err)
	}

	var newManifestData manifest.ManifestData
	if err := store.Get(ctx, newManifest.Data, &newManifestData); err != nil {
		return cid.Undef, xerrors.Errorf("error loading new manifest data: %w", err)
	}

	if len(oldManifestData.Entries) != len(manifest.GetBuiltinActorsKeys(oldAv)) {
		return cid.Undef, xerrors.Errorf("incomplete old manifest with %d code CIDs", len(oldManifestData.Entries))
	}
	if len(newManifestData.Entries) != len(manifest.GetBuiltinActorsKeys(newAv)) {
		return cid.Undef, xerrors.Errorf("incomplete new manifest with %d code CIDs", len(newManifestData.Entries))
	}

	// Maps prior version code CIDs to migration functions.
	migrations := make(map[cid.Cid]cid.Cid)

	for _, entry := range oldManifestData.Entries {
		newCodeCid, ok := newManifest.Get(entry.Name)
		if !ok {
			return cid.Undef, xerrors.Errorf("code cid for %s actor not found in new manifest", entry.Name)
		}

		migrations[entry.Code] = newCodeCid
	}

	startTime := time.Now()

	// Load output state tree
	actorsOut, err := state.NewStateTree(adtStore, newStateTreeVersion)
	if err != nil {
		return cid.Undef, err
	}

	// Insert migrated records in output state tree.
	err = st.ForEach(func(addr address.Address, actorIn *types.Actor) error {
		newCid, ok := migrations[actorIn.Code]
		if !ok {
			return xerrors.Errorf("new code cid not found in migrations for actor %s", addr)
		}
		var head cid.Cid
		if addr == system.Address {
			newSystemState, err := system.MakeState(store, newAv, newManifest.Data)
			if err != nil {
				return xerrors.Errorf("could not make system actor state: %w", err)
			}
			head, err = store.Put(ctx, newSystemState)
			if err != nil {
				return xerrors.Errorf("could not set system actor state head: %w", err)
			}
		} else {
			head = actorIn.Head
		}
		newActor := types.Actor{
			Code:    newCid,
			Head:    head,
			Nonce:   actorIn.Nonce,
			Balance: actorIn.Balance,
		}
		err = actorsOut.SetActor(addr, &newActor)
		if err != nil {
			return xerrors.Errorf("could not set actor at address %s: %w", addr, err)
		}

		return nil
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed update actor states: %w", err)
	}

	elapsed := time.Since(startTime)
	log.Infof("All done after %v. Flushing state tree root.", elapsed)
	newRoot, err := actorsOut.Flush(ctx)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to flush new actors: %w", err)
	}

	// Persist the new tree.
	{
		from := buf
		to := buf.Read()

		if err := vm.Copy(ctx, from, to, newRoot); err != nil {
			return cid.Undef, xerrors.Errorf("copying migrated tree: %w", err)
		}
	}

	return newRoot, nil
}

type migrationLogger struct{}

func (ml migrationLogger) Log(level rt.LogLevel, msg string, args ...interface{}) {
	switch level {
	case rt.DEBUG:
		log.Debugf(msg, args...)
	case rt.INFO:
		log.Infof(msg, args...)
	case rt.WARN:
		log.Warnf(msg, args...)
	case rt.ERROR:
		log.Errorf(msg, args...)
	}
}

func getMigrationProgressLogPeriod() (time.Duration, error) {
	logPeriod := time.Second * 2 // default period
	period := os.Getenv("LOTUS_MIGRATE_PROGRESS_LOG_SECONDS")
	if period != "" {
		seconds, err := strconv.Atoi(period)
		if err != nil {
			return 0, xerrors.Errorf("LOTUS_MIGRATE_PROGRESS_LOG_SECONDS must be an integer: %w", err)
		}
		if seconds <= 0 {
			return 0, xerrors.Errorf("LOTUS_MIGRATE_PROGRESS_LOG_SECONDS must be positive")
		}
		logPeriod = time.Duration(seconds) * time.Second
	}
	return logPeriod, nil
}
