package client

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/filecoin-project/go-fil-markets/stores"
	"github.com/filecoin-project/lotus/build"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	chunker "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files2 "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"
)

// doImport takes a standard file (src), forms a UnixFS DAG, and writes a
// CARv2 file with positional mapping (backed by the go-filestore library).
func (a *API) doImport(ctx context.Context, src string, dst string) (cid.Cid, error) {
	// This method uses a two-phase approach with a staging CAR blockstore and
	// a final CAR blockstore.
	//
	// This is necessary because of https://github.com/ipld/go-car/issues/196
	//
	// TODO: do we need to chunk twice? Isn't the first output already in the
	//  right order? Can't we just copy the CAR file and replace the header?
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create temp file: %w", err)
	}
	_ = f.Close() // close; we only want the path.
	tmp := f.Name()
	defer os.Remove(tmp)

	// Step 1. Compute the UnixFS DAG and write it to a CARv2 file to get
	// the root CID of the DAG.
	fstore, err := stores.ReadWriteFilestore(tmp)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create temporary filestore: %w", err)
	}

	bsvc := blockservice.New(fstore, offline.Exchange(fstore))
	dags := merkledag.NewDAGService(bsvc)

	root, err := buildUnixFS(ctx, src, dags)
	if err != nil {
		_ = fstore.Close()
		return cid.Undef, xerrors.Errorf("failed to import file to store to compute root: %w", err)
	}

	if err := fstore.Close(); err != nil {
		return cid.Undef, xerrors.Errorf("failed to finalize car filestore: %w", err)
	}

	// Step 2. We now have the root of the UnixFS DAG, and we can write the
	// final CAR for real under `dst`.
	bs, err := blockstore.OpenReadWrite(dst, []cid.Cid{root},
		car.ZeroLengthSectionAsEOF(true),
		blockstore.UseWholeCIDs(true))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create a carv2 read/write blockstore: %w", err)
	}

	bsvc = blockservice.New(bs, offline.Exchange(bs))
	dags = merkledag.NewDAGService(bsvc)
	finalRoot, err := buildUnixFS(ctx, src, dags)
	if err != nil {
		_ = bs.Close()
		return cid.Undef, xerrors.Errorf("failed to create UnixFS DAG with carv2 blockstore: %w", err)
	}

	if err := bs.Finalize(); err != nil {
		return cid.Undef, xerrors.Errorf("failed to finalize car blockstore: %w", err)
	}

	if root != finalRoot {
		return cid.Undef, xerrors.New("roots do not match")
	}

	return root, nil
}

// buildUnixFS builds a UnixFS DAG out of the supplied ordinary file,
// and imports the DAG into the supplied service.
func buildUnixFS(ctx context.Context, src string, dag ipld.DAGService) (cid.Cid, error) {
	f, err := os.Open(src)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to open input file: %w", err)
	}
	defer f.Close() //nolint:errcheck

	stat, err := f.Stat()
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to stat file :%w", err)
	}

	file, err := files2.NewReaderPathFile(src, f, stat)
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
