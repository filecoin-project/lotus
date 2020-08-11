package messagepool

import "math"

func noWinnersProb() []float64 {
	poissPdf := func(x float64) float64 {
		const Mu = 5
		lg, _ := math.Lgamma(x + 1)
		result := math.Exp((math.Log(Mu) * x) - lg - Mu)
		return result
	}

	out := make([]float64, 0, MaxBlocks)
	for i := 0; i < MaxBlocks; i++ {
		out = append(out, poissPdf(float64(i)))
	}
	return out
}

func binomialCoefficient(n, k float64) float64 {
	if k > n {
		return math.NaN()
	}
	r := 1.0
	for d := 1.0; d <= k; d++ {
		r *= n
		r /= d
		n -= 1
	}
	return r
}

func (mp *MessagePool) blockProbabilities(tq float64) []float64 {
	noWinners := noWinnersProb() // cache this

	p := 1 - tq
	binoPdf := func(x, trials float64) float64 {
		// based on https://github.com/atgjack/prob
		if x > trials {
			return 0
		}
		if p == 0 {
			if x == 0 {
				return 1.0
			}
			return 0.0
		}
		if p == 1 {
			if x == trials {
				return 1.0
			}
			return 0.0
		}
		coef := binomialCoefficient(trials, x)
		pow := math.Pow(p, x) * math.Pow(1-p, trials-x)
		if math.IsInf(coef, 0) {
			return 0
		}
		return coef * pow
	}

	out := make([]float64, 0, MaxBlocks)
	for place := 0; place < MaxBlocks; place++ {
		var pPlace float64
		for otherWinners, pCase := range noWinners {
			pPlace += pCase * binoPdf(float64(place), float64(otherWinners+1))
		}
		out = append(out, pPlace)
	}
	return out
}
