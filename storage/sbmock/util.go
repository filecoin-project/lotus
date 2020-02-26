package sbmock

import (
	"crypto/rand"
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
