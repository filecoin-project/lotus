package statestore

import (
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"golang.org/x/xerrors"
)

type StateStore struct {
	ds datastore.Datastore
}

func New(ds datastore.Datastore) *StateStore {
	return &StateStore{ds:ds}
}

func (st *StateStore) Begin(i cid.Cid, state interface{}) error {
	k := datastore.NewKey(i.String())
	has, err := st.ds.Has(k)
	if err != nil {
		return err
	}
	if has {
		return xerrors.Errorf("Already tracking state for %s", i)
	}

	b, err := cborrpc.Dump(state)
	if err != nil {
		return err
	}

	return st.ds.Put(k, b)
}

func (st *StateStore) End(i cid.Cid) error {
	k := datastore.NewKey(i.String())
	has, err := st.ds.Has(k)
	if err != nil {
		return err
	}
	if !has {
		return xerrors.Errorf("No state for %s", i)
	}
	return st.ds.Delete(k)
}

func (st *StateStore) Mutate(i cid.Cid, mutator func([]byte) ([]byte, error)) error {
	k := datastore.NewKey(i.String())
	has, err := st.ds.Has(k)
	if err != nil {
		return err
	}
	if !has {
		return xerrors.Errorf("No state for %s", i)
	}

	cur, err := st.ds.Get(k)
	if err != nil {
		return err
	}

	mutated, err := mutator(cur)
	if err != nil {
		return err
	}

	return st.ds.Put(k, mutated)
}

func (st *StateStore) List() ([]query.Entry, error) {
	var out []query.Entry

	res, err := st.ds.Query(query.Query{})
	if err != nil {
		return nil, err
	}
	defer res.Close()

	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}
		if res.Error != nil {
			return nil, res.Error
		}

		out = append(out, res.Entry)
	}

	return out, nil
}

