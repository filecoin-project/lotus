package repo

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ipfs/go-datastore"
	levelds "github.com/ipfs/go-ds-leveldb"
	measure "github.com/ipfs/go-ds-measure"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
	"golang.org/x/xerrors"
)

type dsCtor func(path string, readonly bool) (datastore.Batching, error)

var fsDatastores = map[string]dsCtor{
	"metadata": levelDs,
	"f3":       levelDs,
}

// Helper badgerDs() and its imports are unused
// Leaving here for completeness
//
/*

dgbadger "github.com/dgraph-io/badger/v2"
badger "github.com/ipfs/go-ds-badger2"

func badgerDs(path string, readonly bool) (datastore.Batching, error) {
	opts := badger.DefaultOptions
	opts.ReadOnly = readonly

	opts.Options = dgbadger.DefaultOptions("").WithTruncate(true).
		WithValueThreshold(1 << 10)
	return badger.NewDatastore(path, &opts)
}

*/

func levelDs(path string, readonly bool) (datastore.Batching, error) {
	return levelds.NewDatastore(path, &levelds.Options{
		Compression: ldbopts.NoCompression,
		NoSync:      false,
		Strict:      ldbopts.StrictAll,
		ReadOnly:    readonly,
	})
}

func (fsr *fsLockedRepo) openDatastores(readonly bool) (map[string]datastore.Batching, error) {
	if err := os.MkdirAll(fsr.join(fsDatastore), 0755); err != nil {
		return nil, xerrors.Errorf("mkdir %s: %w", fsr.join(fsDatastore), err)
	}

	out := map[string]datastore.Batching{}

	for p, ctor := range fsDatastores {
		prefix := datastore.NewKey(p)

		// TODO: optimization: don't init datastores we don't need
		ds, err := ctor(fsr.join(filepath.Join(fsDatastore, p)), readonly)
		if err != nil {
			return nil, xerrors.Errorf("opening datastore %s: %w", prefix, err)
		}

		ds = measure.New("fsrepo."+p, ds)

		out[datastore.NewKey(p).String()] = ds
	}

	return out, nil
}

func (fsr *fsLockedRepo) Datastore(_ context.Context, ns string) (datastore.Batching, error) {
	fsr.dsOnce.Do(func() {
		fsr.ds, fsr.dsErr = fsr.openDatastores(fsr.readonly)
	})

	if fsr.dsErr != nil {
		return nil, fsr.dsErr
	}
	ds, ok := fsr.ds[ns]
	if ok {
		return ds, nil
	}
	return nil, xerrors.Errorf("no such datastore: %s", ns)
}
