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
	BeaconRoundsForEpoch(abi.ChainEpoch, types.BeaconEntry) []uint64
	IsEntryForEpoch(e types.BeaconEntry, epoch abi.ChainEpoch, nulls int) (bool, error)
}

func ValidateBlockValues(b DrandBeacon, h *types.BlockHeader, prevEntry types.BeaconEntry) error {
	rounds := b.BeaconRoundsForEpoch(h.Height, prevEntry)
	if len(rounds) != len(h.BeaconEntries) {
		return xerrors.Errorf("mismatch in number of expected beacon entries (exp %d, got %d)", len(rounds), len(h.BeaconEntries))
	}

	for i, e := range h.BeaconEntries {
		if e.Round != rounds[i] {
			return xerrors.Errorf("entry at index %d did not match expected round (exp %d, got %d)", i, rounds[i], e.Round)
		}

		if err := b.VerifyEntry(e, prevEntry); err != nil {
			return xerrors.Errorf("beacon entry %d was invalid: %w", i, err)
		}
		prevEntry = e
	}

	return nil
}

func BeaconEntriesForBlock(ctx context.Context, beacon DrandBeacon, round abi.ChainEpoch, prev types.BeaconEntry) ([]types.BeaconEntry, error) {
	start := time.Now()

	var out []types.BeaconEntry
	for _, ei := range beacon.BeaconRoundsForEpoch(round, prev) {
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

func (mb *mockBeacon) BeaconRoundsForEpoch(epoch abi.ChainEpoch, prevEntry types.BeaconEntry) []uint64 {
	var out []uint64
	for i := prevEntry.Round + 1; i < uint64(epoch); i++ {
		out = append(out, i)
	}
	return out
}

var _ DrandBeacon = (*mockBeacon)(nil)
