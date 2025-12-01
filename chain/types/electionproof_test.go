package types

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xorcare/golden"
)

func TestPoissonFunction(t *testing.T) {
	tests := []struct {
		lambdaBase  uint64
		lambdaShift uint
	}{
		{10, 10},      // 0.0097
		{209714, 20},  // 0.19999885
		{1036915, 20}, // 0.9888792038
		{1706, 10},    // 1.6660
		{2, 0},        // 2
		{5242879, 20}, //4.9999990
		{5, 0},        // 5
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("lam-%d-%d", test.lambdaBase, test.lambdaShift), func(t *testing.T) {
			b := &bytes.Buffer{}
			b.WriteString("icdf\n")

			lam := new(big.Int).SetUint64(test.lambdaBase)
			lam = lam.Lsh(lam, precision-test.lambdaShift)
			p, icdf := newPoiss(lam)

			b.WriteString(icdf.String())
			b.WriteRune('\n')

			for i := 0; i < 15; i++ {
				b.WriteString(p.next().String())
				b.WriteRune('\n')
			}
			golden.Assert(t, []byte(b.String()))
		})
	}
}

func TestLambdaFunction(t *testing.T) {
	tests := []struct {
		power      string
		totalPower string
		target     float64
	}{
		{"10", "100", .1 * 5.},
		{"1024", "2048", 0.5 * 5.},
		{"2000000000000000", "100000000000000000", 0.02 * 5.},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s-%s", test.power, test.totalPower), func(t *testing.T) {
			pow, ok := new(big.Int).SetString(test.power, 10)
			assert.True(t, ok)
			total, ok := new(big.Int).SetString(test.totalPower, 10)
			assert.True(t, ok)
			lam := lambda(pow, total)
			assert.Equal(t, test.target, q256ToF(lam))
			golden.Assert(t, []byte(lam.String()))
		})
	}
}

func TestExpFunction(t *testing.T) {
	const N = 256

	step := big.NewInt(5)
	step = step.Lsh(step, 256) // Q.256
	step = step.Div(step, big.NewInt(N-1))

	x := big.NewInt(0)
	b := &bytes.Buffer{}

	b.WriteString("x, y\n")
	for i := 0; i < N; i++ {
		y := expneg(x)
		fmt.Fprintf(b, "%s,%s\n", x, y)
		x = x.Add(x, step)
	}

	golden.Assert(t, b.Bytes())
}

func q256ToF(x *big.Int) float64 {
	deno := big.NewInt(1)
	deno = deno.Lsh(deno, 256)
	rat := new(big.Rat).SetFrac(x, deno)
	f, _ := rat.Float64()
	return f
}

func TestElectionLam(t *testing.T) {
	p := big.NewInt(64)
	tot := big.NewInt(128)
	lam := lambda(p, tot)
	target := 64. * 5. / 128.
	if q256ToF(lam) != target {
		t.Fatalf("wrong lambda: %f, should be: %f", q256ToF(lam), target)
	}
}

var Res int64

func BenchmarkWinCounts(b *testing.B) {
	totalPower := NewInt(100)
	power := NewInt(100)
	ep := &ElectionProof{VRFProof: nil}
	var res int64

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		ep.VRFProof = []byte{byte(b.N), byte(b.N >> 8), byte(b.N >> 16), byte(b.N >> 24), byte(b.N >> 32)}
		j := ep.ComputeWinCount(power, totalPower)
		res += j
	}
	Res += res
}

func TestWinCounts(t *testing.T) {
	totalPower := NewInt(100)
	power := NewInt(20)

	count := uint64(1000000)
	total := uint64(0)
	ep := &ElectionProof{VRFProof: make([]byte, 5)}
	for i := uint64(0); i < count; i++ {
		w := i + count
		ep.VRFProof[0] = byte(w)
		ep.VRFProof[1] = byte(w >> 8)
		ep.VRFProof[2] = byte(w >> 16)
		ep.VRFProof[3] = byte(w >> 24)
		ep.VRFProof[4] = byte(w >> 32)

		total += uint64(ep.ComputeWinCount(power, totalPower))
	}
	// We have 1/5 of the power, so we expect to win 1 block per epoch on average. Plus or minus
	// 1%.
	avgWins := float64(total) / float64(count)
	assert.GreaterOrEqual(t, avgWins, 1.0-0.01)
	assert.LessOrEqual(t, avgWins, 1.0+0.01)
}
