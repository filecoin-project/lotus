// Package connmanager tracks open connections maping storage proposal CID -> StorageDealStream
package connmanager

import (
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

// ConnManager is a simple threadsafe map of proposal CID -> network deal stream
type ConnManager struct {
	connsLk sync.RWMutex
	conns   map[cid.Cid]network.StorageDealStream
}

// NewConnManager returns a new conn manager
func NewConnManager() *ConnManager {
	return &ConnManager{
		conns: map[cid.Cid]network.StorageDealStream{},
	}
}

// DealStream returns the deal stream for the given proposal, or an error if not present
func (c *ConnManager) DealStream(proposalCid cid.Cid) (network.StorageDealStream, error) {
	c.connsLk.RLock()
	s, ok := c.conns[proposalCid]
	c.connsLk.RUnlock()
	if ok {
		return s, nil
	}
	return nil, xerrors.New("no connection to provider")
}

// AddStream adds the given stream to the conn manager, and errors if one already
// exists for the given proposal CID
func (c *ConnManager) AddStream(proposalCid cid.Cid, s network.StorageDealStream) error {
	c.connsLk.Lock()
	defer c.connsLk.Unlock()
	_, ok := c.conns[proposalCid]
	if ok {
		return xerrors.Errorf("already have connected for proposal %s", proposalCid)
	}
	c.conns[proposalCid] = s
	return nil
}

// Disconnect removes the given connection from the conn manager and closes
// the stream. It errors if an error occurs closing the stream
func (c *ConnManager) Disconnect(proposalCid cid.Cid) error {
	c.connsLk.Lock()
	defer c.connsLk.Unlock()
	s, ok := c.conns[proposalCid]
	if !ok {
		return nil
	}

	err := s.Close()
	delete(c.conns, proposalCid)
	return err
}
