package kit

import (
	"testing"
	"time"
)

// EnsembleMinimal creates and starts an Ensemble with a single full node and a single miner.
// It does not interconnect nodes nor does it begin mining.
//
// This function supports passing both ensemble and node functional options.
// Functional options are applied to all nodes.
func EnsembleMinimal(t *testing.T, opts ...interface{}) (*TestFullNode, *TestMiner, *Ensemble) {
	opts = append(opts, WithAllSubsystems())

	eopts, nopts := siftOptions(t, opts)

	var (
		full  TestFullNode
		miner TestMiner
	)
	ens := NewEnsemble(t, eopts...).FullNode(&full, nopts...).Miner(&miner, &full, nopts...).Start()
	return &full, &miner, ens
}

func EnsembleWithMinerAndMarketNodes(t *testing.T, opts ...interface{}) (*TestFullNode, *TestMiner, *TestMiner, *Ensemble) {
	eopts, nopts := siftOptions(t, opts)

	var (
		fullnode     TestFullNode
		main, market TestMiner
	)

	mainNodeOpts := []NodeOpt{WithSubsystems(SSealing, SSectorStorage, SMining), DisableLibp2p()}
	mainNodeOpts = append(mainNodeOpts, nopts...)

	blockTime := 100 * time.Millisecond
	ens := NewEnsemble(t, eopts...).FullNode(&fullnode, nopts...).Miner(&main, &fullnode, mainNodeOpts...).Start()
	ens.BeginMining(blockTime)

	marketNodeOpts := []NodeOpt{OwnerAddr(fullnode.DefaultKey), MainMiner(&main), WithSubsystems(SMarkets)}
	marketNodeOpts = append(marketNodeOpts, nopts...)

	ens.Miner(&market, &fullnode, marketNodeOpts...).Start().Connect(market, fullnode)

	return &fullnode, &main, &market, ens
}

// EnsembleTwoOne creates and starts an Ensemble with two full nodes and one miner.
// It does not interconnect nodes nor does it begin mining.
//
// This function supports passing both ensemble and node functional options.
// Functional options are applied to all nodes.
func EnsembleTwoOne(t *testing.T, opts ...interface{}) (*TestFullNode, *TestFullNode, *TestMiner, *Ensemble) {
	opts = append(opts, WithAllSubsystems())

	eopts, nopts := siftOptions(t, opts)

	var (
		one, two TestFullNode
		miner    TestMiner
	)
	ens := NewEnsemble(t, eopts...).FullNode(&one, nopts...).FullNode(&two, nopts...).Miner(&miner, &one, nopts...).Start()
	return &one, &two, &miner, ens
}

// EnsembleOneTwo creates and starts an Ensemble with one full node and two miners.
// It does not interconnect nodes nor does it begin mining.
//
// This function supports passing both ensemble and node functional options.
// Functional options are applied to all nodes.
func EnsembleOneTwo(t *testing.T, opts ...interface{}) (*TestFullNode, *TestMiner, *TestMiner, *Ensemble) {
	opts = append(opts, WithAllSubsystems())

	eopts, nopts := siftOptions(t, opts)

	var (
		full     TestFullNode
		one, two TestMiner
	)
	ens := NewEnsemble(t, eopts...).
		FullNode(&full, nopts...).
		Miner(&one, &full, nopts...).
		Miner(&two, &full, nopts...).
		Start()

	return &full, &one, &two, ens
}

func siftOptions(t *testing.T, opts []interface{}) (eopts []EnsembleOpt, nopts []NodeOpt) {
	for _, v := range opts {
		switch o := v.(type) {
		case EnsembleOpt:
			eopts = append(eopts, o)
		case NodeOpt:
			nopts = append(nopts, o)
		default:
			t.Fatalf("invalid option type: %T", o)
		}
	}
	return eopts, nopts
}
