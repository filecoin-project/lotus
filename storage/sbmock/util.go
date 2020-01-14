package sbmock

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"

	"github.com/xjrwfilecoin/go-sectorbuilder"
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
	c, err := sectorbuilder.GeneratePieceCommitment(bytes.NewReader(b), uint64(len(b)))
	if err != nil {
		panic(err)
	}
	return c
}
