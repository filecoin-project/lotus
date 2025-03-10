package stmgr

import (
	"context"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	msig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	_init "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
)

// sets up information about the vesting schedule
func (sm *StateManager) setupGenesisVestingSchedule(ctx context.Context) error {

	gb, err := sm.cs.GetGenesis(ctx)
	if err != nil {
		return xerrors.Errorf("getting genesis block: %w", err)
	}

	gts, err := types.NewTipSet([]*types.BlockHeader{gb})
	if err != nil {
		return xerrors.Errorf("getting genesis tipset: %w", err)
	}

	st, _, err := sm.TipSetState(ctx, gts)
	if err != nil {
		return xerrors.Errorf("getting genesis tipset state: %w", err)
	}

	cst := cbor.NewCborStore(sm.cs.StateBlockstore())
	sTree, err := state.LoadStateTree(cst, st)
	if err != nil {
		return xerrors.Errorf("loading state tree: %w", err)
	}

	gp, err := getFilPowerLocked(ctx, sTree)
	if err != nil {
		return xerrors.Errorf("setting up genesis pledge: %w", err)
	}

	sm.genesisMsigLk.Lock()
	defer sm.genesisMsigLk.Unlock()
	sm.genesisPledge = gp

	totalsByEpoch := make(map[abi.ChainEpoch]abi.TokenAmount)

	// 6 months
	sixMonths := abi.ChainEpoch(183 * builtin.EpochsInDay)
	totalsByEpoch[sixMonths] = big.NewInt(49_929_341)
	totalsByEpoch[sixMonths] = big.Add(totalsByEpoch[sixMonths], big.NewInt(32_787_700))

	// 1 year
	oneYear := abi.ChainEpoch(365 * builtin.EpochsInDay)
	totalsByEpoch[oneYear] = big.NewInt(22_421_712)

	// 2 years
	twoYears := abi.ChainEpoch(2 * 365 * builtin.EpochsInDay)
	totalsByEpoch[twoYears] = big.NewInt(7_223_364)

	// 3 years
	threeYears := abi.ChainEpoch(3 * 365 * builtin.EpochsInDay)
	totalsByEpoch[threeYears] = big.NewInt(87_637_883)

	// 6 years
	sixYears := abi.ChainEpoch(6 * 365 * builtin.EpochsInDay)
	totalsByEpoch[sixYears] = big.NewInt(100_000_000)
	totalsByEpoch[sixYears] = big.Add(totalsByEpoch[sixYears], big.NewInt(300_000_000))

	sm.preIgnitionVesting = make([]msig0.State, 0, len(totalsByEpoch))
	for k, v := range totalsByEpoch {
		ns := msig0.State{
			InitialBalance: v,
			UnlockDuration: k,
			PendingTxns:    cid.Undef,
		}
		sm.preIgnitionVesting = append(sm.preIgnitionVesting, ns)
	}

	return nil
}

// sets up information about the vesting schedule post the ignition upgrade
func (sm *StateManager) setupPostIgnitionVesting(ctx context.Context) error {

	totalsByEpoch := make(map[abi.ChainEpoch]abi.TokenAmount)

	// 6 months
	sixMonths := abi.ChainEpoch(183 * builtin.EpochsInDay)
	totalsByEpoch[sixMonths] = big.NewInt(49_929_341)
	totalsByEpoch[sixMonths] = big.Add(totalsByEpoch[sixMonths], big.NewInt(32_787_700))

	// 1 year
	oneYear := abi.ChainEpoch(365 * builtin.EpochsInDay)
	totalsByEpoch[oneYear] = big.NewInt(22_421_712)

	// 2 years
	twoYears := abi.ChainEpoch(2 * 365 * builtin.EpochsInDay)
	totalsByEpoch[twoYears] = big.NewInt(7_223_364)

	// 3 years
	threeYears := abi.ChainEpoch(3 * 365 * builtin.EpochsInDay)
	totalsByEpoch[threeYears] = big.NewInt(87_637_883)

	// 6 years
	sixYears := abi.ChainEpoch(6 * 365 * builtin.EpochsInDay)
	totalsByEpoch[sixYears] = big.NewInt(100_000_000)
	totalsByEpoch[sixYears] = big.Add(totalsByEpoch[sixYears], big.NewInt(300_000_000))

	sm.genesisMsigLk.Lock()
	defer sm.genesisMsigLk.Unlock()
	sm.postIgnitionVesting = make([]msig0.State, 0, len(totalsByEpoch))
	for k, v := range totalsByEpoch {
		ns := msig0.State{
			// In the pre-ignition logic, we incorrectly set this value in Fil, not attoFil, an off-by-10^18 error
			InitialBalance: big.Mul(v, big.NewInt(int64(buildconstants.FilecoinPrecision))),
			UnlockDuration: k,
			PendingTxns:    cid.Undef,
			// In the pre-ignition logic, the start epoch was 0. This changes in the fork logic of the Ignition upgrade itself.
			StartEpoch: buildconstants.UpgradeLiftoffHeight,
		}
		sm.postIgnitionVesting = append(sm.postIgnitionVesting, ns)
	}

	return nil
}

// sets up information about the vesting schedule post the calico upgrade
func (sm *StateManager) setupPostCalicoVesting(ctx context.Context) error {

	totalsByEpoch := make(map[abi.ChainEpoch]abi.TokenAmount)

	// 0 days
	zeroDays := abi.ChainEpoch(0)
	totalsByEpoch[zeroDays] = big.NewInt(10_632_000)

	// 6 months
	sixMonths := abi.ChainEpoch(183 * builtin.EpochsInDay)
	totalsByEpoch[sixMonths] = big.NewInt(19_015_887)
	totalsByEpoch[sixMonths] = big.Add(totalsByEpoch[sixMonths], big.NewInt(32_787_700))

	// 1 year
	oneYear := abi.ChainEpoch(365 * builtin.EpochsInDay)
	totalsByEpoch[oneYear] = big.NewInt(22_421_712)
	totalsByEpoch[oneYear] = big.Add(totalsByEpoch[oneYear], big.NewInt(9_400_000))

	// 2 years
	twoYears := abi.ChainEpoch(2 * 365 * builtin.EpochsInDay)
	totalsByEpoch[twoYears] = big.NewInt(7_223_364)

	// 3 years
	threeYears := abi.ChainEpoch(3 * 365 * builtin.EpochsInDay)
	totalsByEpoch[threeYears] = big.NewInt(87_637_883)
	totalsByEpoch[threeYears] = big.Add(totalsByEpoch[threeYears], big.NewInt(898_958))

	// 6 years
	sixYears := abi.ChainEpoch(6 * 365 * builtin.EpochsInDay)
	totalsByEpoch[sixYears] = big.NewInt(100_000_000)
	totalsByEpoch[sixYears] = big.Add(totalsByEpoch[sixYears], big.NewInt(300_000_000))
	totalsByEpoch[sixYears] = big.Add(totalsByEpoch[sixYears], big.NewInt(9_805_053))

	sm.genesisMsigLk.Lock()
	defer sm.genesisMsigLk.Unlock()

	sm.postCalicoVesting = make([]msig0.State, 0, len(totalsByEpoch))
	for k, v := range totalsByEpoch {
		ns := msig0.State{
			InitialBalance: big.Mul(v, big.NewInt(int64(buildconstants.FilecoinPrecision))),
			UnlockDuration: k,
			PendingTxns:    cid.Undef,
			StartEpoch:     buildconstants.UpgradeLiftoffHeight,
		}
		sm.postCalicoVesting = append(sm.postCalicoVesting, ns)
	}

	return nil
}

// GetFilVested returns all funds that have "left" actors that are in the genesis state:
// - For Multisigs, it counts the actual amounts that have vested at the given epoch
// - For Accounts, it counts max(currentBalance - genesisBalance, 0).
func (sm *StateManager) GetFilVested(ctx context.Context, height abi.ChainEpoch) (abi.TokenAmount, error) {
	vf := big.Zero()

	// TODO: combine all this?
	if sm.preIgnitionVesting == nil || sm.genesisPledge.IsZero() {
		err := sm.setupGenesisVestingSchedule(ctx)
		if err != nil {
			return vf, xerrors.Errorf("failed to setup pre-ignition vesting schedule: %w", err)
		}

	}
	if sm.postIgnitionVesting == nil {
		err := sm.setupPostIgnitionVesting(ctx)
		if err != nil {
			return vf, xerrors.Errorf("failed to setup post-ignition vesting schedule: %w", err)
		}

	}
	if sm.postCalicoVesting == nil {
		err := sm.setupPostCalicoVesting(ctx)
		if err != nil {
			return vf, xerrors.Errorf("failed to setup post-calico vesting schedule: %w", err)
		}
	}

	if height <= buildconstants.UpgradeIgnitionHeight {
		for _, v := range sm.preIgnitionVesting {
			au := big.Sub(v.InitialBalance, v.AmountLocked(height))
			vf = big.Add(vf, au)
		}
	} else if height <= buildconstants.UpgradeCalicoHeight {
		for _, v := range sm.postIgnitionVesting {
			// In the pre-ignition logic, we simply called AmountLocked(height), assuming startEpoch was 0.
			// The start epoch changed in the Ignition upgrade.
			au := big.Sub(v.InitialBalance, v.AmountLocked(height-v.StartEpoch))
			vf = big.Add(vf, au)
		}
	} else {
		for _, v := range sm.postCalicoVesting {
			// In the pre-ignition logic, we simply called AmountLocked(height), assuming startEpoch was 0.
			// The start epoch changed in the Ignition upgrade.
			au := big.Sub(v.InitialBalance, v.AmountLocked(height-v.StartEpoch))
			vf = big.Add(vf, au)
		}
	}

	// After UpgradeAssemblyHeight these funds are accounted for in GetFilReserveDisbursed
	if height <= buildconstants.UpgradeAssemblyHeight {
		// continue to use preIgnitionGenInfos, nothing changed at the Ignition epoch
		vf = big.Add(vf, sm.genesisPledge)
	}

	return vf, nil
}

func GetFilReserveDisbursed(ctx context.Context, st *state.StateTree, nv network.Version) (abi.TokenAmount, error) {
	ract, err := st.GetActor(builtin.ReserveAddress)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to get reserve actor: %w", err)
	}

	initial := buildconstants.InitialFilReserved
	if nv >= network.Version25 {
		// See FIP-0100 and https://github.com/filecoin-project/lotus/pull/12938 for why this exists
		initial = buildconstants.UpgradeTeepInitialFilReserved
	}
	// If money enters the reserve actor, this could lead to a negative term
	return big.Sub(big.NewFromGo(initial), ract.Balance), nil
}

func GetFilMined(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	ractor, err := st.GetActor(reward.Address)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load reward actor state: %w", err)
	}

	rst, err := reward.Load(adt.WrapStore(ctx, st.Store), ractor)
	if err != nil {
		return big.Zero(), err
	}

	return rst.TotalStoragePowerReward()
}

func getFilMarketLocked(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	act, err := st.GetActor(market.Address)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load market actor: %w", err)
	}

	mst, err := market.Load(adt.WrapStore(ctx, st.Store), act)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load market state: %w", err)
	}

	return mst.TotalLocked()
}

func getFilPowerLocked(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	pactor, err := st.GetActor(power.Address)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load power actor: %w", err)
	}

	pst, err := power.Load(adt.WrapStore(ctx, st.Store), pactor)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load power state: %w", err)
	}

	return pst.TotalLocked()
}

func GetFilLocked(ctx context.Context, st *state.StateTree, nv network.Version) (abi.TokenAmount, error) {
	if nv >= network.Version23 {
		return getFilPowerLocked(ctx, st)
	}

	filMarketLocked, err := getFilMarketLocked(ctx, st)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to get filMarketLocked: %w", err)
	}

	filPowerLocked, err := getFilPowerLocked(ctx, st)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to get filPowerLocked: %w", err)
	}

	return types.BigAdd(filMarketLocked, filPowerLocked), nil
}

func GetFilBurnt(ctx context.Context, st *state.StateTree) (abi.TokenAmount, error) {
	burnt, err := st.GetActor(builtin.BurntFundsActorAddr)
	if err != nil {
		return big.Zero(), xerrors.Errorf("failed to load burnt actor: %w", err)
	}

	return burnt.Balance, nil
}

func (sm *StateManager) GetVMCirculatingSupply(ctx context.Context, height abi.ChainEpoch, st *state.StateTree) (abi.TokenAmount, error) {
	cs, err := sm.GetVMCirculatingSupplyDetailed(ctx, height, st)
	if err != nil {
		return types.EmptyInt, err
	}

	return cs.FilCirculating, err
}

func (sm *StateManager) GetVMCirculatingSupplyDetailed(ctx context.Context, height abi.ChainEpoch, st *state.StateTree) (api.CirculatingSupply, error) {
	nv := sm.GetNetworkVersion(ctx, height)
	filVested, err := sm.GetFilVested(ctx, height)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filVested: %w", err)
	}

	filReserveDisbursed := big.Zero()
	if height > buildconstants.UpgradeAssemblyHeight {
		filReserveDisbursed, err = GetFilReserveDisbursed(ctx, st, nv)
		if err != nil {
			return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filReserveDisbursed: %w", err)
		}
	}

	filMined, err := GetFilMined(ctx, st)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filMined: %w", err)
	}

	filBurnt, err := GetFilBurnt(ctx, st)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filBurnt: %w", err)
	}

	filLocked, err := GetFilLocked(ctx, st, nv)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("failed to calculate filLocked: %w", err)
	}

	ret := types.BigAdd(filVested, filMined)
	ret = types.BigAdd(ret, filReserveDisbursed)
	ret = types.BigSub(ret, filBurnt)
	ret = types.BigSub(ret, filLocked)

	if ret.LessThan(big.Zero()) {
		ret = big.Zero()
	}

	return api.CirculatingSupply{
		FilVested:           filVested,
		FilMined:            filMined,
		FilBurnt:            filBurnt,
		FilLocked:           filLocked,
		FilCirculating:      ret,
		FilReserveDisbursed: filReserveDisbursed,
	}, nil
}

func (sm *StateManager) GetCirculatingSupply(ctx context.Context, height abi.ChainEpoch, st *state.StateTree) (abi.TokenAmount, error) {
	circ := big.Zero()
	unCirc := big.Zero()
	nv := sm.GetNetworkVersion(ctx, height)

	err := st.ForEach(func(a address.Address, actor *types.Actor) error {
		// this can be a lengthy operation, we need to cancel early when
		// the context is cancelled to avoid resource exhaustion
		select {
		case <-ctx.Done():
			// this will cause ForEach to return
			return ctx.Err()
		default:
		}
		switch {
		case actor.Balance.IsZero():
			// Do nothing for zero-balance actors
			break
		case a == _init.Address ||
			a == reward.Address ||
			a == verifreg.Address ||
			// The power actor itself should never receive funds
			a == power.Address ||
			a == builtin.SystemActorAddr ||
			a == builtin.CronActorAddr ||
			a == builtin.BurntFundsActorAddr ||
			a == builtin.SaftAddress ||
			a == builtin.ReserveAddress ||
			a == builtin.EthereumAddressManagerActorAddr ||
			a == builtin.DatacapActorAddr:

			unCirc = big.Add(unCirc, actor.Balance)

		case a == market.Address:
			if nv >= network.Version23 {
				circ = big.Add(circ, actor.Balance)
			} else {
				mst, err := market.Load(sm.cs.ActorStore(ctx), actor)
				if err != nil {
					return err
				}

				lb, err := mst.TotalLocked()
				if err != nil {
					return err
				}

				circ = big.Add(circ, big.Sub(actor.Balance, lb))
				unCirc = big.Add(unCirc, lb)
			}

		case builtin.IsAccountActor(actor.Code) ||
			builtin.IsPaymentChannelActor(actor.Code) ||
			builtin.IsEthAccountActor(actor.Code) ||
			builtin.IsEvmActor(actor.Code) ||
			builtin.IsPlaceholderActor(actor.Code):

			circ = big.Add(circ, actor.Balance)

		case builtin.IsStorageMinerActor(actor.Code):
			mst, err := miner.Load(sm.cs.ActorStore(ctx), actor)
			if err != nil {
				return err
			}

			ab, err := mst.AvailableBalance(actor.Balance)

			if err == nil {
				circ = big.Add(circ, ab)
				unCirc = big.Add(unCirc, big.Sub(actor.Balance, ab))
			} else {
				// Assume any error is because the miner state is "broken" (lower actor balance than locked funds)
				// In this case, the actor's entire balance is considered "uncirculating"
				unCirc = big.Add(unCirc, actor.Balance)
			}

		case builtin.IsMultisigActor(actor.Code):
			mst, err := multisig.Load(sm.cs.ActorStore(ctx), actor)
			if err != nil {
				return err
			}

			lb, err := mst.LockedBalance(height)
			if err != nil {
				return err
			}

			ab := big.Sub(actor.Balance, lb)
			circ = big.Add(circ, big.Max(ab, big.Zero()))
			unCirc = big.Add(unCirc, big.Min(actor.Balance, lb))
		default:
			return xerrors.Errorf("unexpected actor: %s", a)
		}

		return nil
	})

	if err != nil {
		return types.EmptyInt, err
	}

	total := big.Add(circ, unCirc)
	if !total.Equals(types.TotalFilecoinInt) {
		return types.EmptyInt, xerrors.Errorf("total filecoin didn't add to expected amount: %s != %s", total, types.TotalFilecoinInt)
	}

	return circ, nil
}
