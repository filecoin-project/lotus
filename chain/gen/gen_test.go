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

	for i := 0; i < n; i++ {
		fmt.Println("LOOP: ", i)
		mts, err := g.NextTipSet()
		if err != nil {
			t.Fatalf("error at H:%d, %s", i, err)
		}
		if mts.TipSet.TipSet().Height() != uint64(i+len(mts.TipSet.Blocks[0].Header.Tickets)) {
			t.Fatal("wrong height")
		}
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
