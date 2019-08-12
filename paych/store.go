package paych

import (
	"fmt"
	"strings"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"

	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(ChannelInfo{})
}

type Store struct {
	ds datastore.Batching
}

func NewStore(ds datastore.Batching) *Store {
	return &Store{
		ds: ds,
	}
}

const (
	DirInbound  = 1
	DirOutbound = 2
)

type ChannelInfo struct {
	Channel     address.Address
	ControlAddr address.Address
	Direction   int
	Vouchers    []*types.SignedVoucher
}

func dskeyForChannel(addr address.Address) datastore.Key {
	return datastore.NewKey("/paych/" + addr.String())
}

func (ps *Store) putChannelInfo(ci *ChannelInfo) error {
	k := dskeyForChannel(ci.Channel)

	b, err := cbor.DumpObject(ci)
	if err != nil {
		return err
	}

	return ps.ds.Put(k, b)
}

func (ps *Store) getChannelInfo(addr address.Address) (*ChannelInfo, error) {
	k := dskeyForChannel(addr)

	b, err := ps.ds.Get(k)
	if err != nil {
		return nil, err
	}

	var ci ChannelInfo
	if err := cbor.DecodeInto(b, &ci); err != nil {
		return nil, err
	}

	return &ci, nil
}

func (ps *Store) TrackChannel(ch *ChannelInfo) error {
	_, err := ps.getChannelInfo(ch.Channel)
	switch err {
	default:
		return err
	case nil:
		return fmt.Errorf("already tracking channel: %s", ch.Channel)
	case datastore.ErrNotFound:
		return ps.putChannelInfo(ch)
	}
}

func (ps *Store) ListChannels() ([]address.Address, error) {
	res, err := ps.ds.Query(dsq.Query{Prefix: "/paych/", KeysOnly: true})
	if err != nil {
		return nil, err
	}

	var out []address.Address
	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}

		addr, err := address.NewFromString(strings.TrimPrefix(res.Key, "/paych/"))
		if err != nil {
			return nil, err
		}

		out = append(out, addr)
	}

	return out, nil
}

func (ps *Store) AddVoucher(ch address.Address, sv *types.SignedVoucher) error {
	panic("nyi")
}

func (ps *Store) VouchersForPaych(addr address.Address) ([]*types.SignedVoucher, error) {
	panic("nyi")
}
