package client

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/stores"

	"github.com/filecoin-project/lotus/node/repo/imports"
)

// This test uses a full "dense" CARv2, and not a filestore (positional mapping).
func TestRoundtripUnixFS_Dense(t *testing.T) {
	ctx := context.Background()

	inputPath, inputContents := genInputFile(t)
	defer os.Remove(inputPath) //nolint:errcheck

	carv2File := newTmpFile(t)
	defer os.Remove(carv2File) //nolint:errcheck

	// import a file to a Unixfs DAG using a CARv2 read/write blockstore.
	bs, err := blockstore.OpenReadWrite(carv2File, nil,
		carv2.ZeroLengthSectionAsEOF(true),
		blockstore.UseWholeCIDs(true))
	require.NoError(t, err)

	root, err := buildUnixFS(ctx, bytes.NewBuffer(inputContents), bs, false)
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, root)
	require.NoError(t, bs.Finalize())

	// reconstruct the file.
	readOnly, err := blockstore.OpenReadOnly(carv2File,
		carv2.ZeroLengthSectionAsEOF(true),
		blockstore.UseWholeCIDs(true))
	require.NoError(t, err)
	defer readOnly.Close() //nolint:errcheck

	dags := merkledag.NewDAGService(blockservice.New(readOnly, offline.Exchange(readOnly)))

	nd, err := dags.Get(ctx, root)
	require.NoError(t, err)

	file, err := unixfile.NewUnixfsFile(ctx, dags, nd)
	require.NoError(t, err)

	tmpOutput := newTmpFile(t)
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

func TestRoundtripUnixFS_Filestore(t *testing.T) {
	ctx := context.Background()
	a := &API{
		Imports: &imports.Manager{},
	}

	inputPath, inputContents := genInputFile(t)
	defer os.Remove(inputPath) //nolint:errcheck

	dst := newTmpFile(t)
	defer os.Remove(dst) //nolint:errcheck

	root, err := a.createUnixFSFilestore(ctx, inputPath, dst)
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, root)

	// convert the CARv2 to a normal file again and ensure the contents match
	fs, err := stores.ReadOnlyFilestore(dst)
	require.NoError(t, err)
	defer fs.Close() //nolint:errcheck

	dags := merkledag.NewDAGService(blockservice.New(fs, offline.Exchange(fs)))

	nd, err := dags.Get(ctx, root)
	require.NoError(t, err)

	file, err := unixfile.NewUnixfsFile(ctx, dags, nd)
	require.NoError(t, err)

	tmpOutput := newTmpFile(t)
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

func newTmpFile(t *testing.T) string {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func genInputFile(t *testing.T) (filepath string, contents []byte) {
	s := strings.Repeat("abcde", 100)
	tmp, err := os.CreateTemp("", "")
	require.NoError(t, err)
	_, err = io.Copy(tmp, strings.NewReader(s))
	require.NoError(t, err)
	require.NoError(t, tmp.Close())
	return tmp.Name(), []byte(s)
}
