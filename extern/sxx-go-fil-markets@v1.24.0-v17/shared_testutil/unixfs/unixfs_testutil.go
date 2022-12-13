package unixfs

import (
	"context"
	"os"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-cidutil"
	chunk "github.com/ipfs/go-ipfs-chunker"
	files "github.com/ipfs/go-ipfs-files"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
)

const (
	defaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)
	unixfsChunkSize     = uint64(1 << 10)
	unixfsLinksPerLevel = 1024
)

func WriteUnixfsDAGTo(t *testing.T, path string, into ipldformat.DAGService) cid.Cid {
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	stat, err := file.Stat()
	require.NoError(t, err)

	// get a IPLD reader path file
	// required to write the Unixfs DAG blocks to a filestore
	rpf, err := files.NewReaderPathFile(file.Name(), file, stat)
	require.NoError(t, err)

	// generate the dag and get the root
	// import to UnixFS
	prefix, err := merkledag.PrefixForCidVersion(1)
	require.NoError(t, err)

	prefix.MhType = defaultHashFunction

	bufferedDS := ipldformat.NewBufferedDAG(context.Background(), into)
	params := ihelper.DagBuilderParams{
		Maxlinks:  unixfsLinksPerLevel,
		RawLeaves: true,
		CidBuilder: cidutil.InlineBuilder{
			Builder: prefix,
			Limit:   126,
		},
		Dagserv: bufferedDS,
		NoCopy:  true,
	}

	db, err := params.New(chunk.NewSizeSplitter(rpf, int64(unixfsChunkSize)))
	require.NoError(t, err)

	nd, err := balanced.Layout(db)
	require.NoError(t, err)

	err = bufferedDS.Commit()
	require.NoError(t, err)
	require.NoError(t, rpf.Close())

	return nd.Cid()
}
