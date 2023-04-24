package shared_testutil

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/ipfs/go-cid"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	carindex "github.com/ipld/go-car/v2/index"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/stores"
)

type registration struct {
	CarPath   string
	EagerInit bool
}

// MockDagStoreWrapper is used to mock out the DAG store wrapper operations
// for the tests.
// It simulates getting deal info from a piece store and unsealing the data for
// the deal from a retrieval provider node.
type MockDagStoreWrapper struct {
	pieceStore piecestore.PieceStore
	sa         retrievalmarket.SectorAccessor

	lk              sync.Mutex
	registrations   map[cid.Cid]registration
	piecesWithBlock map[cid.Cid][]cid.Cid
}

var _ stores.DAGStoreWrapper = (*MockDagStoreWrapper)(nil)

func NewMockDagStoreWrapper(pieceStore piecestore.PieceStore, sa retrievalmarket.SectorAccessor) *MockDagStoreWrapper {
	return &MockDagStoreWrapper{
		pieceStore:      pieceStore,
		sa:              sa,
		registrations:   make(map[cid.Cid]registration),
		piecesWithBlock: make(map[cid.Cid][]cid.Cid),
	}
}

func (m *MockDagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) error {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.registrations[pieceCid] = registration{
		CarPath:   carPath,
		EagerInit: eagerInit,
	}

	resch <- dagstore.ShardResult{}
	return nil
}

func (m *MockDagStoreWrapper) DestroyShard(ctx context.Context, pieceCid cid.Cid, resch chan dagstore.ShardResult) error {
	m.lk.Lock()
	defer m.lk.Unlock()
	delete(m.registrations, pieceCid)
	resch <- dagstore.ShardResult{}
	return nil
}

func (m *MockDagStoreWrapper) GetIterableIndexForPiece(c cid.Cid) (carindex.IterableIndex, error) {
	return nil, nil
}

func (m *MockDagStoreWrapper) MigrateDeals(ctx context.Context, deals []storagemarket.MinerDeal) (bool, error) {
	return true, nil
}

func (m *MockDagStoreWrapper) LenRegistrations() int {
	m.lk.Lock()
	defer m.lk.Unlock()

	return len(m.registrations)
}

func (m *MockDagStoreWrapper) GetRegistration(pieceCid cid.Cid) (registration, bool) {
	m.lk.Lock()
	defer m.lk.Unlock()

	reg, ok := m.registrations[pieceCid]
	return reg, ok
}

func (m *MockDagStoreWrapper) ClearRegistrations() {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.registrations = make(map[cid.Cid]registration)
}

func (m *MockDagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (stores.ClosableBlockstore, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	_, ok := m.registrations[pieceCid]
	if !ok {
		return nil, xerrors.Errorf("no shard for piece CID %s", pieceCid)
	}

	// Get the piece info from the piece store
	pi, err := m.pieceStore.GetPieceInfo(pieceCid)
	if err != nil {
		return nil, err
	}

	// Unseal the sector data for the deal
	deal := pi.Deals[0]
	r, err := m.sa.UnsealSector(ctx, deal.SectorID, deal.Offset.Unpadded(), deal.Length.Unpadded())
	if err != nil {
		return nil, xerrors.Errorf("error unsealing deal for piece %s: %w", pieceCid, err)
	}

	return getBlockstoreFromReader(r, pieceCid)
}

func getBlockstoreFromReader(r io.ReadCloser, pieceCid cid.Cid) (stores.ClosableBlockstore, error) {
	// Write the piece to a file
	tmpFile, err := os.CreateTemp("", "dagstoretmp")
	if err != nil {
		return nil, xerrors.Errorf("creating temp file for piece CID %s: %w", pieceCid, err)
	}

	_, err = io.Copy(tmpFile, r)
	if err != nil {
		return nil, xerrors.Errorf("copying read stream to temp file for piece CID %s: %w", pieceCid, err)
	}

	err = tmpFile.Close()
	if err != nil {
		return nil, xerrors.Errorf("closing temp file for piece CID %s: %w", pieceCid, err)
	}

	// Get a blockstore from the CAR file
	return blockstore.OpenReadOnly(tmpFile.Name(), carv2.ZeroLengthSectionAsEOF(true), blockstore.UseWholeCIDs(true))
}

func (m *MockDagStoreWrapper) Close() error {
	return nil
}

func (m *MockDagStoreWrapper) GetPiecesContainingBlock(blockCID cid.Cid) ([]cid.Cid, error) {
	m.lk.Lock()
	defer m.lk.Unlock()

	pieces, ok := m.piecesWithBlock[blockCID]
	if !ok {
		return nil, retrievalmarket.ErrNotFound
	}

	return pieces, nil
}

// Used by the tests to add an entry to the index of block CID -> []piece CID
func (m *MockDagStoreWrapper) AddBlockToPieceIndex(blockCID cid.Cid, pieceCid cid.Cid) {
	m.lk.Lock()
	defer m.lk.Unlock()

	pieces, ok := m.piecesWithBlock[blockCID]
	if !ok {
		m.piecesWithBlock[blockCID] = []cid.Cid{pieceCid}
	} else {
		m.piecesWithBlock[blockCID] = append(pieces, pieceCid)
	}
}
