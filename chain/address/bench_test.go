package address

import (
	"fmt"
	"math/rand"

	"testing"
)

func blsaddr(n int64) Address {
	buf := make([]byte, 48)
	r := rand.New(rand.NewSource(n))
	r.Read(buf)

	addr, err := NewBLSAddress(buf)
	if err != nil {
		panic(err)
	}

	return addr
}

func makeActorAddresses(n int) [][]byte {
	var addrs [][]byte
	for i := 0; i < n; i++ {
		a, err := NewActorAddress([]byte(fmt.Sprintf("ACTOR ADDRESS %d", i)))
		if err != nil {
			panic(err)
		}
		addrs = append(addrs, a.Bytes())
	}

	return addrs
}

func makeBlsAddresses(n int64) [][]byte {
	var addrs [][]byte
	for i := int64(0); i < n; i++ {
		addrs = append(addrs, blsaddr(n).Bytes())
	}
	return addrs
}

func makeSecpAddresses(n int) [][]byte {
	var addrs [][]byte
	for i := 0; i < n; i++ {
		r := rand.New(rand.NewSource(int64(i)))
		buf := make([]byte, 32)
		r.Read(buf)

		a, err := NewSecp256k1Address(buf)
		if err != nil {
			panic(err)
		}

		addrs = append(addrs, a.Bytes())
	}
	return addrs
}

func makeIDAddresses(n int) [][]byte {
	var addrs [][]byte
	for i := 0; i < n; i++ {

		a, err := NewIDAddress(uint64(i))
		if err != nil {
			panic(err)
		}

		addrs = append(addrs, a.Bytes())
	}
	return addrs
}

func BenchmarkParseActorAddress(b *testing.B) {
	benchTestWithAddrs := func(a [][]byte) func(b *testing.B) {
		return func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := NewFromBytes(a[i%len(a)])
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	b.Run("actor", benchTestWithAddrs(makeActorAddresses(20)))
	b.Run("bls", benchTestWithAddrs(makeBlsAddresses(20)))
	b.Run("secp256k1", benchTestWithAddrs(makeSecpAddresses(20)))
	b.Run("id", benchTestWithAddrs(makeIDAddresses(20)))
}
