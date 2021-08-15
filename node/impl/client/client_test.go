package client

import (
	"bytes"
	"context"
	"embed"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/repo/imports"
)

//go:embed testdata/*
var testdata embed.FS

func TestImportLocal(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	dir := t.TempDir()
	im := imports.NewManager(ds, dir)
	ctx := context.Background()

	a := &API{Imports: im}

	b, err := testdata.ReadFile("testdata/payload.txt")
	require.NoError(t, err)

	root, err := a.ClientImportLocal(ctx, bytes.NewReader(b))
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, root)

	list, err := a.ClientListImports(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	it := list[0]
	require.Equal(t, root, *it.Root)
	require.True(t, strings.HasPrefix(it.CARPath, dir))

	local, err := a.ClientHasLocal(ctx, root)
	require.NoError(t, err)
	require.True(t, local)

	order := api.RetrievalOrder{
		Root:         root,
		FromLocalCAR: it.CARPath,
	}

	// retrieve as UnixFS.
	out1 := filepath.Join(dir, "retrieval1.data")
	// out2 := filepath.Join(dir, "retrieval2.data")
	err = a.ClientRetrieve(ctx, order, &api.FileRef{
		Path: out1,
	})
	require.NoError(t, err)

	outBytes, err := ioutil.ReadFile(out1)
	require.NoError(t, err)
	require.Equal(t, b, outBytes)
}
