package deals

import (
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"
)

type StateStore struct {
	ds datastore.Datastore
}

func (st *StateStore) Begin(i cid.Cid, state interface{}) error {
	k := datastore.NewKey(i.String())
	has, err := st.ds.Has(k)
	if err != nil {
		return err
	}
	if has {
		// TODO: uncomment after deals work
		//return xerrors.Errorf("Already tracking state for %s", i)
	}

	b, err := cbor.DumpObject(state)
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

// When this gets used anywhere else, migrate to reflect

func (st *StateStore) MutateMiner(i cid.Cid, mutator func(*MinerDeal) error) error {
	return st.mutate(i, minerMutator(mutator))
}

func minerMutator(m func(*MinerDeal) error) func([]byte) ([]byte, error) {
	return func(in []byte) ([]byte, error) {
		var deal MinerDeal
		err := cbor.DecodeInto(in, &deal)
		if err != nil {
			return nil, err
		}

		if err := m(&deal); err != nil {
			return nil, err
		}

		return cbor.DumpObject(deal)
	}
}

func (st *StateStore) MutateClient(i cid.Cid, mutator func(*ClientDeal) error) error {
	return st.mutate(i, clientMutator(mutator))
}

func clientMutator(m func(*ClientDeal) error) func([]byte) ([]byte, error) {
	return func(in []byte) ([]byte, error) {
		var deal ClientDeal
		err := cbor.DecodeInto(in, &deal)
		if err != nil {
			return nil, err
		}

		if err := m(&deal); err != nil {
			return nil, err
		}

		return cbor.DumpObject(deal)
	}
}

func (st *StateStore) mutate(i cid.Cid, mutator func([]byte) ([]byte, error)) error {
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
