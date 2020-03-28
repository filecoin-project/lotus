package bls

import (
	"crypto/rand"
	"github.com/filecoin-project/go-address"
	"testing"
)

func Benchmark_blsSigner_Sign(b *testing.B) {
	signer := blsSigner{}
	b.ResetTimer()
	for i:=0;i<b.N;i++{
		b.StopTimer()
		pk, err := signer.GenPrivate()
		if err != nil {
			b.Error(err)
		}
		randMsg := make([]byte, 32)
		rand.Read(randMsg)

		b.StartTimer()
		_, err = signer.Sign(pk, randMsg)
		if err != nil {
			b.Error(err)
		}
	}
}

func Benchmark_blsSigner_Verify(b *testing.B) {
	signer := blsSigner{}
	b.ResetTimer()
	for i:=0;i<b.N;i++{
		b.StopTimer()
		priv, err := signer.GenPrivate()
		if err != nil {
			b.Error(err)
		}
		randMsg := make([]byte, 32)
		rand.Read(randMsg)
		sig, err := signer.Sign(priv, randMsg)
		if err != nil {
			b.Error(err)
		}
		pk, err := signer.ToPublic(priv)
		if err != nil {
			b.Error(err)
		}
		addr, err  := address.NewBLSAddress(pk)
		if err != nil {
			b.Error(err)
		}
		b.StartTimer()
		err = signer.Verify(sig, addr, randMsg)
		if err != nil {
			b.Error(err)
		}
	}
}