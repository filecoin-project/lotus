package retrievaladapter

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"

	retrievalimpl "github.com/filecoin-project/go-fil-markets/retrievalmarket/impl"
)

// BlockstoreManager provides an interface to get a blockstore and perform
// transformations based on a DAG root CID.
// It is used to provide the retrieval client with a blockstore backed by
// either an IPFS or CARv2 file.
type BlockstoreManager interface {
	retrievalimpl.BlockstoreAccessor
	WriteCarV1(ctx context.Context, rootCid cid.Cid, out string) error
	WriteUnixFSFile(ctx context.Context, rootCid cid.Cid, out string) error
	Cleanup(rootCid cid.Cid) error
}

// carFileBSMgr provides access to CARv2 files as blockstores.
// It creates the CAR files in the given baseDir and uses the root CID
// as the filename.
type carFileBSMgr struct {
	baseDir string

	lk      sync.Mutex
	bstores map[cid.Cid]*blockstore.ReadWrite
}

var _ BlockstoreManager = (*carFileBSMgr)(nil)

func NewCarFileBlockstoreManager(baseDir string) *carFileBSMgr {
	return &carFileBSMgr{
		baseDir: baseDir,
		bstores: make(map[cid.Cid]*blockstore.ReadWrite),
	}
}

func (ba *carFileBSMgr) Get(rootCid cid.Cid) (bstore.Blockstore, error) {
	carPath := ba.getPath(rootCid)

	ba.lk.Lock()
	defer ba.lk.Unlock()

	// Check if a blockstore has already been created for the root CID
	if bs, ok := ba.bstores[rootCid]; ok {
		return bs, nil
	}

	// Create a new read-write blockstore
	bs, err := blockstore.OpenReadWrite(carPath, []cid.Cid{rootCid}, blockstore.UseWholeCIDs(true))
	if err != nil {
		return nil, xerrors.Errorf("failed to create read-write blockstore: %w", err)
	}

	ba.bstores[rootCid] = bs

	return bs, nil
}

func (ba *carFileBSMgr) Close(rootCid cid.Cid) error {
	ba.lk.Lock()
	defer ba.lk.Unlock()

	if bs, ok := ba.bstores[rootCid]; ok {
		delete(ba.bstores, rootCid)

		// If the blockstore has already been finalized, calling Finalize again
		// will return an error. For our purposes it's simplest if Finalize is
		// idempotent so we just ignore any error.
		_ = bs.Finalize()
	}

	return nil
}

func (ba *carFileBSMgr) WriteCarV1(ctx context.Context, rootCid cid.Cid, out string) error {
	carV2FilePath := ba.getPath(rootCid)
	_, err := os.Stat(carV2FilePath)
	if err != nil {
		return xerrors.Errorf("could not get CAR v2 file for root CID %s: %w", rootCid, err)
	}

	carv2Reader, err := carv2.OpenReader(carV2FilePath)
	if err != nil {
		return err
	}
	defer carv2Reader.Close() //nolint:errcheck

	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	_, err = io.Copy(f, carv2Reader.DataReader())
	return err
}

func (ba *carFileBSMgr) WriteUnixFSFile(ctx context.Context, rootCid cid.Cid, out string) error {
	carV2FilePath := ba.getPath(rootCid)
	_, err := os.Stat(carV2FilePath)
	if err != nil {
		return xerrors.Errorf("could not get CAR v2 file for root CID %s: %w", rootCid, err)
	}

	readOnly, err := blockstore.OpenReadOnly(
		carV2FilePath,
		carv2.ZeroLengthSectionAsEOF(true),
		blockstore.UseWholeCIDs(true))
	if err != nil {
		return err
	}
	defer readOnly.Close() //nolint:errcheck

	return writeBlockstoreToUnixfsFile(ctx, readOnly, rootCid, out)
}

// Cleanup removes the CARv2 file associated with the root CID
func (ba *carFileBSMgr) Cleanup(rootCid cid.Cid) error {
	carV2FilePath := ba.getPath(rootCid)
	_, err := os.Stat(carV2FilePath)
	if err != nil {
		// If the file has already been moved / deleted, no further action
		// required, just ignore it
		return nil
	}

	return os.Remove(carV2FilePath)
}

func (ba *carFileBSMgr) getPath(rootCid cid.Cid) string {
	return filepath.Join(ba.baseDir, rootCid.String(), ".car")
}

// passThroughBSAccessor implements a BlockstoreAccessor that gets and puts
// blocks in the underlying monolithic blockstore
type passThroughBSAccessor struct {
	bs bstore.Blockstore
}

var _ BlockstoreManager = (*passThroughBSAccessor)(nil)

func NewPassThroughBlockstoreManager(bs bstore.Blockstore) *passThroughBSAccessor {
	return &passThroughBSAccessor{bs: bs}
}

func (p *passThroughBSAccessor) Get(rootCid cid.Cid) (bstore.Blockstore, error) {
	return p.bs, nil
}

func (p *passThroughBSAccessor) Close(rootCid cid.Cid) error {
	return nil
}

func (p *passThroughBSAccessor) WriteCarV1(ctx context.Context, rootCid cid.Cid, out string) error {
	// TODO:
	return xerrors.Errorf("implement me")
}

func (p *passThroughBSAccessor) WriteUnixFSFile(ctx context.Context, rootCid cid.Cid, out string) error {
	return writeBlockstoreToUnixfsFile(ctx, p.bs, rootCid, out)
}

func (p *passThroughBSAccessor) Cleanup(rootCid cid.Cid) error {
	return nil
}

func writeBlockstoreToUnixfsFile(ctx context.Context, bs bstore.Blockstore, rootCid cid.Cid, out string) error {
	bsvc := blockservice.New(bs, offline.Exchange(bs))
	dag := merkledag.NewDAGService(bsvc)

	nd, err := dag.Get(ctx, rootCid)
	if err != nil {
		return err
	}
	file, err := unixfile.NewUnixfsFile(ctx, dag, nd)
	if err != nil {
		return err
	}

	return files.WriteTo(file, out)
}
