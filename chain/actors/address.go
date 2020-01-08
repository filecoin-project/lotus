package actors

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
)

func NewIDAddress(id uint64) (address.Address, ActorError) {
	a, err := address.NewIDAddress(id)
	if err != nil {
		return address.Undef, aerrors.Escalate(err, "could not create ID Address")
	}
	return a, nil
}
