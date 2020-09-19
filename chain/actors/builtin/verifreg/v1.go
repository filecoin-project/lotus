package verifreg

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	adt1 "github.com/filecoin-project/specs-actors/actors/util/adt"
	verifreg1 "github.com/filecoin-project/specs-actors/v2/actors/builtin/verifreg"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/adt"
)

var _ State = (*state1)(nil)

type state1 struct {
	verifreg1.State
	store adt.Store
}

func (s *state1) VerifiedClientDataCap(addr address.Address) (bool, abi.StoragePower, error) {
	if addr.Protocol() != address.ID {
		return false, big.Zero(), xerrors.Errorf("can only look up ID addresses")
	}

	vh, err := adt1.AsMap(s.store, s.VerifiedClients)
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
