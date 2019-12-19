package statestore

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-cbor-util"
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
		return xerrors.Errorf("already tracking state for %v", i)
	}

	b, err := cborutil.Dump(state)
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

		err := cborutil.ReadCborRPC(bytes.NewReader(in), state.Interface())
		if err != nil {
			return nil, err
		}

		out := rmut.Call([]reflect.Value{state})

		if err := out[0].Interface(); err != nil {
			return nil, err.(error)
		}

		return cborutil.Dump(state.Interface())
	}
}

// mutator func(*T) error
func (st *StateStore) Mutate(i interface{}, mutator interface{}) error {
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

func (st *StateStore) Has(i interface{}) (bool, error) {
	return st.ds.Has(toKey(i))
}

func (st *StateStore) Get(i interface{}, out cbg.CBORUnmarshaler) error {
	k := toKey(i)
	val, err := st.ds.Get(k)
	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			return xerrors.Errorf("No state for %s: %w", i, err)
		}
		return err
	}

	return out.UnmarshalCBOR(bytes.NewReader(val))
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

	var errs error

	for {
		res, ok := res.NextSync()
		if !ok {
			break
		}
		if res.Error != nil {
			return res.Error
		}

		elem := reflect.New(outT)
		err := cborutil.ReadCborRPC(bytes.NewReader(res.Value), elem.Interface())
		if err != nil {
			errs = multierr.Append(errs, xerrors.Errorf("decoding state for key '%s': %w", res.Key, err))
			continue
		}

		rout.Elem().Set(reflect.Append(rout.Elem(), elem.Elem()))
	}

	return nil
}
