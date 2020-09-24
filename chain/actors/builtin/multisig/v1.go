package multisig

import (
	"encoding/binary"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/adt"

	msig1 "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	adt1 "github.com/filecoin-project/specs-actors/actors/util/adt"
)

var _ State = (*state1)(nil)

func load1(store adt.Store, root cid.Cid) (State, error) {
	out := state1{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type state1 struct {
	msig1.State
	store adt.Store
}

func (s *state1) LockedBalance(currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.State.AmountLocked(currEpoch - s.State.StartEpoch), nil
}

func (s *state1) StartEpoch() (abi.ChainEpoch, error) {
	return s.State.StartEpoch, nil
}

func (s *state1) UnlockDuration() (abi.ChainEpoch, error) {
	return s.State.UnlockDuration, nil
}

func (s *state1) InitialBalance() (abi.TokenAmount, error) {
	return s.State.InitialBalance, nil
}

func (s *state1) Threshold() (uint64, error) {
	return s.State.NumApprovalsThreshold, nil
}

func (s *state1) Signers() ([]address.Address, error) {
	return s.State.Signers, nil
}

func (s *state1) ForEachPendingTxn(cb func(id int64, txn Transaction) error) error {
	arr, err := adt1.AsMap(s.store, s.State.PendingTxns)
	if err != nil {
		return err
	}
	var out msig1.Transaction
	return arr.ForEach(&out, func(key string) error {
		txid, n := binary.Varint([]byte(key))
		if n <= 0 {
			return xerrors.Errorf("invalid pending transaction key: %v", key)
		}
		return cb(txid, (Transaction)(out))
	})
}
