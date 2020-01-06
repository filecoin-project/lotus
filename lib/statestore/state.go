package statestore

import (
	"bytes"
	"reflect"

	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/ipfs/go-datastore"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

type StoredState struct {
	ds   datastore.Datastore
	name datastore.Key
}

func (st *StoredState) End() error {
	has, err := st.ds.Has(st.name)
	if err != nil {
		return err
	}
	if !has {
		return xerrors.Errorf("No state for %s", st.name)
	}
	if err := st.ds.Delete(st.name); err != nil {
		return xerrors.Errorf("removing state from datastore: %w", err)
	}
	st.name = datastore.Key{}
	st.ds = nil

	return nil
}

func (st *StoredState) Get(out cbg.CBORUnmarshaler) error {
	val, err := st.ds.Get(st.name)
	if err != nil {
		if xerrors.Is(err, datastore.ErrNotFound) {
			return xerrors.Errorf("No state for %s: %w", st.name, err)
		}
		return err
	}

	return out.UnmarshalCBOR(bytes.NewReader(val))
}

// mutator func(*T) error
func (st *StoredState) Mutate(mutator interface{}) error {
	return st.mutate(cborMutator(mutator))
}

func (st *StoredState) mutate(mutator func([]byte) ([]byte, error)) error {
	has, err := st.ds.Has(st.name)
	if err != nil {
		return err
	}
	if !has {
		return xerrors.Errorf("No state for %s", st.name)
	}

	cur, err := st.ds.Get(st.name)
	if err != nil {
		return err
	}

	mutated, err := mutator(cur)
	if err != nil {
		return err
	}

	return st.ds.Put(st.name, mutated)
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
