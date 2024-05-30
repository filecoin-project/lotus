package sealing

import (
	"bytes"
	"context"
	"encoding/binary"
	"regexp"

	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
)

var StorageCounterDSPrefix = "/storage/nextid"
var SectorBitfieldsDSPrefix = "/storage/sectorsids/"
var SectorReservationsDSPrefix = "/storage/sectorsids/reserved/"

var allocatedSectorsKey = datastore.NewKey(SectorBitfieldsDSPrefix + "allocated")
var reservedSectorsKey = datastore.NewKey(SectorBitfieldsDSPrefix + "reserved")

func (m *Sealing) loadBitField(ctx context.Context, name datastore.Key) (*bitfield.BitField, error) {
	raw, err := m.ds.Get(ctx, name)
	if err == datastore.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var bf bitfield.BitField

	if err := bf.UnmarshalCBOR(bytes.NewBuffer(raw)); err != nil {
		return nil, err
	}
	return &bf, nil
}

func (m *Sealing) saveBitField(ctx context.Context, name datastore.Key, bf bitfield.BitField) error {
	var bb bytes.Buffer
	err := bf.MarshalCBOR(&bb)
	if err != nil {
		return err
	}
	return m.ds.Put(ctx, name, bb.Bytes())
}

func (m *Sealing) NextSectorNumber(ctx context.Context) (abi.SectorNumber, error) {
	m.sclk.Lock()
	defer m.sclk.Unlock()

	am, err := m.numAssignerMetaLocked(ctx)
	if err != nil {
		return 0, err
	}

	allocated := am.Allocated

	allocated.Set(uint64(am.Next))

	if err := m.saveBitField(ctx, allocatedSectorsKey, allocated); err != nil {
		return 0, err
	}

	// save legacy counter so that in case of a miner downgrade things keep working
	{
		buf := make([]byte, binary.MaxVarintLen64)
		size := binary.PutUvarint(buf, uint64(am.Next))

		if err := m.ds.Put(ctx, datastore.NewKey(StorageCounterDSPrefix), buf[:size]); err != nil {
			return 0, err
		}
	}

	return am.Next, nil
}

func (m *Sealing) NumAssignerMeta(ctx context.Context) (api.NumAssignerMeta, error) {
	m.sclk.Lock()
	defer m.sclk.Unlock()

	return m.numAssignerMetaLocked(ctx)
}

func (m *Sealing) numAssignerMetaLocked(ctx context.Context) (api.NumAssignerMeta, error) {
	// load user-reserved and allocated bitfields
	reserved, err := m.loadBitField(ctx, reservedSectorsKey)
	if err != nil {
		return api.NumAssignerMeta{}, xerrors.Errorf("loading reserved sectors bitfield: %w", err)
	}
	allocated, err := m.loadBitField(ctx, allocatedSectorsKey)
	if err != nil {
		return api.NumAssignerMeta{}, xerrors.Errorf("loading allocated sectors bitfield: %w", err)
	}

	// if the allocated bitfield doesn't exist, create it from the legacy sector counter
	if allocated == nil {
		var i uint64
		{
			curBytes, err := m.ds.Get(ctx, datastore.NewKey(StorageCounterDSPrefix))
			if err != nil && err != datastore.ErrNotFound {
				return api.NumAssignerMeta{}, err
			}
			if err == nil {
				cur, _ := binary.Uvarint(curBytes)
				i = cur + 1
			}
		}

		rl := &rlepluslazy.RunSliceIterator{Runs: []rlepluslazy.Run{
			{
				Val: true,
				Len: i,
			},
		}}

		bf, err := bitfield.NewFromIter(rl)
		if err != nil {
			return api.NumAssignerMeta{}, xerrors.Errorf("bitfield from iter: %w", err)
		}
		allocated = &bf
	}

	// if there are no user reservations, use an empty bitfield
	if reserved == nil {
		emptyBf := bitfield.New()
		reserved = &emptyBf
	}

	// todo union with miner allocated nums
	inuse, err := bitfield.MergeBitFields(*reserved, *allocated)
	if err != nil {
		return api.NumAssignerMeta{}, xerrors.Errorf("merge reserved/allocated: %w", err)
	}

	stateAllocated, err := m.Api.StateMinerAllocated(ctx, m.maddr, types.EmptyTSK)
	if err != nil || stateAllocated == nil {
		return api.NumAssignerMeta{}, xerrors.Errorf("getting state-allocated sector numbers: %w", err)
	}
	inuse, err = bitfield.MergeBitFields(inuse, *stateAllocated)
	if err != nil {
		return api.NumAssignerMeta{}, xerrors.Errorf("merge inuse/stateAllocated: %w", err)
	}

	// find first available sector number
	iri, err := inuse.RunIterator()
	if err != nil {
		return api.NumAssignerMeta{}, xerrors.Errorf("inuse run iter: %w", err)
	}

	var firstFree abi.SectorNumber
	for iri.HasNext() {
		r, err := iri.NextRun()
		if err != nil {
			return api.NumAssignerMeta{}, xerrors.Errorf("next run: %w", err)
		}
		if !r.Val {
			break
		}
		firstFree += abi.SectorNumber(r.Len)
	}

	return api.NumAssignerMeta{
		Reserved:  *reserved,
		Allocated: *allocated,
		InUse:     inuse,
		Next:      firstFree,
	}, nil
}

// NumReservations returns a list of named sector reservations
func (m *Sealing) NumReservations(ctx context.Context) (map[string]bitfield.BitField, error) {
	res, err := m.ds.Query(ctx, dsq.Query{Prefix: SectorReservationsDSPrefix})
	if err != nil {
		return nil, xerrors.Errorf("query reservations: %w", err)
	}
	defer res.Close() //nolint:errcheck

	out := map[string]bitfield.BitField{}

	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}

		if res.Error != nil {
			return nil, xerrors.Errorf("res error: %w", err)
		}

		b := bitfield.New()
		if err := b.UnmarshalCBOR(bytes.NewReader(res.Value)); err != nil {
			return nil, err
		}

		out[datastore.NewKey(res.Key).BaseNamespace()] = b
	}

	return out, nil
}

// NumReserve creates a new sector reservation
func (m *Sealing) NumReserve(ctx context.Context, name string, reserving bitfield.BitField, force bool) error {
	m.sclk.Lock()
	defer m.sclk.Unlock()

	return m.numReserveLocked(ctx, name, reserving, force)
}

// numReserveLocked creates a new sector reservation
func (m *Sealing) numReserveLocked(ctx context.Context, name string, reserving bitfield.BitField, force bool) error {
	rk, err := reservationKey(name)
	if err != nil {
		return err
	}

	// check if there's an existing reservation with this name
	cur := bitfield.New()

	curRes, err := m.ds.Get(ctx, rk)
	if err == nil {
		log.Warnw("reservation with this name already exists", "name", name)
		if !force {
			return xerrors.Errorf("reservation with this name already exists")
		}

		if err := cur.UnmarshalCBOR(bytes.NewReader(curRes)); err != nil {
			return xerrors.Errorf("unmarshaling existing reservation: %w", err)
		}
	} else if err != datastore.ErrNotFound {
		return xerrors.Errorf("checking if reservation exists: %w", err)
	}

	// load the global reserved bitfield and subtract current reservation if we're overwriting

	nm, err := m.numAssignerMetaLocked(ctx)
	if err != nil {
		return err
	}
	allReserved := nm.Reserved

	allReserved, err = bitfield.SubtractBitField(allReserved, cur)
	if err != nil {
		return xerrors.Errorf("allReserved - cur: %w", err)
	}

	// check if the reservation is colliding with any other reservation
	colliding, err := bitfield.IntersectBitField(allReserved, reserving)
	if err != nil {
		return xerrors.Errorf("intersect all / reserving: %w", err)
	}

	empty, err := colliding.IsEmpty()
	if err != nil {
		return xerrors.Errorf("colliding.empty: %w", err)
	}
	if !empty {
		log.Warnw("new reservation is colliding with another existing reservation", "name", name)
		if !force {
			return xerrors.Errorf("new reservation is colliding with another existing reservation")
		}
	}

	// check if the reservation is colliding with allocated sectors
	colliding, err = bitfield.IntersectBitField(nm.Allocated, reserving)
	if err != nil {
		return xerrors.Errorf("intersect all / reserving: %w", err)
	}

	empty, err = colliding.IsEmpty()
	if err != nil {
		return xerrors.Errorf("colliding.empty: %w", err)
	}
	if !empty {
		log.Warnw("new reservation is colliding with allocated sector numbers", "name", name)
		if !force {
			return xerrors.Errorf("new reservation is colliding with allocated sector numbers")
		}
	}

	// write the reservation
	newReserved, err := bitfield.MergeBitFields(allReserved, reserving)
	if err != nil {
		return xerrors.Errorf("merge allReserved+reserving: %w", err)
	}

	if err := m.saveBitField(ctx, rk, reserving); err != nil {
		return xerrors.Errorf("saving reservation: %w", err)
	}

	if err := m.saveBitField(ctx, reservedSectorsKey, newReserved); err != nil {
		return xerrors.Errorf("save reserved nums: %w", err)
	}

	return nil
}

func (m *Sealing) NumReserveCount(ctx context.Context, name string, count uint64) (bitfield.BitField, error) {
	m.sclk.Lock()
	defer m.sclk.Unlock()

	nm, err := m.numAssignerMetaLocked(ctx)
	if err != nil {
		return bitfield.BitField{}, err
	}

	// figure out `count` unused sectors at lowest possible numbers

	usedCount, err := nm.InUse.Count()
	if err != nil {
		return bitfield.BitField{}, err
	}

	// get a bitfield mask which has at least `count` bits more set than the nm.InUse field
	mask, err := bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{Runs: []rlepluslazy.Run{
		{
			Val: true,
			Len: count + usedCount,
		},
	}})
	if err != nil {
		return bitfield.BitField{}, err
	}

	free, err := bitfield.SubtractBitField(mask, nm.InUse)
	if err != nil {
		return bitfield.BitField{}, err
	}

	// free now has at least 'count' bits set - it's possible that InUse had some bits set outside the count+usedCount range

	free, err = free.Slice(0, count)
	if err != nil {
		return bitfield.BitField{}, err
	}

	if err := m.numReserveLocked(ctx, name, free, false); err != nil {
		return bitfield.BitField{}, err
	}

	return free, nil
}

// NumFree removes a named sector reservation
func (m *Sealing) NumFree(ctx context.Context, name string) error {
	rk, err := reservationKey(name)
	if err != nil {
		return err
	}

	res, err := m.loadBitField(ctx, rk)
	if err != nil {
		return xerrors.Errorf("loading reservation: %w", err)
	}
	if res == nil {
		return xerrors.Errorf("reservation with this name doesn't exist")
	}

	allRes, err := m.loadBitField(ctx, reservedSectorsKey)
	if err != nil {
		return xerrors.Errorf("leading all reservations: %w", err)
	}
	if allRes == nil {
		return xerrors.Errorf("all reservations bitfield not found")
	}

	newAll, err := bitfield.SubtractBitField(*allRes, *res)
	if err != nil {
		return xerrors.Errorf("allRes-res: %w", err)
	}

	if err := m.saveBitField(ctx, reservedSectorsKey, newAll); err != nil {
		return xerrors.Errorf("saving reservations bitfield: %w", err)
	}

	if err := m.ds.Delete(ctx, rk); err != nil {
		return xerrors.Errorf("deleting reservation: %w", err)
	}

	return nil
}

func reservationKey(name string) (datastore.Key, error) {
	ok, err := regexp.Match("^[a-zA-Z0-9_\\-]+$", []byte(name))
	if err != nil {
		return datastore.Key{}, err
	}
	if !ok {
		return datastore.Key{}, xerrors.Errorf("reservation name contained disallowed characters (allowed: a-z, A-Z, 0-9, '-', '_')")
	}

	return datastore.KeyWithNamespaces([]string{SectorReservationsDSPrefix, name}), nil
}
