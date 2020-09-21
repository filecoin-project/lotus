package multisig

import (
	"encoding/binary"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"golang.org/x/xerrors"

	msig0 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
)

var _ State = (*state0)(nil)

type state0 struct {
	msig0.State
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

func (s *state0) Threshold() uint64 {
	return s.State.NumApprovalsThreshold
}

func (s *state0) Signers() []address.Address {
	return s.State.Signers
}

func (s *state0) ForEachPendingTxn(cb func(id int64, txn Transaction) error) error {
	arr, err := adt0.AsMap(s.store, s.State.PendingTxns)
	if err != nil {
		return err
	}
	var out msig0.Transaction
	return arr.ForEach(&out, func(key string) error {
		txid, n := binary.Varint([]byte(key))
		if n <= 0 {
			return xerrors.Errorf("invalid pending transaction key: %v", key)
		}
		return cb(txid, (Transaction)(out))
	})
}
