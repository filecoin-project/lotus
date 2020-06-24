package main

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/drand/drand/chain"
	hclient "github.com/drand/drand/client/http"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/testground/sdk-go/sync"

	"github.com/drand/drand/demo/node"
)

var (
	PrepareDrandTimeout = time.Minute
	drandConfigTopic = sync.NewTopic("drand-config", &dtypes.DrandConfig{})
)

// waitForDrandConfig should be called by filecoin instances before constructing the lotus Node
// you can use the returned dtypes.DrandConfig to override the default production config.
func waitForDrandConfig(ctx context.Context, client sync.Client) (*dtypes.DrandConfig, error) {
	ch := make(chan *dtypes.DrandConfig, 1)
	sub := client.MustSubscribe(ctx, drandConfigTopic, ch)
	select {
	case cfg := <-ch:
		return cfg, nil
	case err := <-sub.Done():
		return nil, err
	}
}

// prepareDrandNode starts a drand instance and runs a DKG with the other members of the composition group.
// Once the chain is running, the leader publishes the chain info needed by lotus nodes on
// drandConfigTopic
func prepareDrandNode(t *TestEnvironment) (node.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareDrandTimeout)
	defer cancel()

	startTime := time.Now()

	seq := t.GroupSeq
	isLeader := seq == 1
	nNodes := t.TestGroupInstanceCount

	myAddr := t.NetClient.MustGetDataNetworkIP()
	// TODO: add test params for drand
	period := "10s"
	beaconOffset := 12
	threshold := 2

	// TODO(maybe): use TLS?
	n := node.NewLocalNode(int(seq), period, "~/", false, myAddr.String())

	// share the node addresses with other nodes
	// TODO: if we implement TLS, this is where we'd share public TLS keys
	type NodeAddr struct {
		PrivateAddr string
		PublicAddr  string
		IsLeader    bool
	}
	addrTopic := sync.NewTopic("drand-addrs", &NodeAddr{})
	var publicAddrs []string
	var leaderAddr string
	ch := make(chan *NodeAddr)
	t.SyncClient.MustPublishSubscribe(ctx, addrTopic, &NodeAddr{
		PrivateAddr: n.PrivateAddr(),
		PublicAddr: n.PublicAddr(),
		IsLeader: isLeader,
	}, ch)
	for i := 0; i < nNodes; i++ {
		msg, ok := <-ch
		if !ok {
			return nil, fmt.Errorf("failed to read drand node addr from sync service")
		}
		publicAddrs = append(publicAddrs, fmt.Sprintf("http://%s", msg.PublicAddr))
		if msg.IsLeader {
			leaderAddr = msg.PrivateAddr
		}
	}
	if leaderAddr == "" {
		return nil, fmt.Errorf("got %d drand addrs, but no leader", len(publicAddrs))
	}

	t.SyncClient.MustSignalAndWait(ctx, "drand-start", nNodes)
	t.RecordMessage("Starting drand sharing ceremony")
	if err := n.Start("~/"); err != nil {
		return nil, err
	}

	alive := false
	waitSecs := 10
	for i := 0; i < waitSecs; i++ {
		if !n.Ping() {
			time.Sleep(time.Second)
			continue
		}
		t.R().RecordPoint("drand_first_ping", time.Now().Sub(startTime).Seconds())
		alive = true
		break
	}
	if !alive {
		return nil, fmt.Errorf("drand node %d failed to start after %d seconds", t.GroupSeq, waitSecs)
	}

	// run DKG
	t.SyncClient.MustSignalAndWait(ctx, "drand-dkg-start", nNodes)
	grp := n.RunDKG(nNodes, threshold, period, isLeader, leaderAddr, beaconOffset)
	if grp == nil {
		return nil, fmt.Errorf("drand dkg failed")
	}
	t.R().RecordPoint("drand_dkg_complete", time.Now().Sub(startTime).Seconds())

	// wait for chain to begin
	to := time.Until(time.Unix(grp.GenesisTime, 0).Add(3 * time.Second).Add(grp.Period))
	time.Sleep(to)



	// verify that we can get a round of randomness from the chain using an http client
	info := chain.NewChainInfo(grp)
	client, err := hclient.NewWithInfo(publicAddrs[0], info, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create drand http client: %w", err)
	}

	_, err = client.Get(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("unable to get initial drand round: %w", err)
	}

	// if we're the leader, publish the config to the sync service
	if isLeader {
		buf := bytes.Buffer{}
		if err := info.ToJSON(&buf); err != nil {
			return nil, fmt.Errorf("error marshaling chain info: %w", err)
		}
		msg := dtypes.DrandConfig{
			Servers:       publicAddrs,
			ChainInfoJSON: buf.String(),
		}
		t.SyncClient.MustPublish(ctx, drandConfigTopic, &msg)
	}

	return n, nil
}
