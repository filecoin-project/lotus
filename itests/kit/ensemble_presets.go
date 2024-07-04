package kit

import (
	"testing"

	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
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

func EnsembleWorker(t *testing.T, opts ...interface{}) (*TestFullNode, *TestMiner, *TestWorker, *Ensemble) {
	opts = append(opts, WithAllSubsystems())

	eopts, nopts := siftOptions(t, opts)

	var (
		full   TestFullNode
		miner  TestMiner
		worker TestWorker
	)
	ens := NewEnsemble(t, eopts...).FullNode(&full, nopts...).Miner(&miner, &full, nopts...).Worker(&miner, &worker, nopts...).Start()
	return &full, &miner, &worker, ens
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

// EnsembleOneTwo creates and starts an Ensemble with one full node and two miners participating in F3.
// It does not interconnect nodes nor does it begin mining.
//
// This ensemble only configured miners to participate in F3, and it expects to provide,
// F3Enabled as an option to configure the parameters for F3. If F3Enabled is nos passed as a node option
// to the ensemble, building the miner fails.
func EnsembleOneTwoF3(t *testing.T, opts ...interface{}) (*TestFullNode, *TestMiner, *TestMiner, *Ensemble) {
	opts = append(opts, WithAllSubsystems())

	eopts, nopts := siftOptions(t, opts)

	var (
		full     TestFullNode
		one, two TestMiner
	)
	minerOpts := append(nopts, ConstructorOpts(
		node.Override(node.F3Participation, modules.F3Participation),
	))
	ens := NewEnsemble(t, eopts...).
		FullNode(&full, nopts...).
		Miner(&one, &full, minerOpts...).
		Miner(&two, &full, minerOpts...).
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
