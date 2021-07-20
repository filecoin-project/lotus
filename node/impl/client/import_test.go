package client

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/filecoin-project/go-fil-markets/filestorecaradapter"
	"github.com/filecoin-project/lotus/node/repo/importmgr"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"
)

func TestImportNormalFileToUnixfsDAG(t *testing.T) {
	ctx := context.Background()
	inputPath, inputContents := genNormalInputFile(t)
	defer os.Remove(inputPath) //nolint:errcheck
	carv2File := genTmpFile(t)
	defer os.Remove(carv2File) //nolint:errcheck

	// import a normal file to a Unixfs DAG using a CARv2 read-write blockstore and flush it out to a CARv2 file.
	tempCARv2Store, err := blockstore.OpenReadWrite(carv2File, []cid.Cid{})
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
	importID := importmgr.ImportID(rand.Uint64())

	inputFilePath, inputContents := genNormalInputFile(t)
	defer os.Remove(inputFilePath) //nolint:errcheck

	outputCARv2 := genTmpFile(t)
	defer os.Remove(outputCARv2) //nolint:errcheck

	root, err := a.importNormalFileToFilestoreCARv2(ctx, importID, inputFilePath, outputCARv2)
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, root)

	// convert the CARv2 to a normal file again and ensure the contents match
	readOnly, err := filestorecaradapter.NewReadOnlyFileStore(outputCARv2)
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
