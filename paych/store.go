package paych

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dsq "github.com/ipfs/go-datastore/query"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborrpc "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var ErrChannelNotTracked = errors.New("channel not tracked")

type Store struct {
	lk sync.Mutex // TODO: this can be split per paych

	ds datastore.Batching
}

func NewStore(ds dtypes.MetadataDS) *Store {
	ds = namespace.Wrap(ds, datastore.NewKey("/paych/"))
	return &Store{
		ds: ds,
	}
}

const (
	DirInbound  = 1
	DirOutbound = 2
)

type VoucherInfo struct {
	Voucher *types.SignedVoucher
	Proof   []byte
}

type ChannelInfo struct {
	Channel address.Address
	Control address.Address
	Target  address.Address

	Direction uint64
	Vouchers  []*VoucherInfo
	NextLane  uint64
}

func dskeyForChannel(addr address.Address) datastore.Key {
	return datastore.NewKey(addr.String())
}

func (ps *Store) putChannelInfo(ci *ChannelInfo) error {
	k := dskeyForChannel(ci.Channel)

	b, err := cborrpc.Dump(ci)
	if err != nil {
		return err
	}

	return ps.ds.Put(k, b)
}

func (ps *Store) getChannelInfo(addr address.Address) (*ChannelInfo, error) {
	k := dskeyForChannel(addr)

	b, err := ps.ds.Get(k)
	if err == datastore.ErrNotFound {
		return nil, ErrChannelNotTracked
	}
	if err != nil {
		return nil, err
	}

	var ci ChannelInfo
	if err := ci.UnmarshalCBOR(bytes.NewReader(b)); err != nil {
		return nil, err
	}

	return &ci, nil
}

func (ps *Store) TrackChannel(ch *ChannelInfo) error {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	return ps.trackChannel(ch)
}

func (ps *Store) trackChannel(ch *ChannelInfo) error {
	_, err := ps.getChannelInfo(ch.Channel)
	switch err {
	default:
		return err
	case nil:
		return fmt.Errorf("already tracking channel: %s", ch.Channel)
	case ErrChannelNotTracked:
		return ps.putChannelInfo(ch)
	}
}

func (ps *Store) ListChannels() ([]address.Address, error) {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	res, err := ps.ds.Query(dsq.Query{KeysOnly: true})
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var out []address.Address
	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}

		if res.Error != nil {
			return nil, err
		}

		addr, err := address.NewFromString(strings.TrimPrefix(res.Key, "/"))
		if err != nil {
			return nil, xerrors.Errorf("failed reading paych key (%q) from datastore: %w", res.Key, err)
		}

		out = append(out, addr)
	}

	return out, nil
}

func (ps *Store) findChan(filter func(*ChannelInfo) bool) (address.Address, error) {
	res, err := ps.ds.Query(dsq.Query{})
	if err != nil {
		return address.Undef, err
	}
	defer res.Close()

	var ci ChannelInfo

	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}

		if res.Error != nil {
			return address.Undef, err
		}

		if err := ci.UnmarshalCBOR(bytes.NewReader(res.Value)); err != nil {
			return address.Undef, err
		}

		if !filter(&ci) {
			continue
		}

		addr, err := address.NewFromString(strings.TrimPrefix(res.Key, "/"))
		if err != nil {
			return address.Undef, xerrors.Errorf("failed reading paych key (%q) from datastore: %w", res.Key, err)
		}

		return addr, nil
	}

	return address.Undef, nil
}

func (ps *Store) AllocateLane(ch address.Address) (uint64, error) {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	ci, err := ps.getChannelInfo(ch)
	if err != nil {
		return 0, err
	}

	out := ci.NextLane
	ci.NextLane++

	return out, ps.putChannelInfo(ci)
}

func (ps *Store) VouchersForPaych(ch address.Address) ([]*VoucherInfo, error) {
	ci, err := ps.getChannelInfo(ch)
	if err != nil {
		return nil, err
	}

	return ci.Vouchers, nil
}
