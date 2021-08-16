package storageadapter

import (
	"sync"

	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/stores"
	"github.com/filecoin-project/lotus/node/repo/imports"
)

// ProxyBlockstoreAccessor is an accessor that returns a fixed blockstore.
// To be used in combination with IPFS integration.
type ProxyBlockstoreAccessor struct {
	Blockstore blockstore.Blockstore
}

var _ storagemarket.BlockstoreAccessor = (*ProxyBlockstoreAccessor)(nil)

func NewFixedBlockstoreAccessor(bs blockstore.Blockstore) storagemarket.BlockstoreAccessor {
	return &ProxyBlockstoreAccessor{Blockstore: bs}
}

func (p *ProxyBlockstoreAccessor) Get(cid storagemarket.PayloadCID) (blockstore.Blockstore, error) {
	return p.Blockstore, nil
}

func (p *ProxyBlockstoreAccessor) Done(cid storagemarket.PayloadCID) error {
	return nil
}

// ImportsBlockstoreAccessor is a blockstore accessor backed by the
// imports.Manager.
type ImportsBlockstoreAccessor struct {
	m    *imports.Manager
	lk   sync.Mutex
	open map[cid.Cid]struct {
		st   stores.ClosableBlockstore
		refs int
	}
}

var _ storagemarket.BlockstoreAccessor = (*ImportsBlockstoreAccessor)(nil)

func NewImportsBlockstoreAccessor(importmgr *imports.Manager) *ImportsBlockstoreAccessor {
	return &ImportsBlockstoreAccessor{
		m: importmgr,
		open: make(map[cid.Cid]struct {
			st   stores.ClosableBlockstore
			refs int
		}),
	}
}

func (s *ImportsBlockstoreAccessor) Get(payloadCID storagemarket.PayloadCID) (blockstore.Blockstore, error) {
	s.lk.Lock()
	defer s.lk.Unlock()

	e, ok := s.open[payloadCID]
	if ok {
		e.refs++
		return e.st, nil
	}

	path, err := s.m.CARPathFor(payloadCID)
	if err != nil {
		return nil, xerrors.Errorf("failed to get client blockstore for root %s: %w", payloadCID, err)
	}
	if path == "" {
		return nil, xerrors.Errorf("no client blockstore for root %s", payloadCID)
	}
	ret, err := stores.ReadOnlyFilestore(path)
	if err != nil {
		return nil, err
	}
	e.st = ret
	s.open[payloadCID] = e
	return ret, nil
}

func (s *ImportsBlockstoreAccessor) Done(payloadCID storagemarket.PayloadCID) error {
	s.lk.Lock()
	defer s.lk.Unlock()

	e, ok := s.open[payloadCID]
	if !ok {
		return nil
	}

	e.refs--
	if e.refs == 0 {
		if err := e.st.Close(); err != nil {
			log.Warnf("failed to close blockstore: %s", err)
		}
		delete(s.open, payloadCID)
	}
	return nil
}
