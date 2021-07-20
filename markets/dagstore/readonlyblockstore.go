package dagstore

import (
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
)

// ReadOnlyBlockstore stubs out Blockstore mutators with methods that error out
type ReadOnlyBlockstore struct {
	dagstore.ReadBlockstore
}

func NewReadOnlyBlockstore(rbs dagstore.ReadBlockstore) bstore.Blockstore {
	return ReadOnlyBlockstore{ReadBlockstore: rbs}
}

func (r ReadOnlyBlockstore) DeleteBlock(c cid.Cid) error {
	return xerrors.Errorf("DeleteBlock called but not implemented")
}

func (r ReadOnlyBlockstore) Put(block blocks.Block) error {
	return xerrors.Errorf("Put called but not implemented")
}

func (r ReadOnlyBlockstore) PutMany(blocks []blocks.Block) error {
	return xerrors.Errorf("PutMany called but not implemented")
}

var _ bstore.Blockstore = (*ReadOnlyBlockstore)(nil)
