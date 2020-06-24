package main

// This is the basline test; Filecoin 101.
//
// A network with a bootstrapper, a number of miners, and a number of clients/full nodes
// is constructed and connected through the bootstrapper.
// Some funds are allocated to each node and a number of sectors are presealed in the genesis block.
//
// The test plan:
// One or more clients store content to one or more miners, testing storage deals.
// The plan ensures that the storage deals hit the blockchain and measure the time it took.
// Verification: one or more clients retrieve and verify the hashes of stored content.
// The plan ensures that all (previously) published content can be correctly retrieved
// and measures the time it took.
//
// Preparation of the genesis block: this is the responsibility of the bootstrapper.
// In order to compute the genesis block, we need to collect identities and presealed
// sectors from each node.
// The we create a genesis block that allocates some funds to each node and collects
// the presealed sectors.
var baselineRoles = map[string]func(*TestEnvironment) error{
	"bootstrapper": runBaselineBootstrapper,
	"miner":        runBaselineMiner,
	"client":       runBaselineClient,
}

func runBaselineBootstrapper(t *TestEnvironment) error {
	t.RecordMessage("running bootstrapper")
	_, err := prepareBootstrapper(t)
	if err != nil {
		return err
	}

	// TODO just wait until completion of test, nothing else to do

	return nil
}

func runBaselineMiner(t *TestEnvironment) error {
	t.RecordMessage("running miner")
	_, err := prepareMiner(t)
	if err != nil {
		return err
	}

	// TODO wait a bit for network to bootstrap
	// TODO just wait until completion of test, serving requests -- the client does all the job

	return nil
}

func runBaselineClient(t *TestEnvironment) error {
	t.RecordMessage("running client")
	_, err := prepareClient(t)
	if err != nil {
		return err
	}

	// TODO generate a number of random "files" and publish them to one or more miners
	// TODO broadcast published content CIDs to other clients
	// TODO select a random piece of content published by some other client and retreieve it

	return nil
}
