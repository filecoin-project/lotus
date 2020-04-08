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
	//RoundTime() uint64
	//StartTime() uint64
	LastEntry() (types.BeaconEntry, error)
	Entry(context.Context, uint64) <-chan Response
	VerifyEntry(types.BeaconEntry, types.BeaconEntry) error
	MaxBeaconRoundForEpoch(abi.ChainEpoch, types.BeaconEntry) uint64
	IsEntryForEpoch(e types.BeaconEntry, epoch abi.ChainEpoch, nulls int) (bool, error)
}

func ValidateBlockValues(b DrandBeacon, h *types.BlockHeader, prevEntry types.BeaconEntry) error {
	maxRound := b.MaxBeaconRoundForEpoch(h.Height, prevEntry)
	if maxRound == prevEntry.Round {
		if len(h.BeaconEntries) != 0 {
			return xerrors.Errorf("expected not to have any beacon entries in this block, got %d", len(h.BeaconEntries))
		}
		return nil
	}

	last := h.BeaconEntries[len(h.BeaconEntries)-1]
	if last.Round != maxRound {
		return xerrors.Errorf("expected final beacon entry in block to be at round %d, got %d", maxRound, last.Round)
	}

	for i, e := range h.BeaconEntries {
		if err := b.VerifyEntry(e, prevEntry); err != nil {
			return xerrors.Errorf("beacon entry %d was invalid: %w", i, err)
		}
		prevEntry = e
	}

	return nil
}

func BeaconEntriesForBlock(ctx context.Context, beacon DrandBeacon, round abi.ChainEpoch, prev types.BeaconEntry) ([]types.BeaconEntry, error) {
	start := time.Now()

	maxRound := beacon.MaxBeaconRoundForEpoch(round, prev)
	if maxRound == prev.Round {
		return nil, nil
	}

	cur := maxRound
	var out []types.BeaconEntry
	for cur > prev.Round {
		rch := beacon.Entry(ctx, cur)
		select {
		case resp := <-rch:
			if resp.Err != nil {
				return nil, xerrors.Errorf("beacon entry request returned error: %w", resp.Err)
			}

			out = append(out, resp.Entry)
			cur = resp.Entry.Round - 1
		case <-ctx.Done():
			return nil, xerrors.Errorf("context timed out waiting on beacon entry to come back for round %d: %w", round, ctx.Err())
		}
	}

	log.Debugw("fetching beacon entries", "took", time.Since(start), "numEntries", len(out))
	reverse(out)
	return out, nil
}

func reverse(arr []types.BeaconEntry) {
	for i := 0; i < len(arr)/2; i++ {
		arr[i], arr[len(arr)-(1+i)] = arr[len(arr)-(1+i)], arr[i]
	}
}

// Mock beacon assumes that filecoin rounds are 1:1 mapped with the beacon rounds
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
		Round: index,
		Data:  rval[:],
	}
}

func (mb *mockBeacon) Entry(ctx context.Context, index uint64) <-chan Response {
	e := mb.entryForIndex(index)
	out := make(chan Response, 1)
	out <- Response{Entry: e}
	return out
}

func (mb *mockBeacon) VerifyEntry(from types.BeaconEntry, to types.BeaconEntry) error {
	// TODO: cache this, especially for bls
	oe := mb.entryForIndex(from.Round)
	if !bytes.Equal(from.Data, oe.Data) {
		return xerrors.Errorf("mock beacon entry was invalid!")
	}
	return nil
}

func (mb *mockBeacon) IsEntryForEpoch(e types.BeaconEntry, epoch abi.ChainEpoch, nulls int) (bool, error) {
	return int64(e.Round) <= int64(epoch) && int64(epoch)-int64(nulls) >= int64(e.Round), nil
}

func (mb *mockBeacon) MaxBeaconRoundForEpoch(epoch abi.ChainEpoch, prevEntry types.BeaconEntry) uint64 {
	return uint64(epoch)
}

var _ DrandBeacon = (*mockBeacon)(nil)
