package statestore

import (
	"testing"

	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/go-cbor-util"
)

func TestList(t *testing.T) {
	ds := datastore.NewMapDatastore()

	e, err := cborutil.Dump(types.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}

	if err := ds.Put(datastore.NewKey("/2"), e); err != nil {
		t.Fatal(err)
	}

	st := &StateStore{ds: ds}

	var out []types.BigInt
	if err := st.List(&out); err != nil {
		t.Fatal(err)
	}

	if len(out) != 1 {
		t.Fatal("wrong len")
	}

	if out[0].Int64() != 7 {
		t.Fatal("wrong data")
	}
}
