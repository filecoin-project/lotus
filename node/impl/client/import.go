package client

import (
	"context"
	"os"

	"github.com/filecoin-project/go-fil-markets/filestorecaradapter"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/repo/importmgr"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	"github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-filestore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files2 "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"golang.org/x/xerrors"
)

// importNormalFileToFilestoreCARv2 transforms the client's "normal file" to a Unixfs IPLD DAG and writes out the DAG to a CARv2 file
// that can be used to back a filestore.
func (a *API) importNormalFileToFilestoreCARv2(ctx context.Context, importID importmgr.ImportID, inputFilePath string, outputCARv2Path string) (c cid.Cid, finalErr error) {

	// TODO: We've currently put in a hack to create the Unixfs DAG as a CARv2 without using Badger.
	// We first create the Unixfs DAG using a filestore to get the root of the Unixfs DAG.
	// We can't create the UnixfsDAG right away using a CARv2 read-write blockstore as the blockstore
	// needs the root of the DAG during instantiation to write out a valid CARv2 file.
	//
	// In the second pass, we create a CARv2 file with the root present using the root node we get in the above step.
	// This hack should be fixed when CARv2 allows specifying the root AFTER finishing the CARv2 streaming write.
	fm := filestore.NewFileManager(ds_sync.MutexWrap(datastore.NewMapDatastore()), "/")
	fm.AllowFiles = true
	fstore := filestore.NewFilestore(bstore.NewMemorySync(), fm)
	bsvc := blockservice.New(fstore, offline.Exchange(fstore))
	defer bsvc.Close() //nolint:errcheck

	// ---- First Pass ---  Write out the UnixFS DAG to a rootless CARv2 file by instantiating a read-write CARv2 blockstore without the root.
	root, err := importNormalFileToUnixfsDAG(ctx, inputFilePath, merkledag.NewDAGService(bsvc))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to import file to store: %w", err)
	}

	//------ Second Pass --- Now that we have the root of the Unixfs DAG -> write out the Unixfs DAG to a CARv2 file with the root present by using a
	// filestore backed by a read-write CARv2 blockstore.
	fsb, err := filestorecaradapter.NewReadWriteFileStore(outputCARv2Path, []cid.Cid{root})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create a CARv2 read-write blockstore: %w", err)
	}
	defer fsb.Close() //nolint:errcheck

	bsvc = blockservice.New(fsb, offline.Exchange(fsb))
	root2, err := importNormalFileToUnixfsDAG(ctx, inputFilePath, merkledag.NewDAGService(bsvc))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create Unixfs DAG with CARv2 blockstore: %w", err)
	}

	if root != root2 {
		return cid.Undef, xerrors.New("roots do not match")
	}

	return root, nil
}

// importNormalFileToUnixfsDAG transforms a client's normal file to a UnixfsDAG and imports the DAG to the given DAG service.
func importNormalFileToUnixfsDAG(ctx context.Context, inputFilePath string, dag ipld.DAGService) (cid.Cid, error) {
	f, err := os.Open(inputFilePath)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to open input file: %w", err)
	}
	defer f.Close() //nolint:errcheck

	stat, err := f.Stat()
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to stat file :%w", err)
	}

	file, err := files2.NewReaderPathFile(inputFilePath, f, stat)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create reader path file: %w", err)
	}

	bufDs := ipld.NewBufferedDAG(ctx, dag)

	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return cid.Undef, err
	}
	prefix.MhType = DefaultHashFunction

	params := ihelper.DagBuilderParams{
		Maxlinks:  build.UnixfsLinksPerLevel,
		RawLeaves: true,
		CidBuilder: cidutil.InlineBuilder{
			Builder: prefix,
			Limit:   126,
		},
		Dagserv: bufDs,
		NoCopy:  true,
	}

	db, err := params.New(chunker.NewSizeSplitter(file, int64(build.UnixfsChunkSize)))
	if err != nil {
		return cid.Undef, err
	}
	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}

	if err := bufDs.Commit(); err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), nil
}
