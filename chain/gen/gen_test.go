package gen

import (
	"testing"
)

func testGeneration(t testing.TB, n int) {
	g, err := NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < n; i++ {
		b, _, err := g.NextBlock()
		if err != nil {
			t.Fatalf("error at H:%d, %s", i, err)
		}
		if b.Header.Height != uint64(i+1) {
			t.Fatal("wrong height")
		}
	}
}

func TestChainGeneration(t *testing.T) {
	testGeneration(t, 10)
}

func BenchmarkChainGeneration(b *testing.B) {
	testGeneration(b, b.N)
}
