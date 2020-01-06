package statestore

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/filecoin-project/go-cbor-util"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"
)

type StateStore struct {
	ds datastore.Datastore
}

func New(ds datastore.Datastore) *StateStore {
	return &StateStore{ds: ds}
}

func ToKey(k interface{}) datastore.Key {
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
	k := ToKey(i)
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

func (st *StateStore) Get(i interface{}) *StoredState {
	return &StoredState{
		ds:   st.ds,
		name: ToKey(i),
	}
}

func (st *StateStore) Has(i interface{}) (bool, error) {
	return st.ds.Has(ToKey(i))
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
