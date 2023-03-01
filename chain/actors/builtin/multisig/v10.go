package multisig

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin10 "github.com/filecoin-project/go-state-types/builtin"
	msig10 "github.com/filecoin-project/go-state-types/builtin/v10/multisig"
	adt10 "github.com/filecoin-project/go-state-types/builtin/v10/util/adt"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state10)(nil)

func load10(store adt.Store, root cid.Cid) (State, error) {
	out := state10{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make10(store adt.Store, signers []address.Address, threshold uint64, startEpoch abi.ChainEpoch, unlockDuration abi.ChainEpoch, initialBalance abi.TokenAmount) (State, error) {
	out := state10{store: store}
	out.State = msig10.State{}
	out.State.Signers = signers
	out.State.NumApprovalsThreshold = threshold
	out.State.StartEpoch = startEpoch
	out.State.UnlockDuration = unlockDuration
	out.State.InitialBalance = initialBalance

	em, err := adt10.StoreEmptyMap(store, builtin10.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}

	out.State.PendingTxns = em

	return &out, nil
}

type state10 struct {
	msig10.State
	store adt.Store
}

func (s *state10) LockedBalance(currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.State.AmountLocked(currEpoch - s.State.StartEpoch), nil
}

func (s *state10) StartEpoch() (abi.ChainEpoch, error) {
	return s.State.StartEpoch, nil
}

func (s *state10) UnlockDuration() (abi.ChainEpoch, error) {
	return s.State.UnlockDuration, nil
}

func (s *state10) InitialBalance() (abi.TokenAmount, error) {
	return s.State.InitialBalance, nil
}

func (s *state10) Threshold() (uint64, error) {
	return s.State.NumApprovalsThreshold, nil
}

func (s *state10) Signers() ([]address.Address, error) {
	return s.State.Signers, nil
}

func (s *state10) ForEachPendingTxn(cb func(id int64, txn Transaction) error) error {
	arr, err := adt10.AsMap(s.store, s.State.PendingTxns, builtin10.DefaultHamtBitwidth)
	if err != nil {
		return err
	}
	var out msig10.Transaction
	return arr.ForEach(&out, func(key string) error {
		txid, n := binary.Varint([]byte(key))
		if n <= 0 {
			return xerrors.Errorf("invalid pending transaction key: %v", key)
		}
		return cb(txid, (Transaction)(out)) //nolint:unconvert
	})
}

func (s *state10) PendingTxnChanged(other State) (bool, error) {
	other10, ok := other.(*state10)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.PendingTxns.Equals(other10.PendingTxns), nil
}

func (s *state10) transactions() (adt.Map, error) {
	return adt10.AsMap(s.store, s.PendingTxns, builtin10.DefaultHamtBitwidth)
}

func (s *state10) decodeTransaction(val *cbg.Deferred) (Transaction, error) {
	var tx msig10.Transaction
	if err := tx.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Transaction{}, err
	}
	return Transaction(tx), nil
}

func (s *state10) GetState() interface{} {
	return &s.State
}

func (s *state10) ActorKey() string {
	return manifest.MultisigKey
}

func (s *state10) ActorVersion() actorstypes.Version {
	return actorstypes.Version10
}

func (s *state10) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
