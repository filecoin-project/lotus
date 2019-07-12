package types

import (
	"math/big"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/ipfs/go-hamt-ipld"
)

type VMContext interface {
	Message() *Message
	Ipld() *hamt.CborIpldStore
	Send(to address.Address, method string, value *big.Int, params []interface{}) ([][]byte, uint8, error)
	BlockHeight() uint64
	GasUsed() BigInt
}
