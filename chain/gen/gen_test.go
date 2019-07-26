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
			t.Fatalf("error at H:%d, %s", i, err)
		}
		if b.Header.Height != uint64(i+1) {
			t.Fatal("wrong height")
		}
	}

}
