package beacon

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"time"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/types"
)

// MockBeacon assumes that filecoin rounds are 1:1 mapped with the beacon rounds
type MockBeacon struct {
	interval     time.Duration
	maxIndex     int
	waitingEntry int
	lk           sync.Mutex
	cond         *sync.Cond
}

func (mb *MockBeacon) IsChained() bool {
	return true
}

func NewMockBeacon(interval time.Duration) RandomBeacon {
	mb := &MockBeacon{interval: interval, maxIndex: -1}
	mb.cond = sync.NewCond(&mb.lk)
	return mb
}

// SetMaxIndex sets the maximum index that the beacon will return, and optionally blocks until all
// waiting requests are satisfied. If maxIndex is -1, the beacon will return entries indefinitely.
func (mb *MockBeacon) SetMaxIndex(maxIndex int, blockTillNoneWaiting bool) {
	mb.lk.Lock()
	defer mb.lk.Unlock()
	mb.maxIndex = maxIndex
	mb.cond.Broadcast()
	if !blockTillNoneWaiting {
		return
	}

	for mb.waitingEntry > 0 {
		mb.cond.Wait()
	}
}

// WaitingOnEntryCount returns the number of requests that are currently waiting for an entry. Where
// maxIndex has not been set, this will always return 0 as beacon entries are generated on demand.
func (mb *MockBeacon) WaitingOnEntryCount() int {
	mb.lk.Lock()
	defer mb.lk.Unlock()
	return mb.waitingEntry
}

func (mb *MockBeacon) RoundTime() time.Duration {
	return mb.interval
}

func (mb *MockBeacon) entryForIndex(index uint64) types.BeaconEntry {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, index)
	rval := blake2b.Sum256(buf)
	return types.BeaconEntry{
		Round: index,
		Data:  rval[:],
	}
}

func (mb *MockBeacon) Entry(ctx context.Context, index uint64) <-chan Response {
	out := make(chan Response, 1)

	mb.lk.Lock()
	defer mb.lk.Unlock()

	if mb.maxIndex >= 0 && index > uint64(mb.maxIndex) {
		mb.waitingEntry++
		go func() {
			mb.lk.Lock()
			defer mb.lk.Unlock()
			for index > uint64(mb.maxIndex) {
				mb.cond.Wait()
			}
			out <- Response{Entry: mb.entryForIndex(index)}
			mb.waitingEntry--
			mb.cond.Broadcast()
		}()
	} else {
		out <- Response{Entry: mb.entryForIndex(index)}
	}

	return out
}

func (mb *MockBeacon) VerifyEntry(from types.BeaconEntry, _prevEntrySig []byte) error {
	// TODO: cache this, especially for bls
	oe := mb.entryForIndex(from.Round)
	if !bytes.Equal(from.Data, oe.Data) {
		return xerrors.Errorf("mock beacon entry was invalid!")
	}
	return nil
}

func (mb *MockBeacon) MaxBeaconRoundForEpoch(nv network.Version, epoch abi.ChainEpoch) uint64 {
	// offset for better testing
	return uint64(epoch + 100)
}

var _ RandomBeacon = (*MockBeacon)(nil)
