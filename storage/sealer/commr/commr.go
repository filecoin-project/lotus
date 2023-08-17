package commr

import (
	"math/big"

	"github.com/triplewz/poseidon"
	ff "github.com/triplewz/poseidon/bls12_381"
)

func CommR(commC, commRLast [32]byte) [32]byte {
	// reverse commC and commRLast so that endianness is correct
	for i, j := 0, len(commC)-1; i < j; i, j = i+1, j-1 {
		commC[i], commC[j] = commC[j], commC[i]
		commRLast[i], commRLast[j] = commRLast[j], commRLast[i]
	}

	input_a := new(big.Int)
	input_a.SetBytes(commC[:])
	input_b := new(big.Int)
	input_b.SetBytes(commRLast[:])
	input := []*big.Int{input_a, input_b}

	cons, _ := poseidon.GenPoseidonConstants(3)
	h1, _ := poseidon.Hash(input, cons, poseidon.OptimizedStatic)
	h1element := new(ff.Element).SetBigInt(h1).Bytes()

	// reverse the bytes so that endianness is correct
	for i, j := 0, len(h1element)-1; i < j; i, j = i+1, j-1 {
		h1element[i], h1element[j] = h1element[j], h1element[i]
	}

	return h1element
}
