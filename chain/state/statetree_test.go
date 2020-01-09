package state

import (
	"testing"

	address "github.com/filecoin-project/go-address"
	actors "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	hamt "github.com/ipfs/go-hamt-ipld"
)

func BenchmarkStateTreeSet(b *testing.B) {
	cst := hamt.NewCborStore()
	st, err := NewStateTree(cst)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		a, err := address.NewIDAddress(uint64(i))
		if err != nil {
			b.Fatal(err)
		}
		err = st.SetActor(a, &types.Actor{
			Balance: types.NewInt(1258812523),
			Code:    actors.StorageMinerCodeCid,
			Head:    actors.AccountCodeCid,
			Nonce:   uint64(i),
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateTreeSetFlush(b *testing.B) {
	cst := hamt.NewCborStore()
	st, err := NewStateTree(cst)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		a, err := address.NewIDAddress(uint64(i))
		if err != nil {
			b.Fatal(err)
		}
		err = st.SetActor(a, &types.Actor{
			Balance: types.NewInt(1258812523),
			Code:    actors.StorageMinerCodeCid,
			Head:    actors.AccountCodeCid,
			Nonce:   uint64(i),
		})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := st.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateTree10kGetActor(b *testing.B) {
	cst := hamt.NewCborStore()
	st, err := NewStateTree(cst)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		a, err := address.NewIDAddress(uint64(i))
		if err != nil {
			b.Fatal(err)
		}
		err = st.SetActor(a, &types.Actor{
			Balance: types.NewInt(1258812523 + uint64(i)),
			Code:    actors.StorageMinerCodeCid,
			Head:    actors.AccountCodeCid,
			Nonce:   uint64(i),
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	if _, err := st.Flush(); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		a, err := address.NewIDAddress(uint64(i % 10000))
		if err != nil {
			b.Fatal(err)
		}

		_, err = st.GetActor(a)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestSetCache(t *testing.T) {
	cst := hamt.NewCborStore()
	st, err := NewStateTree(cst)
	if err != nil {
		t.Fatal(err)
	}

	a, err := address.NewIDAddress(uint64(222))
	if err != nil {
		t.Fatal(err)
	}

	act := &types.Actor{
		Balance: types.NewInt(0),
		Code:    actors.StorageMinerCodeCid,
		Head:    actors.AccountCodeCid,
		Nonce:   0,
	}

	err = st.SetActor(a, act)
	if err != nil {
		t.Fatal(err)
	}

	act.Nonce = 1

	outact, err := st.GetActor(a)
	if err != nil {
		t.Fatal(err)
	}

	if outact.Nonce != act.Nonce {
		t.Error("nonce didn't match")
	}
}
