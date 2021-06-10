package kit

import "testing"

// EnsembleMinimal creates and starts an ensemble with a single full node and a single miner.
// It does not interconnect nodes nor does it begin mining.
func EnsembleMinimal(t *testing.T, opts ...NodeOpt) (*TestFullNode, *TestMiner, *Ensemble) {
	var (
		full  TestFullNode
		miner TestMiner
	)
	ensemble := NewEnsemble(t).FullNode(&full, opts...).Miner(&miner, &full, opts...).Start()
	return &full, &miner, ensemble
}

// EnsembleTwoOne creates and starts an ensemble with two full nodes and one miner.
// It does not interconnect nodes nor does it begin mining.
func EnsembleTwoOne(t *testing.T, opts ...NodeOpt) (*TestFullNode, *TestFullNode, *TestMiner, *Ensemble) {
	var (
		one, two TestFullNode
		miner    TestMiner
	)
	ensemble := NewEnsemble(t).FullNode(&one, opts...).FullNode(&two, opts...).Miner(&miner, &one, opts...).Start()
	return &one, &two, &miner, ensemble
}

// EnsembleOneTwo creates and starts an ensemble with one full node and two miners.
// It does not interconnect nodes nor does it begin mining.
func EnsembleOneTwo(t *testing.T, opts ...NodeOpt) (*TestFullNode, *TestMiner, *TestMiner, *Ensemble) {
	var (
		full     TestFullNode
		one, two TestMiner
	)
	ensemble := NewEnsemble(t).
		FullNode(&full, opts...).
		Miner(&one, &full, opts...).
		Miner(&two, &full, opts...).
		Start()

	return &full, &one, &two, ensemble
}
