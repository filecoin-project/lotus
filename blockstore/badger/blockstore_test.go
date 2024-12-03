package badgerbs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/lotus/blockstore"
)

func TestBadgerBlockstore(t *testing.T) {
	(&Suite{
		NewBlockstore:  newBlockstore(DefaultOptions),
		OpenBlockstore: openBlockstore(DefaultOptions),
	}).RunTests(t, "non_prefixed")

	prefixed := func(path string) Options {
		opts := DefaultOptions(path)
		opts.Prefix = "/prefixed/"
		return opts
	}

	(&Suite{
		NewBlockstore:  newBlockstore(prefixed),
		OpenBlockstore: openBlockstore(prefixed),
	}).RunTests(t, "prefixed")
}

func TestStorageKey(t *testing.T) {
	bs, _ := newBlockstore(DefaultOptions)(t)
	bbs := bs.(*Blockstore)
	defer bbs.Close() //nolint:errcheck

	cid1 := blocks.NewBlock([]byte("some data")).Cid()
	cid2 := blocks.NewBlock([]byte("more data")).Cid()
	cid3 := blocks.NewBlock([]byte("a little more data")).Cid()
	require.NotEqual(t, cid1, cid2) // sanity check
	require.NotEqual(t, cid2, cid3) // sanity check

	// nil slice; let StorageKey allocate for us.
	k1 := bbs.StorageKey(nil, cid1)
	require.Len(t, k1, 55)
	require.True(t, cap(k1) == len(k1))

	// k1's backing array is reused.
	k2 := bbs.StorageKey(k1, cid2)
	require.Len(t, k2, 55)
	require.True(t, cap(k2) == len(k1))

	// bring k2 to len=0, and verify that its backing array gets reused
	// (i.e. k1 and k2 are overwritten)
	k3 := bbs.StorageKey(k2[:0], cid3)
	require.Len(t, k3, 55)
	require.True(t, cap(k3) == len(k3))

	// backing array of k1 and k2 has been modified, i.e. memory is shared.
	require.Equal(t, k3, k1)
	require.Equal(t, k3, k2)
}

func newBlockstore(optsSupplier func(path string) Options) func(tb testing.TB) (bs blockstore.BasicBlockstore, path string) {
	return func(tb testing.TB) (bs blockstore.BasicBlockstore, path string) {
		tb.Helper()

		path = tb.TempDir()

		db, err := Open(optsSupplier(path))
		if err != nil {
			tb.Fatal(err)
		}

		return db, path
	}
}

func openBlockstore(optsSupplier func(path string) Options) func(tb testing.TB, path string) (bs blockstore.BasicBlockstore, err error) {
	return func(tb testing.TB, path string) (bs blockstore.BasicBlockstore, err error) {
		tb.Helper()
		return Open(optsSupplier(path))
	}
}

func testMove(t *testing.T, optsF func(string) Options) {
	ctx := context.Background()
	basePath := t.TempDir()

	dbPath := filepath.Join(basePath, "db")

	db, err := Open(optsF(dbPath))
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close() //nolint

	var have []blocks.Block
	var deleted []cid.Cid

	// add some blocks
	for i := 0; i < 10; i++ {
		blk := blocks.NewBlock([]byte(fmt.Sprintf("some data %d", i)))
		err := db.Put(ctx, blk)
		if err != nil {
			t.Fatal(err)
		}
		have = append(have, blk)
	}

	// delete some of them
	for i := 5; i < 10; i++ {
		c := have[i].Cid()
		err := db.DeleteBlock(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		deleted = append(deleted, c)
	}
	have = have[:5]

	// start a move concurrent with some more puts
	g := new(errgroup.Group)
	g.Go(func() error {
		for i := 10; i < 1000; i++ {
			blk := blocks.NewBlock([]byte(fmt.Sprintf("some data %d", i)))
			err := db.Put(ctx, blk)
			if err != nil {
				return err
			}
			have = append(have, blk)
		}
		return nil
	})
	g.Go(func() error {
		return db.CollectGarbage(ctx, blockstore.WithFullGC(true))
	})

	err = g.Wait()
	if err != nil {
		t.Fatal(err)
	}

	// now check that we have all the blocks in have and none in the deleted lists
	checkBlocks := func() {
		for _, blk := range have {
			has, err := db.Has(ctx, blk.Cid())
			if err != nil {
				t.Fatal(err)
			}

			if !has {
				t.Fatal("missing block")
			}

			blk2, err := db.Get(ctx, blk.Cid())
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(blk.RawData(), blk2.RawData()) {
				t.Fatal("data mismatch")
			}
		}

		for _, c := range deleted {
			has, err := db.Has(ctx, c)
			if err != nil {
				t.Fatal(err)
			}

			if has {
				t.Fatal("resurrected block")
			}
		}
	}

	checkBlocks()

	// check the basePath -- it should contain a directory with name db.{timestamp}, soft-linked
	// to db and nothing else
	checkPath := func() {
		entries, err := os.ReadDir(basePath)
		if err != nil {
			t.Fatal(err)
		}

		if len(entries) != 2 {
			t.Fatalf("too many entries; expected %d but got %d", 2, len(entries))
		}

		var haveDB, haveDBLink bool
		for _, e := range entries {
			if e.Name() == "db" {
				if (e.Type() & os.ModeSymlink) == 0 {
					t.Fatal("found db, but it's not a symlink")
				}
				haveDBLink = true
				continue
			}
			if strings.HasPrefix(e.Name(), "db.") {
				if !e.Type().IsDir() {
					t.Fatal("found db prefix, but it's not a directory")
				}
				haveDB = true
				continue
			}
		}

		if !haveDB {
			t.Fatal("db directory is missing")
		}
		if !haveDBLink {
			t.Fatal("db link is missing")
		}
	}

	checkPath()

	// now do another FullGC to test the double move and following of symlinks
	if err := db.CollectGarbage(ctx, blockstore.WithFullGC(true)); err != nil {
		t.Fatal(err)
	}

	checkBlocks()
	checkPath()

	// reopen the db to make sure our relative link works:
	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	db, err = Open(optsF(dbPath))
	if err != nil {
		t.Fatal(err)
	}

	// db.Close() is already deferred

	checkBlocks()
}

func TestMoveNoPrefix(t *testing.T) {
	testMove(t, DefaultOptions)
}

func TestMoveWithPrefix(t *testing.T) {
	testMove(t, func(path string) Options {
		opts := DefaultOptions(path)
		opts.Prefix = "/prefixed/"
		return opts
	})
}
