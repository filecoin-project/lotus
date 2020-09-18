package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	verifreg0 "github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	adt0 "github.com/filecoin-project/specs-actors/actors/util/adt"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

type state0 struct {
	verifreg0.State
	store adt.Store
}

func (s *state0) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	if addr.Protocol() != address.ID {
		return false, big.Zero(), xerrors.Errorf("can only look up ID addresses")
	}

	vh, err := adt0.AsMap(s.store, s.VerifiedClients)
	if err != nil {
		return false, big.Zero(), xerrors.Errorf("loading verified clients: %w", err)
	}

	var dcap abi.StoragePower
	if found, err := vh.Get(abi.AddrKey(addr), &dcap); err != nil {
		return false, big.Zero(), xerrors.Errorf("looking up verified clients: %w", err)
	} else if !found {
		return false, big.Zero(), nil
	}

	return true, dcap, nil
}
