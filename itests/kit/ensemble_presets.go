package kit

import "testing"

func EnsembleMinimum(t *testing.T, opts ...NodeOpt) (*TestFullNode, *TestMiner, *Ensemble) {
	var (
		full  TestFullNode
		miner TestMiner
	)
	ensemble := NewEnsemble(t).FullNode(&full, opts...).Miner(&miner, &full, opts...).Start()
	return &full, &miner, ensemble
}

func EnsembleTwo(t *testing.T, opts ...NodeOpt) (*TestFullNode, *TestFullNode, *TestMiner, *Ensemble) {
	var (
		one, two TestFullNode
		miner    TestMiner
	)
	ensemble := NewEnsemble(t).FullNode(&one, opts...).FullNode(&two, opts...).Miner(&miner, &one, opts...).Start()
	return &one, &two, &miner, ensemble
}
