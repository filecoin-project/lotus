package stati

import (
	"fmt"
	"math"
)

type MeanVar struct {
	n    float64
	mean float64
	m2   float64
}

func (v1 *MeanVar) AddPoint(value float64) {
	// based on https://en.wikipedia.org/wiki/Algorithms_for_calculating_variance#Welford's_online_algorithm
	v1.n++
	delta := value - v1.mean
	v1.mean += delta / v1.n
	delta2 := value - v1.mean
	v1.m2 += delta * delta2
}

func (v1 *MeanVar) Mean() float64 {
	return v1.mean
}
func (v1 *MeanVar) N() float64 {
	return v1.n
}
func (v1 *MeanVar) Variance() float64 {
	return v1.m2 / (v1.n - 1)
}
func (v1 *MeanVar) Stddev() float64 {
	return math.Sqrt(v1.Variance())
}

func (v1 MeanVar) String() string {
	return fmt.Sprintf("%f stddev: %f (%.0f)", v1.Mean(), v1.Stddev(), v1.N())
}

func (v1 *MeanVar) Combine(v2 *MeanVar) {
	if v1.n == 0 {
		*v1 = *v2
		return
	}
	if v2.n == 0 {
		return
	}
	if v1.n == 1 {
		cpy := *v2
		cpy.AddPoint(v1.mean)
		*v1 = cpy
		return
	}
	if v2.n == 1 {
		v1.AddPoint(v2.mean)
		return
	}

	newCount := v1.n + v2.n
	delta := v2.mean - v1.mean
	meanDelta := delta * v2.n / newCount
	m2 := v1.m2 + v2.m2 + delta*meanDelta*v1.n
	v1.n = newCount
	v1.mean += meanDelta
	v1.m2 = m2
}
