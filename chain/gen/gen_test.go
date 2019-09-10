package gen

import (
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
	b.Run("0-messages", func(b *testing.B) {
		testGeneration(b, b.N, 0)
	})

	b.Run("10-messages", func(b *testing.B) {
		testGeneration(b, b.N, 10)
	})

	b.Run("100-messages", func(b *testing.B) {
		testGeneration(b, b.N, 100)
	})

	b.Run("1000-messages", func(b *testing.B) {
		testGeneration(b, b.N, 1000)
	})
}
