package state

import (
	"testing"

	actors "github.com/filecoin-project/go-lotus/chain/actors"
	address "github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
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
			Head:    actors.AccountActorCodeCid,
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
			Head:    actors.AccountActorCodeCid,
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
			Head:    actors.AccountActorCodeCid,
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
