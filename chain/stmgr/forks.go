package stmgr

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"

	"github.com/filecoin-project/lotus/chain/actors/builtin"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	multisig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	power0 "github.com/filecoin-project/specs-actors/actors/builtin/power"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/specs-actors/actors/migration/nv3"
	m2 "github.com/filecoin-project/specs-actors/v2/actors/migration"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/bufbstore"
)

// UpgradeFunc is a migration function run at every upgrade.
//
// - The oldState is the state produced by the upgrade epoch.
// - The returned newState is the new state that will be used by the next epoch.
// - The height is the upgrade epoch height (already executed).
// - The tipset is the tipset for the last non-null block before the upgrade. Do
//   not assume that ts.Height() is the upgrade height.
type UpgradeFunc func(ctx context.Context, sm *StateManager, cb ExecCallback, oldState cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (newState cid.Cid, err error)

type Upgrade struct {
	Height    abi.ChainEpoch
	Network   network.Version
	Expensive bool
	Migration UpgradeFunc
}

type UpgradeSchedule []Upgrade

func DefaultUpgradeSchedule() UpgradeSchedule {
	var us UpgradeSchedule

	updates := []Upgrade{{
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
		Height:    build.UpgradeActorsV2Height,
		Network:   network.Version4,
		Expensive: true,
		Migration: UpgradeActorsV2,
	}, {
		Height:  build.UpgradeTapeHeight,
		Network: network.Version5,
	}, {
		Height:    build.UpgradeLiftoffHeight,
		Network:   network.Version5,
		Migration: UpgradeLiftoff,
	}}

	if build.UpgradeActorsV2Height == math.MaxInt64 { // disable actors upgrade
		updates = []Upgrade{{
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
			Height:    build.UpgradeLiftoffHeight,
			Network:   network.Version3,
			Migration: UpgradeLiftoff,
		}}
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

func (us UpgradeSchedule) Validate() error {
	// Make sure we're not trying to upgrade to version 0.
	for _, u := range us {
		if u.Network <= 0 {
			return xerrors.Errorf("cannot upgrade to version <= 0: %d", u.Network)
		}
	}

	// Make sure all the upgrades make sense.
	for i := 1; i < len(us); i++ {
		prev := &us[i-1]
		curr := &us[i]
		if !(prev.Network <= curr.Network) {
			return xerrors.Errorf("cannot downgrade from version %d to version %d", prev.Network, curr.Network)
		}
		// Make sure the heights make sense.
		if prev.Height < 0 {
			// Previous upgrade was disabled.
			continue
		}
		if !(prev.Height < curr.Height) {
			return xerrors.Errorf("upgrade heights must be strictly increasing: upgrade %d was at height %d, followed by upgrade %d at height %d", i-1, prev.Height, i, curr.Height)
		}
	}
	return nil
}

func (sm *StateManager) handleStateForks(ctx context.Context, root cid.Cid, height abi.ChainEpoch, cb ExecCallback, ts *types.TipSet) (cid.Cid, error) {
	retCid := root
	var err error
	f, ok := sm.stateMigrations[height]
	if ok {
		retCid, err = f(ctx, sm, cb, root, height, ts)
		if err != nil {
			return cid.Undef, err
		}
	}

	return retCid, nil
}

func (sm *StateManager) hasExpensiveFork(ctx context.Context, height abi.ChainEpoch) bool {
	_, ok := sm.expensiveUpgrades[height]
	return ok
}

func doTransfer(tree types.StateTree, from, to address.Address, amt abi.TokenAmount, cb func(trace types.ExecutionTrace)) error {
	fromAct, err := tree.GetActor(from)
	if err != nil {
		return xerrors.Errorf("failed to get 'from' actor for transfer: %w", err)
	}

	fromAct.Balance = types.BigSub(fromAct.Balance, amt)
	if fromAct.Balance.Sign() < 0 {
		return xerrors.Errorf("(sanity) deducted more funds from target account than it had (%s, %s)", from, types.FIL(amt))
	}

	if err := tree.SetActor(from, fromAct); err != nil {
		return xerrors.Errorf("failed to persist from actor: %w", err)
	}

	toAct, err := tree.GetActor(to)
	if err != nil {
		return xerrors.Errorf("failed to get 'to' actor for transfer: %w", err)
	}

	toAct.Balance = types.BigAdd(toAct.Balance, amt)

	if err := tree.SetActor(to, toAct); err != nil {
		return xerrors.Errorf("failed to persist to actor: %w", err)
	}

	if cb != nil {
		// record the transfer in execution traces

		fakeMsg := &types.Message{
			From:  from,
			To:    to,
			Value: amt,
		}
		fakeRct := &types.MessageReceipt{
			ExitCode: 0,
			Return:   nil,
			GasUsed:  0,
		}

		cb(types.ExecutionTrace{
			Msg:        fakeMsg,
			MsgRct:     fakeRct,
			Error:      "",
			Duration:   0,
			GasCharges: nil,
			Subcalls:   nil,
		})
	}

	return nil
}

func UpgradeFaucetBurnRecovery(ctx context.Context, sm *StateManager, cb ExecCallback, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
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
			if err := sm.ChainStore().Store(ctx).Get(ctx, act.Head, &st); err != nil {
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
		if err := doTransfer(tree, t.From, t.To, t.Amt, transferCb); err != nil {
			return cid.Undef, xerrors.Errorf("transfer %s %s->%s failed: %w", t.Amt, t.From, t.To, err)
		}
	}

	// pull up power table to give miners back some funds proportional to their power
	var ps power0.State
	powAct, err := tree.GetActor(builtin0.StoragePowerActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to load power actor: %w", err)
	}

	cst := cbor.NewCborStore(sm.ChainStore().Blockstore())
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
			if err := sm.ChainStore().Store(ctx).Get(ctx, act.Head, &st); err != nil {
				return xerrors.Errorf("failed to load miner state: %w", err)
			}

			var minfo miner0.MinerInfo
			if err := cst.Get(ctx, st.Info, &minfo); err != nil {
				return xerrors.Errorf("failed to get miner info: %w", err)
			}

			sectorsArr, err := adt0.AsArray(sm.ChainStore().Store(ctx), st.Sectors)
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
				if err := sm.ChainStore().Store(ctx).Get(ctx, lbact.Head, &lbst); err != nil {
					return xerrors.Errorf("failed to load miner state: %w", err)
				}

				lbsectors, err := adt0.AsArray(sm.ChainStore().Store(ctx), lbst.Sectors)
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
		if err := doTransfer(tree, t.From, t.To, t.Amt, transferCb); err != nil {
			return cid.Undef, xerrors.Errorf("transfer %s %s->%s failed: %w", t.Amt, t.From, t.To, err)
		}
	}

	// transfer all burnt funds back to the reserve account
	burntAct, err := tree.GetActor(builtin0.BurntFundsActorAddr)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to load burnt funds actor: %w", err)
	}
	if err := doTransfer(tree, builtin0.BurntFundsActorAddr, builtin.ReserveAddress, burntAct.Balance, transferCb); err != nil {
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
	if err := doTransfer(tree, builtin.ReserveAddress, reimbAddr, difference, transferCb); err != nil {
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

	if cb != nil {
		// record the transfer in execution traces

		fakeMsg := &types.Message{
			From:  builtin.SystemActorAddr,
			To:    builtin.SystemActorAddr,
			Value: big.Zero(),
			Nonce: uint64(epoch),
		}
		fakeRct := &types.MessageReceipt{
			ExitCode: 0,
			Return:   nil,
			GasUsed:  0,
		}

		if err := cb(fakeMsg.Cid(), fakeMsg, &vm.ApplyRet{
			MessageReceipt: *fakeRct,
			ActorErr:       nil,
			ExecutionTrace: types.ExecutionTrace{
				Msg:        fakeMsg,
				MsgRct:     fakeRct,
				Error:      "",
				Duration:   0,
				GasCharges: nil,
				Subcalls:   subcalls,
			},
			Duration: 0,
			GasCosts: vm.ZeroGasOutputs(),
		}); err != nil {
			return cid.Undef, xerrors.Errorf("recording transfers: %w", err)
		}
	}

	return tree.Flush(ctx)
}

func UpgradeIgnition(ctx context.Context, sm *StateManager, cb ExecCallback, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	store := sm.cs.Store(ctx)

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

	err = setNetworkName(ctx, store, tree, "ignition")
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

	err = resetGenesisMsigs(ctx, sm, store, tree, build.UpgradeLiftoffHeight)
	if err != nil {
		return cid.Undef, xerrors.Errorf("resetting genesis msig start epochs: %w", err)
	}

	err = splitGenesisMultisig(ctx, cb, split1, store, tree, 50, epoch)
	if err != nil {
		return cid.Undef, xerrors.Errorf("splitting first msig: %w", err)
	}

	err = splitGenesisMultisig(ctx, cb, split2, store, tree, 50, epoch)
	if err != nil {
		return cid.Undef, xerrors.Errorf("splitting second msig: %w", err)
	}

	err = nv3.CheckStateTree(ctx, store, nst, epoch, builtin0.TotalFilecoin)
	if err != nil {
		return cid.Undef, xerrors.Errorf("sanity check after ignition upgrade failed: %w", err)
	}

	return tree.Flush(ctx)
}

func UpgradeRefuel(ctx context.Context, sm *StateManager, cb ExecCallback, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {

	store := sm.cs.Store(ctx)
	tree, err := sm.StateTree(root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
	}

	err = resetMultisigVesting(ctx, store, tree, builtin.SaftAddress, 0, 0, big.Zero())
	if err != nil {
		return cid.Undef, xerrors.Errorf("tweaking msig vesting: %w", err)
	}

	err = resetMultisigVesting(ctx, store, tree, builtin.ReserveAddress, 0, 0, big.Zero())
	if err != nil {
		return cid.Undef, xerrors.Errorf("tweaking msig vesting: %w", err)
	}

	err = resetMultisigVesting(ctx, store, tree, builtin.RootVerifierAddress, 0, 0, big.Zero())
	if err != nil {
		return cid.Undef, xerrors.Errorf("tweaking msig vesting: %w", err)
	}

	return tree.Flush(ctx)
}

func UpgradeActorsV2(ctx context.Context, sm *StateManager, cb ExecCallback, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	buf := bufbstore.NewTieredBstore(sm.cs.Blockstore(), bstore.NewTemporarySync())
	store := store.ActorStore(ctx, buf)

	info, err := store.Put(ctx, new(types.StateInfo0))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create new state info for actors v2: %w", err)
	}

	newHamtRoot, err := m2.MigrateStateTree(ctx, store, root, epoch, m2.DefaultConfig())
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

func UpgradeLiftoff(ctx context.Context, sm *StateManager, cb ExecCallback, root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
	tree, err := sm.StateTree(root)
	if err != nil {
		return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
	}

	err = setNetworkName(ctx, sm.cs.Store(ctx), tree, "mainnet")
	if err != nil {
		return cid.Undef, xerrors.Errorf("setting network name: %w", err)
	}

	return tree.Flush(ctx)
}

func setNetworkName(ctx context.Context, store adt.Store, tree *state.StateTree, name string) error {
	ia, err := tree.GetActor(builtin0.InitActorAddr)
	if err != nil {
		return xerrors.Errorf("getting init actor: %w", err)
	}

	initState, err := init_.Load(store, ia)
	if err != nil {
		return xerrors.Errorf("reading init state: %w", err)
	}

	if err := initState.SetNetworkName(name); err != nil {
		return xerrors.Errorf("setting network name: %w", err)
	}

	ia.Head, err = store.Put(ctx, initState)
	if err != nil {
		return xerrors.Errorf("writing new init state: %w", err)
	}

	if err := tree.SetActor(builtin0.InitActorAddr, ia); err != nil {
		return xerrors.Errorf("setting init actor: %w", err)
	}

	return nil
}

func splitGenesisMultisig(ctx context.Context, cb ExecCallback, addr address.Address, store adt0.Store, tree *state.StateTree, portions uint64, epoch abi.ChainEpoch) error {
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
		keyAddr, err := makeKeyAddr(addr, i)
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

		if err := doTransfer(tree, addr, idAddr, newIbal, transferCb); err != nil {
			return xerrors.Errorf("transferring split msig balance: %w", err)
		}

		i++
	}

	if cb != nil {
		// record the transfer in execution traces

		fakeMsg := &types.Message{
			From:  builtin.SystemActorAddr,
			To:    addr,
			Value: big.Zero(),
			Nonce: uint64(epoch),
		}
		fakeRct := &types.MessageReceipt{
			ExitCode: 0,
			Return:   nil,
			GasUsed:  0,
		}

		if err := cb(fakeMsg.Cid(), fakeMsg, &vm.ApplyRet{
			MessageReceipt: *fakeRct,
			ActorErr:       nil,
			ExecutionTrace: types.ExecutionTrace{
				Msg:        fakeMsg,
				MsgRct:     fakeRct,
				Error:      "",
				Duration:   0,
				GasCharges: nil,
				Subcalls:   subcalls,
			},
			Duration: 0,
			GasCosts: vm.ZeroGasOutputs(),
		}); err != nil {
			return xerrors.Errorf("recording transfers: %w", err)
		}
	}

	return nil
}

func makeKeyAddr(splitAddr address.Address, count uint64) (address.Address, error) {
	var b bytes.Buffer
	if err := splitAddr.MarshalCBOR(&b); err != nil {
		return address.Undef, xerrors.Errorf("marshalling split address: %w", err)
	}

	if err := binary.Write(&b, binary.BigEndian, count); err != nil {
		return address.Undef, xerrors.Errorf("writing count into a buffer: %w", err)
	}

	if err := binary.Write(&b, binary.BigEndian, []byte("Ignition upgrade")); err != nil {
		return address.Undef, xerrors.Errorf("writing fork name into a buffer: %w", err)
	}

	addr, err := address.NewActorAddress(b.Bytes())
	if err != nil {
		return address.Undef, xerrors.Errorf("create actor address: %w", err)
	}

	return addr, nil
}

// TODO: After the Liftoff epoch, refactor this to use resetMultisigVesting
func resetGenesisMsigs(ctx context.Context, sm *StateManager, store adt0.Store, tree *state.StateTree, startEpoch abi.ChainEpoch) error {
	gb, err := sm.cs.GetGenesis()
	if err != nil {
		return xerrors.Errorf("getting genesis block: %w", err)
	}

	gts, err := types.NewTipSet([]*types.BlockHeader{gb})
	if err != nil {
		return xerrors.Errorf("getting genesis tipset: %w", err)
	}

	cst := cbor.NewCborStore(sm.cs.Blockstore())
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

func resetMultisigVesting(ctx context.Context, store adt0.Store, tree *state.StateTree, addr address.Address, startEpoch abi.ChainEpoch, duration abi.ChainEpoch, balance abi.TokenAmount) error {
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
