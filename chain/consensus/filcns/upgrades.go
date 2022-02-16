package filcns

import (
	"context"
	"runtime"
	"time"

	"github.com/docker/go-units"

	"github.com/filecoin-project/specs-actors/v6/actors/migration/nv14"
	"github.com/filecoin-project/specs-actors/v7/actors/migration/nv15"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-state-types/rt"

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

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func DefaultUpgradeSchedule() stmgr.UpgradeSchedule {
	var us stmgr.UpgradeSchedule

	updates := []stmgr.Upgrade{{
		Height:    build.UpgradeBreezeHeight,
		Network:   network.Version1,
		Migration: UpgradeFaucetBurnRecovery,
	}, {
		Height:    build.UpgradeSmokeHeight,
		Network:   network.Version2,
		Migration: nil,
	}, {
		Height:    build.UpgradeIgnitionHeight,
		Network:   network.Version3,
		Migration: UpgradeIgnition,
	}, {
		Height:    build.UpgradeRefuelHeight,
		Network:   network.Version3,
		Migration: UpgradeRefuel,
	}, {
		Height:    build.UpgradeAssemblyHeight,
		Network:   network.Version4,
		Expensive: true,
		Migration: UpgradeActorsV2,
	}, {
		Height:    build.UpgradeTapeHeight,
		Network:   network.Version5,
		Migration: nil,
	}, {
		Height:    build.UpgradeLiftoffHeight,
		Network:   network.Version5,
		Migration: UpgradeLiftoff,
	}, {
		Height:    build.UpgradeKumquatHeight,
		Network:   network.Version6,
		Migration: nil,
	}, {
		Height:    build.UpgradeCalicoHeight,
		Network:   network.Version7,
		Migration: UpgradeCalico,
	}, {
		Height:    build.UpgradePersianHeight,
		Network:   network.Version8,
		Migration: nil,
	}, {
		Height:    build.UpgradeOrangeHeight,
		Network:   network.Version9,
		Migration: nil,
	}, {
		Height:    build.UpgradeTrustHeight,
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
		Height:    build.UpgradeNorwegianHeight,
		Network:   network.Version11,
		Migration: nil,
	}, {
		Height:    build.UpgradeTurboHeight,
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
		Height:    build.UpgradeHyperdriveHeight,
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
		Height:    build.UpgradeChocolateHeight,
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
		Height:    build.UpgradeOhSnapHeight,
		Network:   network.Version15,
		Migration: UpgradeActorsV7,
		PreMigrations: []stmgr.PreMigration{{
			PreMigration:    PreUpgradeActorsV7,
			StartWithin:     180,
			DontStartWithin: 60,
			StopWithin:      5,
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
				// this panics if the miner doesnt have enough funds to cover their locked pledge
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
			if !xerrors.Is(err, types.ErrActorNotFound) {
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

	exp := types.FromFil(build.FilBase)
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
				Msg:        fakeMsg,
				MsgRct:     stmgr.MakeFakeRct(),
				Error:      "",
				Duration:   0,
				GasCharges: nil,
				Subcalls:   subcalls,
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

	if build.UpgradeLiftoffHeight <= epoch {
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

	err = resetGenesisMsigs0(ctx, sm, store, tree, build.UpgradeLiftoffHeight)
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
				Msg:        fakeMsg,
				MsgRct:     stmgr.MakeFakeRct(),
				Error:      "",
				Duration:   0,
				GasCharges: nil,
				Subcalls:   subcalls,
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
	gb, err := sm.ChainStore().GetGenesis()
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
	if build.BuildType != build.BuildMainnet {
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
	workerCount := runtime.NumCPU() - 3
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

	if build.BuildType == build.BuildMainnet {
		err := stmgr.TerminateActor(ctx, tree, build.ZeroAddress, cb, epoch, ts)
		if err != nil && !xerrors.Is(err, types.ErrActorNotFound) {
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
	workerCount := runtime.NumCPU()
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
	workerCount := runtime.NumCPU() - 3
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
	workerCount := runtime.NumCPU()
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
	workerCount := runtime.NumCPU() - 3
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
	workerCount := runtime.NumCPU()
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
	workerCount := runtime.NumCPU() - 3
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
	workerCount := runtime.NumCPU()
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
	workerCount := runtime.NumCPU() - 3
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
	workerCount := runtime.NumCPU()
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
