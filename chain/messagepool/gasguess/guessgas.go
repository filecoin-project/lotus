package gasguess

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
)

type ActorLookup func(context.Context, address.Address, types.TipSetKey) (*types.Actor, error)

const failedGasGuessRatio = 0.5
const failedGasGuessMax = 25_000_000

type CostKey struct {
	Code cid.Cid
	M    abi.MethodNum
}

var Costs = map[CostKey]int64{
	{builtin.InitActorCodeID, 2}:          8916753,
	{builtin.StorageMarketActorCodeID, 2}: 6955002,
	{builtin.StorageMarketActorCodeID, 4}: 245436108,
	{builtin.StorageMinerActorCodeID, 4}:  2315133,
	{builtin.StorageMinerActorCodeID, 5}:  1600271356,
	{builtin.StorageMinerActorCodeID, 6}:  22864493,
	{builtin.StorageMinerActorCodeID, 7}:  142002419,
	{builtin.StorageMinerActorCodeID, 10}: 23008274,
	{builtin.StorageMinerActorCodeID, 11}: 19303178,
	{builtin.StorageMinerActorCodeID, 14}: 566356835,
	{builtin.StorageMinerActorCodeID, 16}: 5325185,
	{builtin.StorageMinerActorCodeID, 18}: 2328637,
	{builtin.StoragePowerActorCodeID, 2}:  23600956,
}

func failedGuess(msg *types.SignedMessage) int64 {
	guess := int64(float64(msg.Message.GasLimit) * failedGasGuessRatio)
	if guess > failedGasGuessMax {
		guess = failedGasGuessMax
	}
	return guess
}

func GuessGasUsed(ctx context.Context, tsk types.TipSetKey, msg *types.SignedMessage, al ActorLookup) (int64, error) {
	if msg.Message.Method == builtin.MethodSend {
		switch msg.Message.From.Protocol() {
		case address.BLS:
			return 1298450, nil
		case address.SECP256K1:
			return 1385999, nil
		default:
			// who knows?
			return 1298450, nil
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
