package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/filecoin-project/go-multistore"
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
	"github.com/ipld/go-car"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"golang.org/x/xerrors"
)

// importNormalFileToCARv2 imports the client's normal file to a CARv2 file.
// It first generates a Unixfs DAG using the given store to store the resulting blocks and gets the root cid of the Unixfs DAG.
// It then writes out the Unixfs DAG to a CARv2 file by generating a Unixfs DAG again using a CARv2 read-write blockstore as the backing store
// and then finalizing the CARv2 read-write blockstore to get the backing CARv2 file.
func importNormalFileToCARv2(ctx context.Context, st *multistore.Store, inputFilePath string, outputCARv2Path string) (c cid.Cid, finalErr error) {
	// create the UnixFS DAG and import the file to store to get the root.
	root, err := importNormalFileToUnixfsDAG(ctx, inputFilePath, st.DAG)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to import file to store: %w", err)
	}

	//---
	// transform the file to a CARv2 file by writing out a Unixfs DAG via the CARv2 read-write blockstore.
	rw, err := blockstore.NewReadWrite(outputCARv2Path, []cid.Cid{root})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create a CARv2 read-write blockstore: %w", err)
	}

	// make sure to call finalize on the CARv2 read-write blockstore to ensure that the blockstore flushes out a valid CARv2 file
	// and releases the file handle it acquires.
	defer func() {
		err := rw.Finalize()
		if finalErr != nil {
			finalErr = xerrors.Errorf("failed to import file to CARv2, err=%w", finalErr)
		} else {
			finalErr = err
		}
	}()

	bsvc := blockservice.New(rw, offline.Exchange(rw))
	root2, err := importNormalFileToUnixfsDAG(ctx, inputFilePath, merkledag.NewDAGService(bsvc))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to create Unixfs DAG with CARv2 blockstore: %w", err)
	}

	fmt.Printf("\n root1 is %s and root2 is %s", root, root2)

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
	defer f.Close()

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

// transformCarToCARv2 transforms a client's CAR file to a CARv2 file.
func transformCarToCARv2(inputCARPath string, outputCARv2Path string) (root cid.Cid, err error) {
	inputF, err := os.Open(inputCARPath)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to open input CAR: %w", err)
	}
	defer inputF.Close() //nolint:errcheck

	// read the CAR header to determine the DAG root and the CAR version.
	hd, _, err := car.ReadHeader(bufio.NewReader(inputF))
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to read CAR header: %w", err)
	}
	if len(hd.Roots) != 1 {
		return cid.Undef, xerrors.New("cannot import CAR with more than one root")
	}

	switch hd.Version {
	case 2:

		// This is a CARv2, we can import it as it is by simply copying it.
		outF, err := os.Open(outputCARv2Path)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to open output CARv2 file: %w", err)
		}
		defer outF.Close()
		_, err = io.Copy(outF, inputF)
		if err != nil {
			return cid.Undef, xerrors.Errorf("failed to copy CARv2 file: %w", err)
		}
	case 1:

		// This is a CARv1, let's transform it to a CARv2.
		if err := carv2.WrapV1File(inputCARPath, outputCARv2Path); err != nil {
			return cid.Undef, xerrors.Errorf("failed to transform CARv1 to CARv2: %w", err)
		}

	default:
		return cid.Undef, xerrors.Errorf("unrecognized CAR version %d", hd.Version)
	}

	return hd.Roots[0], nil
}
