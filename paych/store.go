package paych

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	dsq "github.com/ipfs/go-datastore/query"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"
)

var ErrChannelNotTracked = errors.New("channel not tracked")

func init() {
	cbor.RegisterCborType(VoucherInfo{})
	cbor.RegisterCborType(ChannelInfo{})
}

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

	Direction int
	Vouchers  []*VoucherInfo
	NextLane  uint64
}

func dskeyForChannel(addr address.Address) datastore.Key {
	return datastore.NewKey(addr.String())
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
	if err == datastore.ErrNotFound {
		return nil, ErrChannelNotTracked
	}
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

		if err := cbor.DecodeInto(res.Value, &ci); err != nil {
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

func (ps *Store) AddVoucher(ch address.Address, sv *types.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	ps.lk.Lock()
	defer ps.lk.Unlock()

	ci, err := ps.getChannelInfo(ch)
	if err != nil {
		return types.NewInt(0), err
	}

	var bestAmount types.BigInt
	var bestNonce uint64 = math.MaxUint64

	// look for duplicates
	for i, v := range ci.Vouchers {
		if v.Voucher.Lane == sv.Lane && v.Voucher.Nonce+1 > bestNonce+1 {
			bestNonce = v.Voucher.Nonce
			bestAmount = v.Voucher.Amount
		}
		if !sv.Equals(v.Voucher) {
			continue
		}
		if v.Proof != nil {
			if !bytes.Equal(v.Proof, proof) {
				log.Warnf("AddVoucher: multiple proofs for single voucher, storing both")
				break
			}
			log.Warnf("AddVoucher: voucher re-added with matching proof")
			return types.NewInt(0), nil
		}

		log.Warnf("AddVoucher: adding proof to stored voucher")
		ci.Vouchers[i] = &VoucherInfo{
			Voucher: v.Voucher,
			Proof:   proof,
		}

		return types.NewInt(0), ps.putChannelInfo(ci)
	}

	if bestAmount == (types.BigInt{}) {
		bestAmount = types.NewInt(0)
	}

	delta := types.BigSub(sv.Amount, bestAmount)
	if minDelta.GreaterThan(delta) {
		return delta, xerrors.Errorf("addVoucher: supplied token amount too low; minD=%s, D=%s; bestAmt=%s; v.Amt=%s", minDelta, delta, bestAmount, sv.Amount)
	}

	ci.Vouchers = append(ci.Vouchers, &VoucherInfo{
		Voucher: sv,
		Proof:   proof,
	})

	if ci.NextLane <= sv.Lane {
		ci.NextLane = sv.Lane + 1
	}

	return delta, ps.putChannelInfo(ci)
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
