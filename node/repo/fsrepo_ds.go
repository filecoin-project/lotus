package repo

import (
	"fmt"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	badger "github.com/ipfs/go-ds-badger2"
	levelds "github.com/ipfs/go-ds-leveldb"
	"github.com/ipfs/go-ds-measure"
	ldbopts "github.com/syndtr/goleveldb/leveldb/opt"
)

type dsCtor func(path string) (datastore.Batching, error)

var fsDatastores = map[string]dsCtor{
	"chain":    badgerDs,
	"metadata": levelDs,

	// Those need to be fast for large writes... but also need a really good GC :c
	"staging": badgerDs, // miner specific
}

var fsMultiDatastores = map[string]dsCtor{
	"client": badgerDs, // client specific
}

func badgerDs(path string) (datastore.Batching, error) {
	opts := badger.DefaultOptions
	opts.Truncate = true

	return badger.NewDatastore(path, &opts)
}

func levelDs(path string) (datastore.Batching, error) {
	return levelds.NewDatastore(path, &levelds.Options{
		Compression: ldbopts.NoCompression,
	})
}

func (fsr *fsLockedRepo) openDatastores() (map[string]datastore.Batching, error) {
	if err := os.MkdirAll(fsr.join(fsDatastore), 0755); err != nil {
		return nil, xerrors.Errorf("mkdir %s: %w", fsr.join(fsDatastore), err)
	}

	out := map[string]datastore.Batching{}

	for p, ctor := range fsDatastores {
		prefix := datastore.NewKey(p)

		// TODO: optimization: don't init datastores we don't need
		ds, err := ctor(fsr.join(filepath.Join(fsDatastore, p)))
		if err != nil {
			return nil, xerrors.Errorf("opening datastore %s: %w", prefix, err)
		}

		ds = measure.New("fsrepo."+p, ds)

		out[datastore.NewKey(p).String()] = ds
	}

	return out, nil
}

func (fsr *fsLockedRepo) openMultiDatastores() (map[string]map[int64]datastore.Batching, error) {
	out := map[string]map[int64]datastore.Batching{}

	for p, ctor := range fsMultiDatastores {
		path := fsr.join(filepath.Join(fsDatastore, p))
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, xerrors.Errorf("mkdir %s: %w", path, err)
		}

		di, err := ioutil.ReadDir(path)
		if err != nil {
			return nil, xerrors.Errorf("readdir '%s': %w", path, err)
		}

		out[p] = map[int64]datastore.Batching{}

		for _, info := range di {
			path = filepath.Join(path, info.Name())

			prefix := datastore.NewKey(p)

			id, err := strconv.ParseInt(info.Name(), 10, 64)
			if err != nil {
				log.Errorf("error parsing multi-datastore id for '%s': %w", path, err)
				continue
			}

			// TODO: optimization: don't init datastores we don't need
			ds, err := ctor(path)
			if err != nil {
				return nil, xerrors.Errorf("opening datastore %s: %w", prefix, err)
			}

			ds = measure.New("fsrepo."+p+"."+info.Name(), ds)

			out[p][id] = ds
		}
	}

	return out, nil
}

func (fsr *fsLockedRepo) openMultiDatastore(ns string, idx int64) (datastore.Batching, error) {
	ctor, ok := fsMultiDatastores[ns]
	if !ok {
		return nil, xerrors.Errorf("no multi-datastore with namespace '%s'", ns)
	}

	si := fmt.Sprintf("%d", idx)
	path := fsr.join(filepath.Join(fsDatastore, ns, si))

	ds, err := ctor(path)
	if err != nil {
		return nil, xerrors.Errorf("opening datastore %s: %w", path, err)
	}

	ds = measure.New("fsrepo."+ns+"."+si, ds)

	return ds, nil
}

func (fsr *fsLockedRepo) Datastore(ns string) (datastore.Batching, error) {
	fsr.dsOnce.Do(func() {
		var err error
		fsr.ds, err = fsr.openDatastores()
		if err != nil {
			fsr.dsErr = err
			return
		}

		fsr.multiDs, fsr.dsErr = fsr.openMultiDatastores()
	})
	if fsr.dsErr != nil {
		return nil, fsr.dsErr
	}
	ds, ok := fsr.ds[ns]
	if ok {
		return ds, nil
	}

	k := datastore.NewKey(ns)
	parts := k.List()
	if len(parts) != 2 {
		return nil, xerrors.Errorf("expected multi-datastore namespace to have 2 parts")
	}

	fsr.dsLk.Lock()
	defer fsr.dsLk.Unlock()

	mds, ok := fsr.multiDs[parts[0]]
	if !ok {
		return nil, xerrors.Errorf("no multi-datastore with namespace %s", ns)
	}

	idx, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, xerrors.Errorf("parsing mult-datastore index('%s'): %w", parts[1], err)
	}

	ds, ok = mds[idx]
	if !ok {
		ds, err = fsr.openMultiDatastore(parts[0], idx)
		if err != nil {
			return nil, xerrors.Errorf("opening multi-datastore: %w", err)
		}

		mds[idx] = ds
	}

	return ds, nil
}

func (fsr *fsLockedRepo) ListDatastores(ns string) ([]int64, error) {
	k := datastore.NewKey(ns)
	parts := k.List()
	if len(parts) != 1 {
		return nil, xerrors.Errorf("expected multi-datastore namespace to have 1 part")
	}

	fsr.dsLk.Lock()
	defer fsr.dsLk.Unlock()

	mds, ok := fsr.multiDs[parts[0]]
	if !ok {
		return nil, xerrors.Errorf("no multi-datastore with namespace %s", ns)
	}

	out := make([]int64, 0, len(mds))
	for i := range mds {
		out = append(out, i)
	}

	return out, nil
}

func (fsr *fsLockedRepo) DeleteDatastore(ns string) error {
	k := datastore.NewKey(ns)
	parts := k.List()
	if len(parts) != 2 {
		return xerrors.Errorf("expected multi-datastore namespace to have 2 parts")
	}

	mds, ok := fsr.multiDs[parts[0]]
	if !ok {
		return xerrors.Errorf("no multi-datastore with namespace %s", ns)
	}

	idx, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return xerrors.Errorf("parsing mult-datastore index('%s'): %w", parts[1], err)
	}

	fsr.dsLk.Lock()
	defer fsr.dsLk.Unlock()

	ds, ok := mds[idx]
	if !ok {
		return xerrors.Errorf("no multi-datastore with at index (namespace %s)", ns)
	}

	if err := ds.Close(); err != nil {
		return xerrors.Errorf("closing datastore: %w", err)
	}

	path := fsr.join(filepath.Join(fsDatastore, parts[0], parts[1]))

	log.Warnw("removing sub-datastore", "path", path, "namespace", ns)
	if err := os.RemoveAll(path); err != nil {
		return xerrors.Errorf("remove '%s': %w", path, err)
	}

	return nil
}