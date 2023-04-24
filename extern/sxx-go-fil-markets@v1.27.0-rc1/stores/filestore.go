package stores

import (
	"context"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/ipfs/go-filestore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	mh "github.com/multiformats/go-multihash"
	"golang.org/x/xerrors"
)

// ReadOnlyFilestore opens the CAR in the specified path as as a read-only
// blockstore, and fronts it with a Filestore whose positional mappings are
// stored inside the CAR itself. It must be closed after done.
func ReadOnlyFilestore(path string) (ClosableBlockstore, error) {
	ro, err := OpenReadOnly(path,
		carv2.ZeroLengthSectionAsEOF(true),
		blockstore.UseWholeCIDs(true),
	)

	if err != nil {
		return nil, err
	}

	bs, err := FilestoreOf(ro)
	if err != nil {
		return nil, err
	}

	return &closableBlockstore{Blockstore: bs, closeFn: ro.Close}, nil
}

// ReadWriteFilestore opens the CAR in the specified path as as a read-write
// blockstore, and fronts it with a Filestore whose positional mappings are
// stored inside the CAR itself. It must be closed after done. Closing will
// finalize the CAR blockstore.
func ReadWriteFilestore(path string, roots ...cid.Cid) (ClosableBlockstore, error) {
	rw, err := OpenReadWrite(path, roots,
		carv2.ZeroLengthSectionAsEOF(true),
		carv2.StoreIdentityCIDs(true),
		blockstore.UseWholeCIDs(true),
	)
	if err != nil {
		return nil, err
	}

	bs, err := FilestoreOf(rw)
	if err != nil {
		return nil, err
	}

	return &closableBlockstore{Blockstore: bs, closeFn: rw.Finalize}, nil
}

// FilestoreOf returns a FileManager/Filestore backed entirely by a
// blockstore without requiring a datastore. It achieves this by coercing the
// blockstore into a datastore. The resulting blockstore is suitable for usage
// with DagBuilderHelper with DagBuilderParams#NoCopy=true.
func FilestoreOf(bs bstore.Blockstore) (bstore.Blockstore, error) {
	coercer := &dsCoercer{bs}

	// the FileManager stores positional infos (positional mappings) in a
	// datastore, which in our case is the blockstore coerced into a datastore.
	//
	// Passing the root dir as a base path makes me uneasy, but these filestores
	// are only used locally.
	fm := filestore.NewFileManager(coercer, "/")
	fm.AllowFiles = true

	// the Filestore sifts leaves (PosInfos) from intermediate nodes. It writes
	// PosInfo leaves to the datastore (which in our case is the coerced
	// blockstore), and the intermediate nodes to the blockstore proper (since
	// they cannot be mapped to the file.
	fstore := filestore.NewFilestore(bs, fm)
	bs = bstore.NewIdStore(fstore)

	return bs, nil
}

var cidBuilder = cid.V1Builder{Codec: cid.Raw, MhType: mh.SHA2_256}

// dsCoercer coerces a Blockstore to present a datastore interface, apt for
// usage with the Filestore/FileManager. Only PosInfos will be written through
// this path.
type dsCoercer struct {
	bstore.Blockstore
}

var _ datastore.Batching = (*dsCoercer)(nil)

func (crcr *dsCoercer) Get(ctx context.Context, key datastore.Key) (value []byte, err error) {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to create cid: %w", err)
	}

	blk, err := crcr.Blockstore.Get(ctx, c)
	if err != nil {
		return nil, xerrors.Errorf("failed to get cid %s: %w", c, err)
	}
	return blk.RawData(), nil
}

func (crcr *dsCoercer) Put(ctx context.Context, key datastore.Key, value []byte) error {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to create cid: %w", err)
	}
	blk, err := blocks.NewBlockWithCid(value, c)
	if err != nil {
		return xerrors.Errorf("failed to create block: %w", err)
	}
	if err := crcr.Blockstore.Put(ctx, blk); err != nil {
		return xerrors.Errorf("failed to put block: %w", err)
	}
	return nil
}

func (crcr *dsCoercer) Has(ctx context.Context, key datastore.Key) (exists bool, err error) {
	c, err := cidBuilder.Sum(key.Bytes())
	if err != nil {
		return false, xerrors.Errorf("failed to create cid: %w", err)
	}
	return crcr.Blockstore.Has(ctx, c)
}

func (crcr *dsCoercer) Batch(_ context.Context) (datastore.Batch, error) {
	return datastore.NewBasicBatch(crcr), nil
}

func (crcr *dsCoercer) GetSize(_ context.Context, _ datastore.Key) (size int, err error) {
	return 0, xerrors.New("operation NOT supported: GetSize")
}

func (crcr *dsCoercer) Query(_ context.Context, _ query.Query) (query.Results, error) {
	return nil, xerrors.New("operation NOT supported: Query")
}

func (crcr *dsCoercer) Delete(_ context.Context, _ datastore.Key) error {
	return xerrors.New("operation NOT supported: Delete")
}

func (crcr *dsCoercer) Sync(_ context.Context, _ datastore.Key) error {
	return xerrors.New("operation NOT supported: Sync")
}

func (crcr *dsCoercer) Close() error {
	return nil
}

type closableBlockstore struct {
	bstore.Blockstore
	closeFn func() error
}

func (c *closableBlockstore) Close() error {
	return c.closeFn()
}
