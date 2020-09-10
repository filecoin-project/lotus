package stmgr

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"
)

var ForksAtHeight = map[abi.ChainEpoch]func(context.Context, *StateManager, types.StateTree, *types.TipSet) error{
	build.UpgradeBreezeHeight: UpgradeFaucetBurnRecovery,
}

func (sm *StateManager) handleStateForks(ctx context.Context, st types.StateTree, height abi.ChainEpoch, ts *types.TipSet) (err error) {
	f, ok := ForksAtHeight[height]
	if ok {
		err := f(ctx, sm, st, ts)
		if err != nil {
			return err
		}
	}

	return nil
}

type forEachTree interface {
	ForEach(func(address.Address, *types.Actor) error) error
}

func doTransfer(tree types.StateTree, from, to address.Address, amt abi.TokenAmount) error {
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

	return nil
}

func UpgradeFaucetBurnRecovery(ctx context.Context, sm *StateManager, tree types.StateTree, ts *types.TipSet) error {
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
		return xerrors.Errorf("failed to get tipset at lookback height: %w", err)
	}

	var lbtree *state.StateTree
	if err = sm.WithStateTree(lbts.ParentState(), func(state *state.StateTree) error {
		lbtree = state
		return nil
	}); err != nil {
		return xerrors.Errorf("loading state tree failed: %w", err)
	}

	ReserveAddress, err := address.NewFromString("t090")
	if err != nil {
		return xerrors.Errorf("failed to parse reserve address: %w", err)
	}

	fetree, ok := tree.(forEachTree)
	if !ok {
		return xerrors.Errorf("fork transition state tree doesnt support ForEach (%T)", tree)
	}

	type transfer struct {
		From address.Address
		To   address.Address
		Amt  abi.TokenAmount
	}

	var transfers []transfer

	// Take all excess funds away, put them into the reserve account
	err = fetree.ForEach(func(addr address.Address, act *types.Actor) error {
		switch act.Code {
		case builtin.AccountActorCodeID, builtin.MultisigActorCodeID, builtin.PaymentChannelActorCodeID:
			sysAcc, err := isSystemAccount(addr)
			if err != nil {
				return xerrors.Errorf("checking system account: %w", err)
			}

			if !sysAcc {
				transfers = append(transfers, transfer{
					From: addr,
					To:   ReserveAddress,
					Amt:  act.Balance,
				})
			}
		case builtin.StorageMinerActorCodeID:
			var st miner.State
			if err := sm.WithActorState(ctx, &st)(act); err != nil {
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

			transfers = append(transfers, transfer{
				From: addr,
				To:   ReserveAddress,
				Amt:  available,
			})
		}
		return nil
	})
	if err != nil {
		return xerrors.Errorf("foreach over state tree failed: %w", err)
	}

	// Execute transfers from previous step
	for _, t := range transfers {
		if err := doTransfer(tree, t.From, t.To, t.Amt); err != nil {
			return xerrors.Errorf("transfer %s %s->%s failed: %w", t.Amt, t.From, t.To, err)
		}
	}

	// pull up power table to give miners back some funds proportional to their power
	var ps power.State
	powAct, err := tree.GetActor(builtin.StoragePowerActorAddr)
	if err != nil {
		return xerrors.Errorf("failed to load power actor: %w", err)
	}

	cst := cbor.NewCborStore(sm.ChainStore().Blockstore())
	if err := cst.Get(ctx, powAct.Head, &ps); err != nil {
		return xerrors.Errorf("failed to get power actor state: %w", err)
	}

	totalPower := ps.TotalBytesCommitted

	var transfersBack []transfer
	// Now, we return some funds to places where they are needed
	err = fetree.ForEach(func(addr address.Address, act *types.Actor) error {
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
		case builtin.AccountActorCodeID, builtin.MultisigActorCodeID, builtin.PaymentChannelActorCodeID:
			nbalance := big.Min(prevBalance, AccountCap)
			if nbalance.Sign() != 0 {
				transfersBack = append(transfersBack, transfer{
					From: ReserveAddress,
					To:   addr,
					Amt:  nbalance,
				})
			}
		case builtin.StorageMinerActorCodeID:
			var st miner.State
			if err := sm.WithActorState(ctx, &st)(act); err != nil {
				return xerrors.Errorf("failed to load miner state: %w", err)
			}

			var minfo miner.MinerInfo
			if err := cst.Get(ctx, st.Info, &minfo); err != nil {
				return xerrors.Errorf("failed to get miner info: %w", err)
			}

			sectorsArr, err := adt.AsArray(sm.ChainStore().Store(ctx), st.Sectors)
			if err != nil {
				return xerrors.Errorf("failed to load sectors array: %w", err)
			}

			slen := sectorsArr.Length()

			power := types.BigMul(types.NewInt(slen), types.NewInt(uint64(minfo.SectorSize)))

			mfunds := minerFundsAlloc(power, totalPower)
			transfersBack = append(transfersBack, transfer{
				From: ReserveAddress,
				To:   minfo.Worker,
				Amt:  mfunds,
			})

			// Now make sure to give each miner who had power at the lookback some FIL
			lbact, err := lbtree.GetActor(addr)
			if err == nil {
				var lbst miner.State
				if err := sm.WithActorState(ctx, &lbst)(lbact); err != nil {
					return xerrors.Errorf("failed to load miner state: %w", err)
				}

				lbsectors, err := adt.AsArray(sm.ChainStore().Store(ctx), lbst.Sectors)
				if err != nil {
					return xerrors.Errorf("failed to load lb sectors array: %w", err)
				}

				if lbsectors.Length() > 0 {
					transfersBack = append(transfersBack, transfer{
						From: ReserveAddress,
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
		return xerrors.Errorf("foreach over state tree failed: %w", err)
	}

	for _, t := range transfersBack {
		if err := doTransfer(tree, t.From, t.To, t.Amt); err != nil {
			return xerrors.Errorf("transfer %s %s->%s failed: %w", t.Amt, t.From, t.To, err)
		}
	}

	// transfer all burnt funds back to the reserve account
	burntAct, err := tree.GetActor(builtin.BurntFundsActorAddr)
	if err != nil {
		return xerrors.Errorf("failed to load burnt funds actor: %w", err)
	}
	if err := doTransfer(tree, builtin.BurntFundsActorAddr, ReserveAddress, burntAct.Balance); err != nil {
		return xerrors.Errorf("failed to unburn funds: %w", err)
	}

	// Top up the reimbursement service
	reimbAddr, err := address.NewFromString("t0111")
	if err != nil {
		return xerrors.Errorf("failed to parse reimbursement service address")
	}

	reimb, err := tree.GetActor(reimbAddr)
	if err != nil {
		return xerrors.Errorf("failed to load reimbursement account actor: %w", err)
	}

	difference := types.BigSub(DesiredReimbursementBalance, reimb.Balance)
	if err := doTransfer(tree, ReserveAddress, reimbAddr, difference); err != nil {
		return xerrors.Errorf("failed to top up reimbursement account: %w", err)
	}

	// Now, a final sanity check to make sure the balances all check out
	total := abi.NewTokenAmount(0)
	err = fetree.ForEach(func(addr address.Address, act *types.Actor) error {
		total = types.BigAdd(total, act.Balance)
		return nil
	})
	if err != nil {
		return xerrors.Errorf("checking final state balance failed: %w", err)
	}

	exp := types.FromFil(build.FilBase)
	if !exp.Equals(total) {
		return xerrors.Errorf("resultant state tree account balance was not correct: %s", total)
	}

	return nil
}
