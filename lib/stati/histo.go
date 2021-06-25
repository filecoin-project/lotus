package stati

import (
	"math"

	"golang.org/x/xerrors"
)

type Histogram struct {
	Buckets []float64
	Counts  []uint64
}

// NewHistogram creates a histograme with buckets defined as:
// {x > -Inf, x >= buckets[0], x >= buckets[1], ..., x >= buckets[i]}
func NewHistogram(buckets []float64) (*Histogram, error) {
	if len(buckets) == 0 {
		return nil, xerrors.Errorf("empty buckets")
	}
	prev := buckets[0]
	for i, v := range buckets[1:] {
		if v < prev {
			return nil, xerrors.Errorf("bucket at index %d is smaller than previous %f < %f", i+1, v, prev)
		}
		prev = v
	}
	h := &Histogram{
		Buckets: append([]float64{math.Inf(-1)}, buckets...),
		Counts:  make([]uint64, len(buckets)+1),
	}
	return h, nil
}

func (h *Histogram) Observe(x float64) {
	for i, b := range h.Buckets {
		if x >= b {
			h.Counts[i]++
		} else {
			break
		}
	}
}

func (h *Histogram) Total() uint64 {
	return h.Counts[0]
}

func (h *Histogram) Get(i int) uint64 {
	if i >= len(h.Counts)-2 {
		return h.Counts[i]
	}
	return h.Counts[i+1] - h.Counts[i+2]
}
func (h *Histogram) GetRatio(i int) float64 {
	return float64(h.Get(i)) / float64(h.Total())
}
