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
	"github.com/filecoin-project/go-state-types/manifest"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	msig5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/multisig"
	adt5 "github.com/filecoin-project/specs-actors/v5/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state5)(nil)

func load5(store adt.Store, root cid.Cid) (State, error) {
	out := state5{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make5(store adt.Store, signers []address.Address, threshold uint64, startEpoch abi.ChainEpoch, unlockDuration abi.ChainEpoch, initialBalance abi.TokenAmount) (State, error) {
	out := state5{store: store}
	out.State = msig5.State{}
	out.State.Signers = signers
	out.State.NumApprovalsThreshold = threshold
	out.State.StartEpoch = startEpoch
	out.State.UnlockDuration = unlockDuration
	out.State.InitialBalance = initialBalance

	em, err := adt5.StoreEmptyMap(store, builtin5.DefaultHamtBitwidth)
	if err != nil {
		return nil, err
	}

	out.State.PendingTxns = em

	return &out, nil
}

type state5 struct {
	msig5.State
	store adt.Store
}

func (s *state5) LockedBalance(currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.State.AmountLocked(currEpoch - s.State.StartEpoch), nil
}

func (s *state5) StartEpoch() (abi.ChainEpoch, error) {
	return s.State.StartEpoch, nil
}

func (s *state5) UnlockDuration() (abi.ChainEpoch, error) {
	return s.State.UnlockDuration, nil
}

func (s *state5) InitialBalance() (abi.TokenAmount, error) {
	return s.State.InitialBalance, nil
}

func (s *state5) Threshold() (uint64, error) {
	return s.State.NumApprovalsThreshold, nil
}

func (s *state5) Signers() ([]address.Address, error) {
	return s.State.Signers, nil
}

func (s *state5) ForEachPendingTxn(cb func(id int64, txn Transaction) error) error {
	arr, err := adt5.AsMap(s.store, s.State.PendingTxns, builtin5.DefaultHamtBitwidth)
	if err != nil {
		return err
	}
	var out msig5.Transaction
	return arr.ForEach(&out, func(key string) error {
		txid, n := binary.Varint([]byte(key))
		if n <= 0 {
			return xerrors.Errorf("invalid pending transaction key: %v", key)
		}
		return cb(txid, (Transaction)(out)) //nolint:unconvert
	})
}

func (s *state5) PendingTxnChanged(other State) (bool, error) {
	other5, ok := other.(*state5)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.PendingTxns.Equals(other5.PendingTxns), nil
}

func (s *state5) transactions() (adt.Map, error) {
	return adt5.AsMap(s.store, s.PendingTxns, builtin5.DefaultHamtBitwidth)
}

func (s *state5) decodeTransaction(val *cbg.Deferred) (Transaction, error) {
	var tx msig5.Transaction
	if err := tx.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Transaction{}, err
	}
	return Transaction(tx), nil
}

func (s *state5) GetState() interface{} {
	return &s.State
}

func (s *state5) ActorKey() string {
	return manifest.MultisigKey
}

func (s *state5) ActorVersion() actorstypes.Version {
	return actorstypes.Version5
}

func (s *state5) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
