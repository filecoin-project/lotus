package stores

import (
	"io"
	"sync"

	bstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"
)

// ReadOnlyBlockstores tracks open read blockstores.
type ReadOnlyBlockstores struct {
	mu     sync.RWMutex
	stores map[string]bstore.Blockstore
}

func NewReadOnlyBlockstores() *ReadOnlyBlockstores {
	return &ReadOnlyBlockstores{
		stores: make(map[string]bstore.Blockstore),
	}
}

func (r *ReadOnlyBlockstores) Track(key string, bs bstore.Blockstore) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.stores[key]; ok {
		return false, nil
	}

	r.stores[key] = bs
	return true, nil
}

func (r *ReadOnlyBlockstores) Get(key string) (bstore.Blockstore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if bs, ok := r.stores[key]; ok {
		return bs, nil
	}

	return nil, xerrors.Errorf("could not get blockstore for key %s: %w", key, ErrNotFound)
}

func (r *ReadOnlyBlockstores) Untrack(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if bs, ok := r.stores[key]; ok {
		delete(r.stores, key)
		if closer, ok := bs.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				return xerrors.Errorf("failed to close read-only blockstore: %w", err)
			}
		}
	}

	return nil
}
