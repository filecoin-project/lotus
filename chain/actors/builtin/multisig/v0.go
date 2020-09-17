package multisig

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
)

type v0State struct {
	multisig.State
	store adt.Store
}

func (s *v0State) LockedBalance(currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.State.AmountLocked(currEpoch - s.State.StartEpoch), nil
}

func (s *v0State) StartEpoch() abi.ChainEpoch {
	return s.State.StartEpoch
}

func (s *v0State) UnlockDuration() abi.ChainEpoch {
	return s.State.UnlockDuration
}

func (s *v0State) InitialBalance() abi.TokenAmount {
	return s.State.InitialBalance
}
