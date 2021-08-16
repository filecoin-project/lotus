package kit

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	cidutil "github.com/ipfs/go-cidutil"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	chunk "github.com/ipfs/go-ipfs-chunker"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-car"
	"github.com/minio/blake2b-simd"
	mh "github.com/multiformats/go-multihash"

	"github.com/stretchr/testify/require"
)

const unixfsChunkSize uint64 = 1 << 10

var defaultHashFunction = uint64(mh.BLAKE2B_MIN + 31)

// CreateRandomFile creates a random file with the provided seed and the
// provided size.
func CreateRandomFile(t *testing.T, rseed, size int) (path string) {
	if size == 0 {
		size = 1600
	}

	source := io.LimitReader(rand.New(rand.NewSource(int64(rseed))), int64(size))

	file, err := os.CreateTemp(t.TempDir(), "sourcefile.dat")
	require.NoError(t, err)

	n, err := io.Copy(file, source)
	require.NoError(t, err)
	require.EqualValues(t, n, size)

	return file.Name()
}

// CreateRandomFile creates a  normal file with the provided seed and the
// provided size and then transforms it to a CARv1 file and returns it.
func CreateRandomCARv1(t *testing.T, rseed, size int) (carV1FilePath string, origFilePath string) {
	ctx := context.Background()
	if size == 0 {
		size = 1600
	}

	source := io.LimitReader(rand.New(rand.NewSource(int64(rseed))), int64(size))

	file, err := os.CreateTemp(t.TempDir(), "sourcefile.dat")
	require.NoError(t, err)

	n, err := io.Copy(file, source)
	require.NoError(t, err)
	require.EqualValues(t, n, size)

	//
	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	root := writeUnixfsDAG(ctx, t, file, dagSvc)

	// create a CARv1 file from the DAG
	tmp, err := os.CreateTemp(t.TempDir(), "randcarv1")
	require.NoError(t, err)
	require.NoError(t, car.WriteCar(ctx, dagSvc, []cid.Cid{root}, tmp))
	_, err = tmp.Seek(0, io.SeekStart)
	require.NoError(t, err)
	hd, _, err := car.ReadHeader(bufio.NewReader(tmp))
	require.NoError(t, err)
	require.EqualValues(t, 1, hd.Version)
	require.Len(t, hd.Roots, 1)
	require.NoError(t, tmp.Close())

	return tmp.Name(), file.Name()
}

func writeUnixfsDAG(ctx context.Context, t *testing.T, rd io.Reader, dag ipldformat.DAGService) cid.Cid {
	rpf := files.NewReaderFile(rd)

	// generate the dag and get the root
	// import to UnixFS
	prefix, err := merkledag.PrefixForCidVersion(1)
	require.NoError(t, err)
	prefix.MhType = defaultHashFunction

	bufferedDS := ipldformat.NewBufferedDAG(ctx, dag)
	params := ihelper.DagBuilderParams{
		Maxlinks:  1024,
		RawLeaves: true,
		CidBuilder: cidutil.InlineBuilder{
			Builder: prefix,
			Limit:   126,
		},
		Dagserv: bufferedDS,
	}

	db, err := params.New(chunk.NewSizeSplitter(rpf, int64(unixfsChunkSize)))
	require.NoError(t, err)

	nd, err := balanced.Layout(db)
	require.NoError(t, err)
	require.NotEqualValues(t, cid.Undef, nd.Cid())

	err = bufferedDS.Commit()
	require.NoError(t, err)
	require.NoError(t, rpf.Close())
	return nd.Cid()
}

// AssertFilesEqual compares two files by blake2b hash equality and
// fails the test if unequal.
func AssertFilesEqual(t *testing.T, left, right string) {
	// initialize hashes.
	leftH, rightH := blake2b.New256(), blake2b.New256()

	// open files.
	leftF, err := os.Open(left)
	require.NoError(t, err)

	rightF, err := os.Open(right)
	require.NoError(t, err)

	// feed hash functions.
	_, err = io.Copy(leftH, leftF)
	require.NoError(t, err)

	_, err = io.Copy(rightH, rightF)
	require.NoError(t, err)

	// compute digests.
	leftD, rightD := leftH.Sum(nil), rightH.Sum(nil)

	require.True(t, bytes.Equal(leftD, rightD))
}
