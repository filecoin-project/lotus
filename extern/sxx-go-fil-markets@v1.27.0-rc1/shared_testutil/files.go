package shared_testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared_testutil/unixfs"
	"github.com/filecoin-project/go-fil-markets/stores"
)

func ThisDir(t *testing.T) string {
	_, fname, _, ok := runtime.Caller(1)
	require.True(t, ok)
	return filepath.Dir(fname)
}

// CreateDenseCARv2 generates a "dense" UnixFS CARv2 from the supplied ordinary file.
// A dense UnixFS CARv2 is one storing leaf data. Contrast to CreateRefCARv2.
func CreateDenseCARv2(t *testing.T, src string) (root cid.Cid, path string) {
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	root = unixfs.WriteUnixfsDAGTo(t, src, dagSvc)

	// Create a UnixFS DAG again AND generate a CARv2 file using a CARv2
	// read-write blockstore now that we have the root.
	out, err := os.CreateTemp("", "rand")
	require.NoError(t, err)
	require.NoError(t, out.Close())

	t.Cleanup(func() { os.Remove(out.Name()) })

	rw, err := blockstore.OpenReadWrite(out.Name(), []cid.Cid{root}, blockstore.UseWholeCIDs(true))
	require.NoError(t, err)

	dagSvc = merkledag.NewDAGService(blockservice.New(rw, offline.Exchange(rw)))

	root2 := unixfs.WriteUnixfsDAGTo(t, src, dagSvc)
	require.NoError(t, rw.Finalize())
	require.Equal(t, root, root2)

	return root, out.Name()
}

// CreateRefCARv2 generates a "ref" CARv2 from the supplied ordinary file.
// A "ref" CARv2 is one that stores leaf data as positional references to the original file.
func CreateRefCARv2(t *testing.T, src string) (cid.Cid, string) {
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))

	root := unixfs.WriteUnixfsDAGTo(t, src, dagSvc)
	path := genRefCARv2(t, src, root)

	return root, path
}

func genRefCARv2(t *testing.T, path string, root cid.Cid) string {
	tmp, err := os.CreateTemp("", "rand")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	fs, err := stores.ReadWriteFilestore(tmp.Name(), root)
	require.NoError(t, err)

	dagSvc := merkledag.NewDAGService(blockservice.New(fs, offline.Exchange(fs)))

	root2 := unixfs.WriteUnixfsDAGTo(t, path, dagSvc)
	require.NoError(t, fs.Close())
	require.Equal(t, root, root2)

	// return the path of the CARv2 file.
	return tmp.Name()
}
