package types

import (
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
)

func blsaddr(n int64) address.Address {
	buf := make([]byte, 48)
	r := rand.New(rand.NewSource(n))
	r.Read(buf)

	addr, err := address.NewBLSAddress(buf)
	if err != nil {
		panic(err) // ok
	}

	return addr
}

func BenchmarkSerializeMessage(b *testing.B) {
	m := &Message{
		To:       blsaddr(1),
		From:     blsaddr(2),
		Nonce:    197,
		Method:   1231254,
		Params:   []byte("some bytes, idk. probably at least ten of them"),
		GasLimit: NewInt(126723),
		GasPrice: NewInt(1776234),
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := m.Serialize()
		if err != nil {
			b.Fatal(err)
		}
	}
}
