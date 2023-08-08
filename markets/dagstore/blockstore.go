package dagstore

import (
	"context"
	"io"

	bstore "github.com/ipfs/boxo/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
)

// Blockstore promotes a dagstore.ReadBlockstore to a full closeable Blockstore,
// stubbing out the write methods with erroring implementations.
type Blockstore struct {
	dagstore.ReadBlockstore
	io.Closer
}

var _ bstore.Blockstore = (*Blockstore)(nil)

func (b *Blockstore) DeleteBlock(context.Context, cid.Cid) error {
	return xerrors.Errorf("DeleteBlock called but not implemented")
}

func (b *Blockstore) Put(context.Context, blocks.Block) error {
	return xerrors.Errorf("Put called but not implemented")
}

func (b *Blockstore) PutMany(context.Context, []blocks.Block) error {
	return xerrors.Errorf("PutMany called but not implemented")
}
