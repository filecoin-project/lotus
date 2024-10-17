package commitment

import (
	"math/big"
	"os"
	"path/filepath"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/triplewz/poseidon"
	"golang.org/x/xerrors"
)

const pauxFile = "p_aux"

func CommR(commC, commRLast [32]byte) ([32]byte, error) {
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

	cons, err := poseidon.GenPoseidonConstants[*fr.Element](3)
	if err != nil {
		return [32]byte{}, err
	}

	h1, err := poseidon.Hash(input, cons, poseidon.OptimizedStatic)
	if err != nil {
		return [32]byte{}, err
	}

	h1element := new(fr.Element).SetBigInt(h1).Bytes()

	// reverse the bytes so that endianness is correct
	for i, j := 0, len(h1element)-1; i < j; i, j = i+1, j-1 {
		h1element[i], h1element[j] = h1element[j], h1element[i]
	}

	return h1element, nil
}

// PAuxCommR reads p_aux and computes CommR
func PAuxCommR(cache string) ([32]byte, error) {
	commCcommRLast, err := os.ReadFile(filepath.Join(cache, pauxFile))
	if err != nil {
		return [32]byte{}, err
	}

	if len(commCcommRLast) != 64 {
		return [32]byte{}, xerrors.Errorf("invalid commCcommRLast length %d", len(commCcommRLast))
	}

	var commC, commRLast [32]byte
	copy(commC[:], commCcommRLast[:32])
	copy(commRLast[:], commCcommRLast[32:])

	return CommR(commC, commRLast)
}
