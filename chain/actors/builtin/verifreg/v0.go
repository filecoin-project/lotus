package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state0)(nil)

type state0 struct {
	verifreg0.State
	store adt.Store
}

func getDataCap(store adt.Store, root cid.Cid, addr address.Address) (bool, abi.StoragePower, error) {
	if addr.Protocol() != address.ID {
		return false, big.Zero(), xerrors.Errorf("can only look up ID addresses")
	}

	vh, err := adt0.AsMap(store, root)
	if err != nil {
		return false, big.Zero(), xerrors.Errorf("loading verifreg: %w", err)
	}

	var dcap abi.StoragePower
	if found, err := vh.Get(abi.AddrKey(addr), &dcap); err != nil {
		return false, big.Zero(), xerrors.Errorf("looking up addr: %w", err)
	} else if !found {
		return false, big.Zero(), nil
	}

	return true, dcap, nil
}

func (s *state0) RootKey() (address.Address, error) {
	return s.State.RootKey, nil
}

func (s *state0) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, s.State.VerifiedClients, addr)
}

func (s *state0) VerifierDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	return getDataCap(s.store, s.State.Verifiers, addr)
}

func forEachCap(store adt.Store, root cid.Cid, cb func(addr address.Address, dcap abi.StoragePower) error) error {
	vh, err := adt0.AsMap(store, root)
	if err != nil {
		return xerrors.Errorf("loading verified clients: %w", err)
	}
	var dcap abi.StoragePower
	return vh.ForEach(&dcap, func(key string) error {
		a, err := address.NewFromBytes([]byte(key))
		if err != nil {
			return err
		}
		return cb(a, dcap)
	})
}

func (s *state0) ForEachVerifier(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, s.State.Verifiers, cb)
}

func (s *state0) ForEachClient(cb func(addr address.Address, dcap abi.StoragePower) error) error {
	return forEachCap(s.store, s.State.VerifiedClients, cb)
}
