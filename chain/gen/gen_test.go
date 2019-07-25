package gen

import (
	"testing"
)

func TestChainGeneration(t *testing.T) {
	g, err := NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		b, err := g.NextBlock()
		if err != nil {
			t.Fatal(err)
		}
		if b.Header.Height != uint64(i+1) {
			t.Fatal("wrong height")
		}
	}

}
