package gasguess

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
)

type ActorLookup func(context.Context, address.Address, types.TipSetKey) (*types.Actor, error)

const failedGasGuessRatio = 0.5
const failedGasGuessMax = 25_000_000

const MinGas = 1298450
const MaxGas = 1600271356

type CostKey struct {
	Code cid.Cid
	M    abi.MethodNum
}

var Costs = map[CostKey]int64{
	{builtin0.InitActorCodeID, builtin0.MethodsInit.Exec}:                            15132024,
	{builtin0.StorageMarketActorCodeID, builtin0.MethodsMarket.AddBalance}:           25889982,
	{builtin0.StorageMarketActorCodeID, builtin0.MethodsMarket.PublishStorageDeals}:  73447170,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.ChangePeerID}:           2057027,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.SubmitWindowedPoSt}:     448890633,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.PreCommitSector}:        22785818,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.ProveCommitSector}:      63443589,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.DeclareFaults}:          23008274,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.DeclareFaultsRecovered}: 40692097,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.AddLockedFund}:          566356835,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.WithdrawBalance}:        19964889,
	{builtin0.StorageMinerActorCodeID, builtin0.MethodsMiner.ChangeMultiaddrs}:       6403604,
	{builtin0.StoragePowerActorCodeID, builtin0.MethodsPower.CreateMiner}:            41679759,
	// TODO: Just reuse v0 values for now, this isn't actually used
	{builtin2.InitActorCodeID, builtin2.MethodsInit.Exec}:                            15132024,
	{builtin2.StorageMarketActorCodeID, builtin2.MethodsMarket.AddBalance}:           25889982,
	{builtin2.StorageMarketActorCodeID, builtin2.MethodsMarket.PublishStorageDeals}:  73447170,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.ChangePeerID}:           2057027,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.SubmitWindowedPoSt}:     448890633,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.PreCommitSector}:        22785818,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.ProveCommitSector}:      63443589,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.DeclareFaults}:          23008274,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.DeclareFaultsRecovered}: 40692097,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.ApplyRewards}:           566356835,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.WithdrawBalance}:        19964889,
	{builtin2.StorageMinerActorCodeID, builtin2.MethodsMiner.ChangeMultiaddrs}:       6403604,
	{builtin2.StoragePowerActorCodeID, builtin2.MethodsPower.CreateMiner}:            41679759,
}

func failedGuess(msg *types.SignedMessage) int64 {
	guess := int64(float64(msg.Message.GasLimit) * failedGasGuessRatio)
	if guess > failedGasGuessMax {
		guess = failedGasGuessMax
	}
	return guess
}

func GuessGasUsed(ctx context.Context, tsk types.TipSetKey, msg *types.SignedMessage, al ActorLookup) (int64, error) {
	// MethodSend is the same in all versions.
	if msg.Message.Method == builtin.MethodSend {
		switch msg.Message.From.Protocol() {
		case address.BLS:
			return 903526, nil
		case address.SECP256K1:
			return 985999, nil
		default:
			// who knows?
			return 903526, nil
		}
	}

	to, err := al(ctx, msg.Message.To, tsk)
	if err != nil {
		return failedGuess(msg), xerrors.Errorf("could not lookup actor: %w", err)
	}

	guess, ok := Costs[CostKey{to.Code, msg.Message.Method}]
	if !ok {
		return failedGuess(msg), xerrors.Errorf("unknown code-method combo")
	}
	if guess > msg.Message.GasLimit {
		guess = msg.Message.GasLimit
	}
	return guess, nil
}
