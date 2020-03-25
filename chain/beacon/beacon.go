package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"

	"github.com/minio/blake2b-simd"
)

var log = logging.Logger("beacon")

type Response struct {
	Entry *types.BeaconEntry
	Err   error
}

type DrandBeacon interface {
	RoundTime() time.Duration
	LastEntry() (*types.BeaconEntry, error)
	Entry(context.Context, uint64) <-chan Response
	VerifyEntry(*types.BeaconEntry) (bool, error)
	BeaconIndexesForEpoch(abi.ChainEpoch, int) []uint64
}

func ValidateBlockValues(b DrandBeacon, h *types.BlockHeader, nulls int) error {
	indexes := b.BeaconIndexesForEpoch(h.Height, nulls)

	if len(h.BeaconEntries) != len(indexes) {
		return xerrors.Errorf("incorrect number of beacon entries, exp:%d got:%d", len(indexes), len(h.BeaconEntries))
	}

	for i, ix := range indexes {
		if h.BeaconEntries[i].Index != ix {
			return xerrors.Errorf("beacon entry at [%d] had wrong index, exp:%d got:%d", i, ix, h.BeaconEntries[i].Index)
		}
		if ok, err := b.VerifyEntry(h.BeaconEntries[i]); err != nil {
			return xerrors.Errorf("failed to verify beacon entry %d: %w", i, err)
		} else if !ok {
			return xerrors.Errorf("beacon entry %d was invalid", i)
		}
	}

	return nil
}

func BeaconEntriesForBlock(ctx context.Context, beacon DrandBeacon, round abi.ChainEpoch, nulls int) ([]*types.BeaconEntry, error) {
	start := time.Now()

	var out []*types.BeaconEntry
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

func (mb *mockBeacon) LastEntry() (*types.BeaconEntry, error) {
	panic("NYI")
}

func (mb *mockBeacon) entryForIndex(index uint64) *types.BeaconEntry {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, index)
	rval := blake2b.Sum256(buf)
	return &types.BeaconEntry{
		Index:     index,
		Signature: crypto.Signature{Type: crypto.SigTypeBLS, Data: rval[:]},
	}
}

func (mb *mockBeacon) Entry(ctx context.Context, index uint64) <-chan Response {
	e := mb.entryForIndex(index)
	out := make(chan Response, 1)
	out <- Response{Entry: e}
	return out
}

func (mb *mockBeacon) VerifyEntry(e *types.BeaconEntry) (bool, error) {
	oe := mb.entryForIndex(e.Index)
	return bytes.Equal(e.Signature.Data, oe.Signature.Data), nil
}

func (mb *mockBeacon) BeaconIndexesForEpoch(epoch abi.ChainEpoch, nulls int) []uint64 {
	var out []uint64
	for i := nulls; i > 0; i-- {
		out = append(out, uint64(epoch)-uint64(i))
	}
	out = append(out, uint64(epoch))
	return []uint64{uint64(epoch)}
}

var _ DrandBeacon = (*mockBeacon)(nil)
