package shared_testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
)

// TestPieceStore is piecestore who's query results are mocked
type TestPieceStore struct {
	addPieceBlockLocationsError error
	addDealForPieceError        error
	getPieceInfoError           error
	piecesStubbed               map[cid.Cid]piecestore.PieceInfo
	piecesExpected              map[cid.Cid]struct{}
	piecesReceived              map[cid.Cid]struct{}
	cidInfosStubbed             map[cid.Cid]piecestore.CIDInfo
	cidInfosExpected            map[cid.Cid]struct{}
	cidInfosReceived            map[cid.Cid]struct{}
}

// TestPieceStoreParams sets parameters for a piece store
type TestPieceStoreParams struct {
	AddDealForPieceError        error
	AddPieceBlockLocationsError error
	GetPieceInfoError           error
}

var _ piecestore.PieceStore = &TestPieceStore{}

// NewTestPieceStore creates a TestPieceStore
func NewTestPieceStore() *TestPieceStore {
	return NewTestPieceStoreWithParams(TestPieceStoreParams{})
}

// NewTestPieceStoreWithParams creates a TestPieceStore with the given parameters
func NewTestPieceStoreWithParams(params TestPieceStoreParams) *TestPieceStore {
	return &TestPieceStore{
		addDealForPieceError:        params.AddDealForPieceError,
		addPieceBlockLocationsError: params.AddPieceBlockLocationsError,
		getPieceInfoError:           params.GetPieceInfoError,
		piecesStubbed:               make(map[cid.Cid]piecestore.PieceInfo),
		piecesExpected:              make(map[cid.Cid]struct{}),
		piecesReceived:              make(map[cid.Cid]struct{}),
		cidInfosStubbed:             make(map[cid.Cid]piecestore.CIDInfo),
		cidInfosExpected:            make(map[cid.Cid]struct{}),
		cidInfosReceived:            make(map[cid.Cid]struct{}),
	}
}

// StubPiece creates a return value for the given piece cid without expecting it
// to be called
func (tps *TestPieceStore) StubPiece(pieceCid cid.Cid, pieceInfo piecestore.PieceInfo) {
	tps.piecesStubbed[pieceCid] = pieceInfo
}

// ExpectPiece records a piece being expected to be queried and return the given piece info
func (tps *TestPieceStore) ExpectPiece(pieceCid cid.Cid, pieceInfo piecestore.PieceInfo) {
	tps.piecesExpected[pieceCid] = struct{}{}
	tps.StubPiece(pieceCid, pieceInfo)
}

// ExpectMissingPiece records a piece being expected to be queried and should fail
func (tps *TestPieceStore) ExpectMissingPiece(pieceCid cid.Cid) {
	tps.piecesExpected[pieceCid] = struct{}{}
}

// StubCID creates a return value for the given CID without expecting it
// to be called
func (tps *TestPieceStore) StubCID(c cid.Cid, cidInfo piecestore.CIDInfo) {
	tps.cidInfosStubbed[c] = cidInfo
}

// ExpectCID records a CID being expected to be queried and return the given CID info
func (tps *TestPieceStore) ExpectCID(c cid.Cid, cidInfo piecestore.CIDInfo) {
	tps.cidInfosExpected[c] = struct{}{}
	tps.StubCID(c, cidInfo)
}

// ExpectMissingCID records a CID being expected to be queried and should fail
func (tps *TestPieceStore) ExpectMissingCID(c cid.Cid) {
	tps.cidInfosExpected[c] = struct{}{}
}

// VerifyExpectations verifies that the piecestore was queried in the expected ways
func (tps *TestPieceStore) VerifyExpectations(t *testing.T) {
	require.Equal(t, tps.piecesExpected, tps.piecesReceived)
	require.Equal(t, tps.cidInfosExpected, tps.cidInfosReceived)
}

// AddDealForPiece returns a preprogrammed error
func (tps *TestPieceStore) AddDealForPiece(pieceCID cid.Cid, _ cid.Cid, dealInfo piecestore.DealInfo) error {
	return tps.addDealForPieceError
}

// AddPieceBlockLocations returns a preprogrammed error
func (tps *TestPieceStore) AddPieceBlockLocations(pieceCID cid.Cid, blockLocations map[cid.Cid]piecestore.BlockLocation) error {
	return tps.addPieceBlockLocationsError
}

func (tps *TestPieceStore) ReturnErrorFromGetPieceInfo(err error) {
	tps.getPieceInfoError = err
}

// GetPieceInfo returns a piece info if it's been stubbed
func (tps *TestPieceStore) GetPieceInfo(pieceCID cid.Cid) (piecestore.PieceInfo, error) {
	if tps.getPieceInfoError != nil {
		return piecestore.PieceInfoUndefined, tps.getPieceInfoError
	}

	tps.piecesReceived[pieceCID] = struct{}{}

	pio, ok := tps.piecesStubbed[pieceCID]
	if ok {
		return pio, nil
	}
	_, ok = tps.piecesExpected[pieceCID]
	if ok {
		return piecestore.PieceInfoUndefined, retrievalmarket.ErrNotFound
	}
	return piecestore.PieceInfoUndefined, errors.New("GetPieceInfo failed")
}

// GetCIDInfo returns cid info if it's been stubbed
func (tps *TestPieceStore) GetCIDInfo(c cid.Cid) (piecestore.CIDInfo, error) {
	tps.cidInfosReceived[c] = struct{}{}

	cio, ok := tps.cidInfosStubbed[c]
	if ok {
		return cio, nil
	}
	_, ok = tps.cidInfosExpected[c]
	if ok {
		return piecestore.CIDInfoUndefined, retrievalmarket.ErrNotFound
	}
	return piecestore.CIDInfoUndefined, errors.New("GetCIDInfo failed")
}

func (tps *TestPieceStore) ListCidInfoKeys() ([]cid.Cid, error) {
	panic("do not call me")
}

func (tps *TestPieceStore) ListPieceInfoKeys() ([]cid.Cid, error) {
	panic("do not call me")
}

func (tps *TestPieceStore) Start(ctx context.Context) error {
	return nil
}

func (tps *TestPieceStore) OnReady(ready shared.ReadyFunc) {
}
