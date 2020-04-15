package drand

import (
	"fmt"
	"testing"
)

func TestPrintDrandPubkey(t *testing.T) {
	bc, err := NewDrandBeacon(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Drand Pubkey:\n%#v\n", bc.pubkey.TOML())
}
