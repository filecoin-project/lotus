package verifreg

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/v7/actors/builtin/verifreg"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
)

// taking this as a function instead of asking the caller to call it helps reduce some of the error
// checking boilerplate.
//
// "go made me do it"
type rootFunc func() (adt.Map, error)

// Assumes that the bitwidth for v3 HAMTs is the DefaultHamtBitwidth
func getDataCap(store adt.Store, ver actors.Version, root rootFunc, addr address.Address) (bool, abi.StoragePower, error) {
	if addr.Protocol() != address.ID {
		return false, big.Zero(), xerrors.Errorf("can only look up ID addresses")
	}
	vh, err := root()
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

// Assumes that the bitwidth for v3 HAMTs is the DefaultHamtBitwidth
func forEachCap(store adt.Store, ver actors.Version, root rootFunc, cb func(addr address.Address, dcap abi.StoragePower) error) error {
	vh, err := root()
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

func getRemoveDataCapProposalID(store adt.Store, ver actors.Version, root rootFunc, verifier address.Address, client address.Address) (bool, uint64, error) {
	if verifier.Protocol() != address.ID {
		return false, 0, xerrors.Errorf("can only look up ID addresses")
	}
	if client.Protocol() != address.ID {
		return false, 0, xerrors.Errorf("can only look up ID addresses")
	}
	vh, err := root()
	if err != nil {
		return false, 0, xerrors.Errorf("loading verifreg: %w", err)
	}
	if vh == nil {
		return false, 0, xerrors.Errorf("remove data cap proposal hamt not found. you are probably using an incompatible version of actors")
	}

	var id verifreg.RmDcProposalID
	if found, err := vh.Get(abi.NewAddrPairKey(verifier, client), &id); err != nil {
		return false, 0, xerrors.Errorf("looking up addr pair: %w", err)
	} else if !found {
		return false, 0, nil
	}

	return true, id.ProposalID, nil
}
