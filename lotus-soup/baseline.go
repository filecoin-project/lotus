package main

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/testground/sdk-go/sync"
)

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

	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func runBaselineMiner(t *TestEnvironment) error {
	t.RecordMessage("running miner")
	_, err := prepareMiner(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	addrs, err := collectClientsAddrs(t, ctx, t.IntParam("clients"))
	if err != nil {
		return err
	}

	t.RecordMessage("got %v client addrs", len(addrs))

	// subscribe to clients

	// TODO wait a bit for network to bootstrap
	// TODO just wait until completion of test, serving requests -- the client does all the job

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func runBaselineClient(t *TestEnvironment) error {
	t.RecordMessage("running client")
	client, err := prepareClient(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	addrs, err := collectMinersAddrs(t, ctx, t.IntParam("miners"))
	if err != nil {
		return err
	}

	t.RecordMessage("got %v miner addrs", len(addrs))

	if err := client.fullApi.NetConnect(ctx, addrs[0]); err != nil {
		return err
	}

	t.RecordMessage("client connected to miner")

	// TODO generate a number of random "files" and publish them to one or more miners
	// TODO broadcast published content CIDs to other clients
	// TODO select a random piece of content published by some other client and retreieve it

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func collectMinersAddrs(t *TestEnvironment, ctx context.Context, miners int) ([]peer.AddrInfo, error) {
	return collectAddrs(t, ctx, minersAddrsTopic, miners)
}

func collectClientsAddrs(t *TestEnvironment, ctx context.Context, clients int) ([]peer.AddrInfo, error) {
	return collectAddrs(t, ctx, clientsAddrsTopic, clients)
}

func collectAddrs(t *TestEnvironment, ctx context.Context, topic *sync.Topic, expectedValues int) ([]peer.AddrInfo, error) {
	ch := make(chan peer.AddrInfo)
	sub := t.SyncClient.MustSubscribe(ctx, topic, ch)

	addrs := make([]peer.AddrInfo, 0, expectedValues)
	for i := 0; i < expectedValues; i++ {
		select {
		case a := <-ch:
			addrs = append(addrs, a)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for %v addrs: %w", topic, err)
		}
	}

	return addrs, nil
}
