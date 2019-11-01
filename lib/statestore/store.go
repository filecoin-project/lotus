package statestore

import (
	"bytes"
	"fmt"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"golang.org/x/xerrors"
	"reflect"
)

type StateStore struct {
	ds datastore.Datastore
}

func New(ds datastore.Datastore) *StateStore {
	return &StateStore{ds: ds}
}

func toKey(k interface{}) datastore.Key {
	switch t := k.(type) {
	case uint64:
		return datastore.NewKey(fmt.Sprint(t))
	case fmt.Stringer:
		return datastore.NewKey(t.String())
	default:
		panic("unexpected key type")
	}
}

func (st *StateStore) Begin(i interface{}, state interface{}) error {
	k := toKey(i)
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

func (st *StateStore) End(i interface{}) error {
	k := toKey(i)
	has, err := st.ds.Has(k)
	if err != nil {
		return err
	}
	if !has {
		return xerrors.Errorf("No state for %s", i)
	}
	return st.ds.Delete(k)
}

func cborMutator(mutator interface{}) func([]byte) ([]byte, error) {
	rmut := reflect.ValueOf(mutator)

	return func(in []byte) ([]byte, error) {
		state := reflect.New(rmut.Type().In(0).Elem())

		err := cborrpc.ReadCborRPC(bytes.NewReader(in), state.Interface())
		if err != nil {
			return nil, err
		}

		out := rmut.Call([]reflect.Value{state})

		if err := out[0].Interface().(error); err != nil {
			return nil, err
		}

		return cborrpc.Dump(state.Interface())
	}
}

// mutator func(*T) error
func (st *StateStore) Mutate(i fmt.Stringer, mutator interface{}) error {
	return st.mutate(i, cborMutator(mutator))
}

func (st *StateStore) mutate(i interface{}, mutator func([]byte) ([]byte, error)) error {
	k := toKey(i)
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

// out: *[]T
func (st *StateStore) List(out interface{}) error {
	res, err := st.ds.Query(query.Query{})
	if err != nil {
		return err
	}
	defer res.Close()

	outT := reflect.TypeOf(out).Elem().Elem()
	rout := reflect.ValueOf(out)

	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}
		if res.Error != nil {
			return res.Error
		}

		elem := reflect.New(outT)
		err := cborrpc.ReadCborRPC(bytes.NewReader(res.Value), elem.Interface())
		if err != nil {
			return err
		}

		rout.Set(reflect.Append(rout.Elem(), elem.Elem()))
	}

	return nil
}
