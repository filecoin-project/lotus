package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/minio/blake2b-simd"
	"golang.org/x/xerrors"
)

// Mock beacon assumes that filecoin rounds are 1:1 mapped with the beacon rounds
type mockBeacon struct {
	interval time.Duration
}

func NewMockBeacon(interval time.Duration) RandomBeacon {
	mb := &mockBeacon{interval: interval}

	return mb
}

func (mb *mockBeacon) RoundTime() time.Duration {
	return mb.interval
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

func (mb *mockBeacon) MaxBeaconRoundForEpoch(epoch abi.ChainEpoch) uint64 {
	// offset for better testing
	return uint64(epoch + 100)
}

var _ RandomBeacon = (*mockBeacon)(nil)
