package vdf

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

func Run(input []byte) ([]byte, []byte, error) {
	h := sha256.Sum256(input)
	// TODO: THIS IS A FAKE VDF. THE SPEC IS UNCLEAR ON WHAT TO REALLY DO HERE
	return h[:], []byte("proof"), nil
}

func Verify(input []byte, out []byte, proof []byte) error {
	// this is a fake VDF
	h := sha256.Sum256(input)

	if !bytes.Equal(h[:], out) {
		return fmt.Errorf("vdf output incorrect")
	}

	if !bytes.Equal(proof, []byte("proof")) {
		return fmt.Errorf("vdf proof failed to validate")
	}

	return nil
}
