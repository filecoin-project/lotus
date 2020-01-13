package sbmock

import (
	"crypto/rand"
	"crypto/sha256"
	"io"
	"io/ioutil"
)

func randB(n uint64) []byte {
	b, err := ioutil.ReadAll(io.LimitReader(rand.Reader, int64(n)))
	if err != nil {
		panic(err)
	}
	return b
}

func commDR(in []byte) (out [32]byte) {
	for i, b := range in {
		out[i] = ^b
	}

	return out
}

func commD(b []byte) [32]byte {
	return sha256.Sum256(b)
}
