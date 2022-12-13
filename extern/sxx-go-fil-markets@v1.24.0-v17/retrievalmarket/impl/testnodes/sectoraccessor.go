package testnodes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
)

type sectorKey struct {
	sectorID abi.SectorNumber
	offset   abi.UnpaddedPieceSize
	length   abi.UnpaddedPieceSize
}

// TestSectorAccessor is a mock implementation of the SectorAccessor
type TestSectorAccessor struct {
	lk           sync.Mutex
	sectorStubs  map[sectorKey][]byte
	expectations map[sectorKey]struct{}
	received     map[sectorKey]struct{}
	unsealed     map[sectorKey]struct{}
	unsealPaused chan struct{}
}

var _ retrievalmarket.SectorAccessor = &TestSectorAccessor{}

// NewTestSectorAccessor instantiates a new TestSectorAccessor
func NewTestSectorAccessor() *TestSectorAccessor {
	return &TestSectorAccessor{
		sectorStubs:  make(map[sectorKey][]byte),
		expectations: make(map[sectorKey]struct{}),
		received:     make(map[sectorKey]struct{}),
		unsealed:     make(map[sectorKey]struct{}),
	}
}

func (trpn *TestSectorAccessor) IsUnsealed(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) (bool, error) {
	_, ok := trpn.unsealed[sectorKey{sectorID, offset, length}]
	return ok, nil
}

func (trpn *TestSectorAccessor) MarkUnsealed(ctx context.Context, sectorID abi.SectorNumber, offset abi.UnpaddedPieceSize, length abi.UnpaddedPieceSize) {
	trpn.unsealed[sectorKey{sectorID, offset, length}] = struct{}{}
}

// StubUnseal stubs a response to attempting to unseal a sector with the given paramters
func (trpn *TestSectorAccessor) StubUnseal(sectorID abi.SectorNumber, offset, length abi.UnpaddedPieceSize, data []byte) {
	trpn.sectorStubs[sectorKey{sectorID, offset, length}] = data
}

// ExpectFailedUnseal indicates an expectation that a call will be made to unseal
// a sector with the given params and should fail
func (trpn *TestSectorAccessor) ExpectFailedUnseal(sectorID abi.SectorNumber, offset, length abi.UnpaddedPieceSize) {
	trpn.expectations[sectorKey{sectorID, offset, length}] = struct{}{}
}

// ExpectUnseal indicates an expectation that a call will be made to unseal
// a sector with the given params and should return the given data
func (trpn *TestSectorAccessor) ExpectUnseal(sectorID abi.SectorNumber, offset, length abi.UnpaddedPieceSize, data []byte) {
	trpn.expectations[sectorKey{sectorID, offset, length}] = struct{}{}
	trpn.StubUnseal(sectorID, offset, length, data)
}

func (trpn *TestSectorAccessor) PauseUnseal() {
	trpn.unsealPaused = make(chan struct{})
}

func (trpn *TestSectorAccessor) FinishUnseal() {
	close(trpn.unsealPaused)
}

// UnsealSector simulates unsealing a sector by returning a stubbed response
// or erroring
func (trpn *TestSectorAccessor) UnsealSector(ctx context.Context, sectorID abi.SectorNumber, offset, length abi.UnpaddedPieceSize) (io.ReadCloser, error) {
	trpn.lk.Lock()
	defer trpn.lk.Unlock()

	if trpn.unsealPaused != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-trpn.unsealPaused:
		}
	}

	trpn.received[sectorKey{sectorID, offset, length}] = struct{}{}
	data, ok := trpn.sectorStubs[sectorKey{sectorID, offset, length}]
	if !ok {
		return nil, errors.New("Could not unseal")
	}

	return ioutil.NopCloser(bytes.NewReader(data)), nil
}

// VerifyExpectations verifies that all expected calls were made and no other calls
// were made
func (trpn *TestSectorAccessor) VerifyExpectations(t *testing.T) {
	require.Equal(t, trpn.expectations, trpn.received)
}
