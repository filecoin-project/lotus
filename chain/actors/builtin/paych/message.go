package paych

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func Message(version actors.Version) MessageBuilder {
	switch version {
	case actors.Version0:
		return message0{}
	case actors.Version2:
		return message2{}
	default:
		panic(fmt.Sprintf("unsupported actors version: %d", version))
	}
}

type MessageBuilder interface {
	Create(from, to address.Address, initialAmount abi.TokenAmount) (*types.Message, error)
	Update(from, paych address.Address, voucher *SignedVoucher, secret []byte) (*types.Message, error)
	Settle(from, paych address.Address) (*types.Message, error)
	Collect(from, paych address.Address) (*types.Message, error)
}
