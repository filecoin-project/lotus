package genesis

import (
	"context"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func mustEnc(i cbg.CBORMarshaler) []byte {
	enc, err := actors.SerializeParams(i)
	if err != nil {
		panic(err) // ok
	}
	return enc
}

func doExecValue(ctx context.Context, vm vm.Interface, to, from address.Address, value types.BigInt, method abi.MethodNum, params []byte) ([]byte, error) {
	ret, err := vm.ApplyImplicitMessage(ctx, &types.Message{
		To:       to,
		From:     from,
		Method:   method,
		Params:   params,
		GasLimit: 1_000_000_000_000_000,
		Value:    value,
		Nonce:    0,
	})
	if err != nil {
		return nil, xerrors.Errorf("doExec apply message failed: %w", err)
	}

	if ret.ExitCode != 0 {
		return nil, xerrors.Errorf("failed to call method: %w", ret.ActorErr)
	}

	return ret.Return, nil
}

// CalculateMinerCreationDepositForGenesis calculates the deposit required for creating a new miner
// during genesis block creation. This is used when API is not available yet.
func CalculateMinerCreationDepositForGenesis(ctx context.Context, vm vm.Interface, store adt.Store) (abi.TokenAmount, error) {
	nh, err := vm.Flush(ctx)
	if err != nil {
		return big.Zero(), xerrors.Errorf("flushing vm: %w", err)
	}

	nst, err := state.LoadStateTree(store, nh)
	if err != nil {
		return big.Zero(), xerrors.Errorf("loading new state tree: %w", err)
	}
	// Get reward actor state
	rewardActor, err := nst.GetActor(reward.Address)
	if err != nil {
		return big.Zero(), xerrors.Errorf("loading reward actor: %w", err)
	}

	rewardState, err := reward.Load(store, rewardActor)
	if err != nil {
		return big.Zero(), xerrors.Errorf("loading reward actor state: %w", err)
	}

	// Get power actor state
	powerActor, err := nst.GetActor(power.Address)
	if err != nil {
		return big.Zero(), xerrors.Errorf("loading power actor: %w", err)
	}

	powerState, err := power.Load(store, powerActor)
	if err != nil {
		return big.Zero(), xerrors.Errorf("loading power actor state: %w", err)
	}

	networkQAPowerSmoothed, err := powerState.TotalPowerSmoothed()
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting network QA power smoothed: %w", err)
	}

	pledgeCollateral, err := powerState.TotalLocked()
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting pledge collateral: %w", err)
	}

	// For genesis, use the initial reserved amount as default circulating supply
	circSupply := types.BigInt{Int: buildconstants.InitialFilReserved}

	// Calculate the deposit power as 1/10 of consensus miner minimum power
	createMinerDepositPower := big.Div(buildconstants.ConsensusMinerMinPower, big.NewInt(10))

	// For genesis, assume we're at the start of the ramp
	rampDurationEpochs := powerState.RampDurationEpochs()
	epochsSinceRampStart := int64(0) // Genesis is at the beginning

	// Use the reward state's InitialPledgeForPower function to calculate the deposit
	deposit, err := rewardState.InitialPledgeForPower(
		createMinerDepositPower,
		pledgeCollateral,
		&networkQAPowerSmoothed,
		circSupply,
		epochsSinceRampStart,
		rampDurationEpochs,
	)
	if err != nil {
		return big.Zero(), xerrors.Errorf("calculating initial pledge for power: %w", err)
	}

	return deposit, nil
}
