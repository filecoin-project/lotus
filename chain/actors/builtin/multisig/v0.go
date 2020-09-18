package multisig

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
)

type state0 struct {
	multisig.State
	store adt.Store
}

func (s *state0) LockedBalance(currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.State.AmountLocked(currEpoch - s.State.StartEpoch), nil
}

func (s *state0) StartEpoch() abi.ChainEpoch {
	return s.State.StartEpoch
}

func (s *state0) UnlockDuration() abi.ChainEpoch {
	return s.State.UnlockDuration
}

func (s *state0) InitialBalance() abi.TokenAmount {
	return s.State.InitialBalance
}
