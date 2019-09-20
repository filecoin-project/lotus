package gen

import (
	"fmt"
	"testing"
)

func testGeneration(t testing.TB, n int, msgs int) {
	g, err := NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	g.msgsPerBlock = msgs

	var height int
	for i := 0; i < n; i++ {
		mts, err := g.NextTipSet()
		if err != nil {
			t.Fatalf("error at H:%d, %s", i, err)
		}

		ts := mts.TipSet.TipSet()
		if ts.Height() != uint64(height+len(ts.Blocks()[0].Tickets)) {
			t.Fatal("wrong height", ts.Height(), i, len(ts.Blocks()[0].Tickets), len(ts.Blocks()))
		}
		height += len(ts.Blocks()[0].Tickets)
	}
}

func TestChainGeneration(t *testing.T) {
	testGeneration(t, 10, 20)
}

func BenchmarkChainGeneration(b *testing.B) {
	nMessages := []int{0, 10, 100, 1000, 10_000, 100_000}
	nEmptyBlocks := []int{1, 10, 100, 1000, 10_000, 100_000}
	n100kChainMsgs := []int{1, 10, 100, 10_000}

	for _, n := range nMessages {
		b.Run(fmt.Sprintf("%d-messages", n), func(b *testing.B) {
			testGeneration(b, b.N, n)
		})
	}

	for _, n := range nEmptyBlocks {
		b.Run(fmt.Sprintf("%d-empty-blocks", n), func(b *testing.B) {
			b.N = n
			testGeneration(b, n, 0)
		})
	}

	for _, n := range n100kChainMsgs {
		b.Run(fmt.Sprintf("%d-tx-blocks", n), func(b *testing.B) {
			b.N = n
			testGeneration(b, 100_000, n)
		})
	}

	// TODO: tx blocks for secp vs bls
	// TODO: sector commitment processing
	// TODO: paych processing

}
