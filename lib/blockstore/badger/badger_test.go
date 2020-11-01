package badgerbs

import (
	"io/ioutil"
	"os"
	"testing"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
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

func newBlockstore(optsSupplier func(path string) Options) func(tb testing.TB) (bs blockstore.Blockstore, path string) {
	return func(tb testing.TB) (bs blockstore.Blockstore, path string) {
		tb.Helper()

		path, err := ioutil.TempDir("", "")
		if err != nil {
			tb.Fatal(err)
		}

		db, err := Open(optsSupplier(path))
		if err != nil {
			tb.Fatal(err)
		}

		tb.Cleanup(func() {
			_ = os.RemoveAll(path)
		})

		return db, path
	}
}

func openBlockstore(optsSupplier func(path string) Options) func(tb testing.TB, path string) (bs blockstore.Blockstore, err error) {
	return func(tb testing.TB, path string) (bs blockstore.Blockstore, err error) {
		tb.Helper()
		return Open(optsSupplier(path))
	}
}
