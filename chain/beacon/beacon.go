package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/minio/blake2b-simd"
)

var log = logging.Logger("beacon")

type Response struct {
	Entry types.BeaconEntry
	Err   error
}

type DrandBeacon interface {
	RoundTime() time.Duration
	LastEntry() (types.BeaconEntry, error)
	Entry(context.Context, uint64) <-chan Response
	VerifyEntry(types.BeaconEntry) (bool, error)
	BeaconIndexesForEpoch(abi.ChainEpoch, int) []uint64
	IsEntryForEpoch(e types.BeaconEntry, epoch abi.ChainEpoch, nulls int) (bool, error)
}

func ValidateBlockValues(b DrandBeacon, h *types.BlockHeader, nulls int) error {
	for i, be := range h.BeaconEntries {
		if ok, err := b.IsEntryForEpoch(be, h.Height, nulls); err != nil {
			return xerrors.Errorf("failed to check if beacon belongs: %w")
		} else if !ok {
			return xerrors.Errorf("beacon does not belong in this block: %d", i)
		}

		if ok, err := b.VerifyEntry(be); err != nil {
			return xerrors.Errorf("failed to verify beacon entry %d: %w", i, err)
		} else if !ok {
			return xerrors.Errorf("beacon entry %d was invalid", i)
		}
	}

	// validate that block contains entry for its own epoch
	should := b.BeaconIndexesForEpoch(h.Height, 0)
	if should[len(should)-1] != h.BeaconEntries[len(h.BeaconEntries)-1].Index {
		return xerrors.Errorf("missing beacon entry for this block")
	}

	return nil
}

func BeaconEntriesForBlock(ctx context.Context, beacon DrandBeacon, round abi.ChainEpoch, nulls int) ([]types.BeaconEntry, error) {
	start := time.Now()

	var out []types.BeaconEntry
	for _, ei := range beacon.BeaconIndexesForEpoch(round, nulls) {
		rch := beacon.Entry(ctx, ei)
		select {
		case resp := <-rch:
			if resp.Err != nil {
				return nil, xerrors.Errorf("beacon entry request returned error: %w", resp.Err)
			}

			out = append(out, resp.Entry)
		case <-ctx.Done():
			return nil, xerrors.Errorf("context timed out waiting on beacon entry to come back for round %d: %w", round, ctx.Err())
		}
	}

	log.Debugw("fetching beacon entries", "took", time.Since(start), "numEntries", len(out))
	return out, nil
}

type mockBeacon struct {
	interval time.Duration
}

func NewMockBeacon(interval time.Duration) *mockBeacon {
	mb := &mockBeacon{interval: interval}

	return mb
}

func (mb *mockBeacon) RoundTime() time.Duration {
	return mb.interval
}

func (mb *mockBeacon) LastEntry() (types.BeaconEntry, error) {
	panic("NYI")
}

func (mb *mockBeacon) entryForIndex(index uint64) types.BeaconEntry {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, index)
	rval := blake2b.Sum256(buf)
	return types.BeaconEntry{
		Index: index,
		Data:  rval[:],
	}
}

func (mb *mockBeacon) Entry(ctx context.Context, index uint64) <-chan Response {
	e := mb.entryForIndex(index)
	out := make(chan Response, 1)
	out <- Response{Entry: e}
	return out
}

func (mb *mockBeacon) VerifyEntry(e types.BeaconEntry) (bool, error) {
	// TODO: cache this, especially for bls
	oe := mb.entryForIndex(e.Index)
	return bytes.Equal(e.Data, oe.Data), nil
}

func (mb *mockBeacon) IsEntryForEpoch(e types.BeaconEntry, epoch abi.ChainEpoch, nulls int) (bool, error) {
	return int64(e.Index) <= int64(epoch) && int64(epoch)-int64(nulls) >= int64(e.Index), nil
}

func (mb *mockBeacon) BeaconIndexesForEpoch(epoch abi.ChainEpoch, nulls int) []uint64 {
	var out []uint64
	for i := nulls; i > 0; i-- {
		out = append(out, uint64(epoch)-uint64(i))
	}
	out = append(out, uint64(epoch))
	return out
}

var _ DrandBeacon = (*mockBeacon)(nil)
