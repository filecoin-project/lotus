package multisig

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"
)

var _ State = (*state1)(nil)

type state1 struct {
	multisig.State
	store adt.Store
}

func (s *state1) LockedBalance(currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.State.AmountLocked(currEpoch - s.State.StartEpoch), nil
}

func (s *state1) StartEpoch() abi.ChainEpoch {
	return s.State.StartEpoch
}

func (s *state1) UnlockDuration() abi.ChainEpoch {
	return s.State.UnlockDuration
}

func (s *state1) InitialBalance() abi.TokenAmount {
	return s.State.InitialBalance
}
