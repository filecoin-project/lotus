package stores

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared_testutil/unixfs"
)

func TestFilestoreRoundtrip(t *testing.T) {
	ctx := context.Background()
	normalFilePath, origBytes := createFile(t, 10, 10485760)

	// write out a unixfs dag to an inmemory store to get the root.
	root := writeUnixfsDAGInmemory(t, normalFilePath)

	// write out a unixfs dag to a file store backed by a CAR file.
	tmpCARv2, err := os.CreateTemp(t.TempDir(), "rand")
	require.NoError(t, err)
	require.NoError(t, tmpCARv2.Close())

	// writing a filestore, and then using it as as source.
	fs, err := ReadWriteFilestore(tmpCARv2.Name(), root)
	require.NoError(t, err)

	dagSvc := merkledag.NewDAGService(blockservice.New(fs, offline.Exchange(fs)))
	root2 := unixfs.WriteUnixfsDAGTo(t, normalFilePath, dagSvc)
	require.NoError(t, fs.Close())
	require.Equal(t, root, root2)

	// it works if we use a Filestore backed by the given CAR file
	fs, err = ReadOnlyFilestore(tmpCARv2.Name())
	require.NoError(t, err)

	fbz, err := dagToNormalFile(t, ctx, root, fs)
	require.NoError(t, err)
	require.NoError(t, fs.Close())

	// assert contents are equal
	require.EqualValues(t, origBytes, fbz)
}

func TestReadOnlyFilestoreWithDenseCARFile(t *testing.T) {
	ctx := context.Background()
	normalFilePath, origContent := createFile(t, 10, 10485760)

	// write out a unixfs dag to an inmemory store to get the root.
	root := writeUnixfsDAGInmemory(t, normalFilePath)

	// write out a unixfs dag to a read-write CARv2 blockstore to get the full CARv2 file.
	tmpCARv2, err := os.CreateTemp(t.TempDir(), "rand")
	require.NoError(t, err)
	require.NoError(t, tmpCARv2.Close())

	bs, err := blockstore.OpenReadWrite(tmpCARv2.Name(), []cid.Cid{root})
	require.NoError(t, err)

	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
	root2 := unixfs.WriteUnixfsDAGTo(t, normalFilePath, dagSvc)
	require.NoError(t, bs.Finalize())
	require.Equal(t, root, root2)

	// Open a read only filestore with the full CARv2 file
	fs, err := ReadOnlyFilestore(tmpCARv2.Name())
	require.NoError(t, err)

	// write out the normal file using the Filestore and assert the contents match.
	finalBytes, err := dagToNormalFile(t, ctx, root, fs)
	require.NoError(t, err)
	require.NoError(t, fs.Close())

	require.EqualValues(t, origContent, finalBytes)
}

func dagToNormalFile(t *testing.T, ctx context.Context, root cid.Cid, bs bstore.Blockstore) ([]byte, error) {
	outputName := filepath.Join(t.TempDir(), "rand"+strconv.Itoa(int(rand.Uint32())))

	bsvc := blockservice.New(bs, offline.Exchange(bs))
	dag := merkledag.NewDAGService(bsvc)
	nd, err := dag.Get(ctx, root)
	if err != nil {
		return nil, err
	}

	file, err := unixfile.NewUnixfsFile(ctx, dag, nd)
	if err != nil {
		return nil, err
	}
	if err := files.WriteTo(file, outputName); err != nil {
		return nil, err
	}

	outputF, err := os.Open(outputName)
	if err != nil {
		return nil, err
	}

	finalBytes, err := ioutil.ReadAll(outputF)
	if err != nil {
		return nil, err
	}

	if err := outputF.Close(); err != nil {
		return nil, err
	}

	return finalBytes, nil
}

func createFile(t *testing.T, rseed int64, size int64) (path string, contents []byte) {
	source := io.LimitReader(rand.New(rand.NewSource(rseed)), size)

	file, err := os.CreateTemp(t.TempDir(), "sourcefile.dat")
	require.NoError(t, err)

	n, err := io.Copy(file, source)
	require.NoError(t, err)
	require.EqualValues(t, n, size)

	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)
	bz, err := ioutil.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	return file.Name(), bz
}

func writeUnixfsDAGInmemory(t *testing.T, path string) cid.Cid {
	bs := bstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	dagSvc := merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
	root := unixfs.WriteUnixfsDAGTo(t, path, dagSvc)
	return root
}
