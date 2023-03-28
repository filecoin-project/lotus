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
	msig2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"
	adt2 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state2)(nil)

func load2(store adt.Store, root cid.Cid) (State, error) {
	out := state2{store: store}
	err := store.Get(store.Context(), root, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func make2(store adt.Store, signers []address.Address, threshold uint64, startEpoch abi.ChainEpoch, unlockDuration abi.ChainEpoch, initialBalance abi.TokenAmount) (State, error) {
	out := state2{store: store}
	out.State = msig2.State{}
	out.State.Signers = signers
	out.State.NumApprovalsThreshold = threshold
	out.State.StartEpoch = startEpoch
	out.State.UnlockDuration = unlockDuration
	out.State.InitialBalance = initialBalance

	em, err := adt2.MakeEmptyMap(store).Root()
	if err != nil {
		return nil, err
	}

	out.State.PendingTxns = em

	return &out, nil
}

type state2 struct {
	msig2.State
	store adt.Store
}

func (s *state2) LockedBalance(currEpoch abi.ChainEpoch) (abi.TokenAmount, error) {
	return s.State.AmountLocked(currEpoch - s.State.StartEpoch), nil
}

func (s *state2) StartEpoch() (abi.ChainEpoch, error) {
	return s.State.StartEpoch, nil
}

func (s *state2) UnlockDuration() (abi.ChainEpoch, error) {
	return s.State.UnlockDuration, nil
}

func (s *state2) InitialBalance() (abi.TokenAmount, error) {
	return s.State.InitialBalance, nil
}

func (s *state2) Threshold() (uint64, error) {
	return s.State.NumApprovalsThreshold, nil
}

func (s *state2) Signers() ([]address.Address, error) {
	return s.State.Signers, nil
}

func (s *state2) ForEachPendingTxn(cb func(id int64, txn Transaction) error) error {
	arr, err := adt2.AsMap(s.store, s.State.PendingTxns)
	if err != nil {
		return err
	}
	var out msig2.Transaction
	return arr.ForEach(&out, func(key string) error {
		txid, n := binary.Varint([]byte(key))
		if n <= 0 {
			return xerrors.Errorf("invalid pending transaction key: %v", key)
		}
		return cb(txid, (Transaction)(out)) //nolint:unconvert
	})
}

func (s *state2) PendingTxnChanged(other State) (bool, error) {
	other2, ok := other.(*state2)
	if !ok {
		// treat an upgrade as a change, always
		return true, nil
	}
	return !s.State.PendingTxns.Equals(other2.PendingTxns), nil
}

func (s *state2) transactions() (adt.Map, error) {
	return adt2.AsMap(s.store, s.PendingTxns)
}

func (s *state2) decodeTransaction(val *cbg.Deferred) (Transaction, error) {
	var tx msig2.Transaction
	if err := tx.UnmarshalCBOR(bytes.NewReader(val.Raw)); err != nil {
		return Transaction{}, err
	}
	return Transaction(tx), nil
}

func (s *state2) GetState() interface{} {
	return &s.State
}

func (s *state2) ActorKey() string {
	return manifest.MultisigKey
}

func (s *state2) ActorVersion() actorstypes.Version {
	return actorstypes.Version2
}

func (s *state2) Code() cid.Cid {
	code, ok := actors.GetActorCodeID(s.ActorVersion(), s.ActorKey())
	if !ok {
		panic(fmt.Errorf("didn't find actor %v code id for actor version %d", s.ActorKey(), s.ActorVersion()))
	}

	return code
}
