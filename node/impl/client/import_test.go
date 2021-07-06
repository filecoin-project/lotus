package client

import (
	"bufio"
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"testing"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/node/repo/importmgr"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/stretchr/testify/require"
)

func TestImportNormalFileToUnixfsDAG(t *testing.T) {
	ctx := context.Background()
	inputPath, inputContents := genNormalInputFile(t)
	defer os.Remove(inputPath) //nolint:errcheck
	carv2File := genTmpFile(t)
	defer os.Remove(carv2File) //nolint:errcheck

	// import a normal file to a Unixfs DAG using a CARv2 read-write blockstore and flush it out to a CARv2 file.
	tempCARv2Store, err := blockstore.NewReadWrite(carv2File, []cid.Cid{})
	require.NoError(t, err)
	bsvc := blockservice.New(tempCARv2Store, offline.Exchange(tempCARv2Store))
	root, err := importNormalFileToUnixfsDAG(ctx, inputPath, merkledag.NewDAGService(bsvc))
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, root)
	require.NoError(t, tempCARv2Store.Finalize())

	// convert the CARv2 file to a normal file again and ensure the contents match.
	readOnly, err := blockstore.OpenReadOnly(carv2File)
	require.NoError(t, err)
	defer readOnly.Close() //nolint:errcheck
	dag := merkledag.NewDAGService(blockservice.New(readOnly, offline.Exchange(readOnly)))

	nd, err := dag.Get(ctx, root)
	require.NoError(t, err)
	file, err := unixfile.NewUnixfsFile(ctx, dag, nd)
	require.NoError(t, err)

	tmpOutput := genTmpFile(t)
	defer os.Remove(tmpOutput) //nolint:errcheck
	require.NoError(t, files.WriteTo(file, tmpOutput))

	// ensure contents of the initial input file and the output file are identical.

	fo, err := os.Open(tmpOutput)
	require.NoError(t, err)
	bz2, err := ioutil.ReadAll(fo)
	require.NoError(t, err)
	require.NoError(t, fo.Close())
	require.Equal(t, inputContents, bz2)
}

func TestImportNormalFileToCARv2(t *testing.T) {
	ctx := context.Background()
	a := &API{
		Imports: &importmgr.Mgr{},
	}
	importID := rand.Uint64()

	inputFilePath, inputContents := genNormalInputFile(t)
	defer os.Remove(inputFilePath) //nolint:errcheck

	outputCARv2 := genTmpFile(t)
	defer os.Remove(outputCARv2) //nolint:errcheck

	root, err := a.importNormalFileToCARv2(ctx, importID, inputFilePath, outputCARv2)
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, root)

	// convert the CARv2 to a normal file again and ensure the contents match
	readOnly, err := blockstore.OpenReadOnly(outputCARv2)
	require.NoError(t, err)
	defer readOnly.Close() //nolint:errcheck
	dag := merkledag.NewDAGService(blockservice.New(readOnly, offline.Exchange(readOnly)))

	nd, err := dag.Get(ctx, root)
	require.NoError(t, err)
	file, err := unixfile.NewUnixfsFile(ctx, dag, nd)
	require.NoError(t, err)

	tmpOutput := genTmpFile(t)
	defer os.Remove(tmpOutput) //nolint:errcheck
	require.NoError(t, files.WriteTo(file, tmpOutput))

	// ensure contents of the initial input file and the output file are identical.
	fo, err := os.Open(tmpOutput)
	require.NoError(t, err)
	bz2, err := ioutil.ReadAll(fo)
	require.NoError(t, err)
	require.NoError(t, fo.Close())
	require.Equal(t, inputContents, bz2)
}

func TestTransformCarv1ToCARv2(t *testing.T) {
	inputFilePath, _ := genNormalInputFile(t)
	defer os.Remove(inputFilePath) //nolint:errcheck

	carv1FilePath := genCARv1(t, inputFilePath)
	defer os.Remove(carv1FilePath) //nolint:errcheck

	outputCARv2 := genTmpFile(t)
	defer os.Remove(outputCARv2) //nolint:errcheck

	root, err := transformCarToCARv2(carv1FilePath, outputCARv2)
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, root)

	// assert what we got back is a valid CARv2 and that the CARv1 payload is exactly what we gave it
	f2, err := os.Open(outputCARv2)
	require.NoError(t, err)
	hd, _, err := car.ReadHeader(bufio.NewReader(f2))
	require.NoError(t, err)
	require.EqualValues(t, 2, hd.Version)
	require.NoError(t, f2.Close())

	v2r, err := carv2.NewReaderMmap(outputCARv2)
	require.NoError(t, err)
	bzout, err := ioutil.ReadAll(v2r.CarV1Reader())
	require.NoError(t, err)
	require.NotNil(t, bzout)
	require.NoError(t, v2r.Close())

	fi, err := os.Open(carv1FilePath)
	require.NoError(t, err)
	bzin, err := ioutil.ReadAll(fi)
	require.NoError(t, err)
	require.NoError(t, fi.Close())
	require.NotNil(t, bzin)

	require.Equal(t, bzin, bzout)
}

func genCARv1(t *testing.T, normalFilePath string) string {
	ctx := context.Background()
	bs := bstore.NewMemorySync()
	root, err := importNormalFileToUnixfsDAG(ctx, normalFilePath, merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs))))
	require.NoError(t, err)

	ssb := builder.NewSelectorSpecBuilder(basicnode.Prototype.Any)
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()
	sc := car.NewSelectiveCar(ctx, bs, []car.Dag{{Root: root, Selector: allSelector}})
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	require.NoError(t, sc.Write(f))

	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	hd, _, err := car.ReadHeader(bufio.NewReader(f))
	require.NoError(t, err)
	require.EqualValues(t, 1, hd.Version)

	require.NoError(t, f.Close())
	return f.Name()
}

func genTmpFile(t *testing.T) string {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func genNormalInputFile(t *testing.T) (filepath string, contents []byte) {
	s := strings.Repeat("abcde", 100)
	tmp, err := os.CreateTemp("", "")
	require.NoError(t, err)
	_, err = io.Copy(tmp, strings.NewReader(s))
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	return tmp.Name(), []byte(s)
}
