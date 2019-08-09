package paych

import (
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/ipfs/go-datastore"
)

type PaychStore struct {
	ds datastore.Batching
}

func NewPaychStore(ds datastore.Batching) *PaychStore {
	return &PaychStore{
		ds: ds,
	}
}

func (ps *PaychStore) TrackChannel(ch address.Address) error {
	panic("nyi")
}

func (ps *PaychStore) AddVoucher(sv *types.SignedVoucher) error {
	panic("nyi")
}

func (ps *PaychStore) VouchersForPaych(addr address.Address) ([]*types.SignedVoucher, error) {
	panic("nyi")
}
