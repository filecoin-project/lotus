package main

import (
	"math/rand"
	"testing"
)

func TestMeanVar(t *testing.T) {
	N := 16
	ss := make([]*meanVar, N)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < N; i++ {
		ss[i] = &meanVar{}
		maxJ := rng.Intn(1000)
		for j := 0; j < maxJ; j++ {
			ss[i].AddPoint(rng.NormFloat64()*5 + 500)
		}
		t.Logf("mean: %f, stddev: %f, count %f", ss[i].mean, ss[i].Stddev(), ss[i].n)
	}
	out := &meanVar{}
	for i := 0; i < N; i++ {
		out.Combine(ss[i])
		t.Logf("combine: mean: %f, stddev: %f", out.mean, out.Stddev())
	}
}

func TestCovar(t *testing.T) {
	N := 16
	ss := make([]*covar, N)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < N; i++ {
		ss[i] = &covar{}
		maxJ := rng.Intn(1000) + 500
		for j := 0; j < maxJ; j++ {
			x := rng.NormFloat64()*5 + 500
			ss[i].AddPoint(x, x*(rng.NormFloat64()/10+2)-rng.NormFloat64()-1000)
		}
		t.Logf("covar: %f, y = %f*x+%f @%.0f", ss[i].Covariance(), ss[i].A(), ss[i].B(), ss[i].n)
	}
	out := &covar{}
	for i := 0; i < N; i++ {
		out.Combine(ss[i])
		t.Logf("combine: covar: %f, y = %f*x+%f", ss[i].Covariance(), ss[i].A(), ss[i].B())
	}
}
