package client

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/filecoin-project/go-fil-markets/stores"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
)

func unixFSCidBuilder() (cid.Builder, error) {
	prefix, err := merkledag.PrefixForCidVersion(1)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize UnixFS CID Builder: %w", err)
	}
	prefix.MhType = DefaultHashFunction
	b := cidutil.InlineBuilder{
		Builder: prefix,
		Limit:   126,
	}
	return b, nil
}

// createUnixFSFilestore takes a standard file whose path is src, forms a UnixFS DAG, and
// writes a CARv2 file with positional mapping (backed by the go-filestore library).
func (a *API) createUnixFSFilestore(ctx context.Context, srcPath string, dstPath string) (cid.Cid, error) {
	// This method uses a two-phase approach with a staging CAR blockstore and
	// a final CAR blockstore.
	//
	// This is necessary because of https://github.com/ipld/go-car/issues/196
	//
	// TODO: do we need to chunk twice? Isn't the first output already in the
	//  right order? Can't we just copy the CAR file and replace the header?

	src, err := os.Open(srcPath)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to open input file: %w", err)
	}
	defer src.Close() //nolint:errcheck

	stat, err := src.Stat()
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to stat file :%w", err)
	}

	file, err := files.NewReaderPathFile(srcPath, src, stat)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create reader path file: %w", err)
	}

	f, err := ioutil.TempFile("", "")
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create temp file: %w", err)
	}
	_ = f.Close() // close; we only want the path.

	tmp := f.Name()
	defer os.Remove(tmp) //nolint:errcheck

	// Step 1. Compute the UnixFS DAG and write it to a CARv2 file to get
	// the root CID of the DAG.
	fstore, err := stores.ReadWriteFilestore(tmp)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create temporary filestore: %w", err)
	}

	finalRoot1, err := buildUnixFS(ctx, file, fstore, true)
	if err != nil {
		_ = fstore.Close()
		return cid.Undef, xerrors.Errorf("failed to import file to store to compute root: %w", err)
	}

	if err := fstore.Close(); err != nil {
		return cid.Undef, xerrors.Errorf("failed to finalize car filestore: %w", err)
	}

	// Step 2. We now have the root of the UnixFS DAG, and we can write the
	// final CAR for real under `dst`.
	bs, err := stores.ReadWriteFilestore(dstPath, finalRoot1)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create a carv2 read/write filestore: %w", err)
	}

	// rewind file to the beginning.
	if _, err := src.Seek(0, 0); err != nil {
		return cid.Undef, xerrors.Errorf("failed to rewind file: %w", err)
	}

	finalRoot2, err := buildUnixFS(ctx, file, bs, true)
	if err != nil {
		_ = bs.Close()
		return cid.Undef, xerrors.Errorf("failed to create UnixFS DAG with carv2 blockstore: %w", err)
	}

	if err := bs.Close(); err != nil {
		return cid.Undef, xerrors.Errorf("failed to finalize car blockstore: %w", err)
	}

	if finalRoot1 != finalRoot2 {
		return cid.Undef, xerrors.New("roots do not match")
	}

	return finalRoot1, nil
}

// buildUnixFS builds a UnixFS DAG out of the supplied reader,
// and imports the DAG into the supplied service.
func buildUnixFS(ctx context.Context, reader io.Reader, into bstore.Blockstore, filestore bool) (cid.Cid, error) {
	b, err := unixFSCidBuilder()
	if err != nil {
		return cid.Undef, err
	}

	bsvc := blockservice.New(into, offline.Exchange(into))
	dags := merkledag.NewDAGService(bsvc)
	bufdag := ipld.NewBufferedDAG(ctx, dags)

	params := ihelper.DagBuilderParams{
		Maxlinks:   build.UnixfsLinksPerLevel,
		RawLeaves:  true,
		CidBuilder: b,
		Dagserv:    bufdag,
		NoCopy:     filestore,
	}

	db, err := params.New(chunker.NewSizeSplitter(reader, int64(build.UnixfsChunkSize)))
	if err != nil {
		return cid.Undef, err
	}
	nd, err := balanced.Layout(db)
	if err != nil {
		return cid.Undef, err
	}

	if err := bufdag.Commit(); err != nil {
		return cid.Undef, err
	}

	return nd.Cid(), nil
}
