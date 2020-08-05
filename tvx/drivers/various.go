package drivers

import (
	"context"

	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/ipfs/go-cid"

	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
)

const (
	totalFilecoin     = 2_000_000_000
	filecoinPrecision = 1_000_000_000_000_000_000
)

var (
	TotalNetworkBalance = big_spec.Mul(big_spec.NewInt(totalFilecoin), big_spec.NewInt(filecoinPrecision))
	EmptyReturnValue    = []byte{}
)

// Actor is an abstraction over the actor states stored in the root of the state tree.
type Actor interface {
	Code() cid.Cid
	Head() cid.Cid
	CallSeqNum() uint64
	Balance() big.Int
}

type contextStore struct {
	cbor.IpldStore
	ctx context.Context
}

func (s *contextStore) Context() context.Context {
	return s.ctx
}
