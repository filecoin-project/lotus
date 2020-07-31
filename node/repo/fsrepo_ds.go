package repo

import (
	"os"
	"path/filepath"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/mount"
	"github.com/ipfs/go-datastore/namespace"
	"golang.org/x/xerrors"

	dgbadger "github.com/dgraph-io/badger/v2"
	badger "github.com/ipfs/go-ds-badger2"
	levelds "github.com/ipfs/go-ds-leveldb"
	measure "github.com/ipfs/go-ds-measure"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
)

var fsDatastores = map[string]func(path string) (datastore.Batching, error){
	"chain":    badgerDs,
	"metadata": levelDs,

	// Those need to be fast for large writes... but also need a really good GC :c
	"staging": badgerDs, // miner specific
	"client":  badgerDs, // client specific
}

func badgerDs(path string) (datastore.Batching, error) {
	opts := badger.DefaultOptions
	opts.Options = dgbadger.DefaultOptions("").WithTruncate(true).
		WithValueThreshold(1 << 10)

	return badger.NewDatastore(path, &opts)
}

func levelDs(path string) (datastore.Batching, error) {
	return levelds.NewDatastore(path, &levelds.Options{
		Compression: ldbopts.NoCompression,
	})
}

func (fsr *fsLockedRepo) openDatastore() (datastore.Batching, error) {
	if err := os.MkdirAll(fsr.join(fsDatastore), 0755); err != nil {
		return nil, xerrors.Errorf("mkdir %s: %w", fsr.join(fsDatastore), err)
	}

	var mounts []mount.Mount

	for p, ctor := range fsDatastores {
		prefix := datastore.NewKey(p)

		// TODO: optimization: don't init datastores we don't need
		ds, err := ctor(fsr.join(filepath.Join(fsDatastore, p)))
		if err != nil {
			return nil, xerrors.Errorf("opening datastore %s: %w", prefix, err)
		}

		ds = measure.New("fsrepo."+p, ds)

		mounts = append(mounts, mount.Mount{
			Prefix:    prefix,
			Datastore: ds,
		})
	}

	return mount.New(mounts), nil
}

func (fsr *fsLockedRepo) Datastore(ns string) (datastore.Batching, error) {
	fsr.dsOnce.Do(func() {
		fsr.ds, fsr.dsErr = fsr.openDatastore()
	})
	if fsr.dsErr != nil {
		return nil, fsr.dsErr
	}
	return namespace.Wrap(fsr.ds, datastore.NewKey(ns)), nil
}
