package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"time"

	"github.com/drand/drand/chain"
	hclient "github.com/drand/drand/client/http"
	"github.com/drand/drand/demo/node"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/testground/sdk-go/sync"
)

var PrepareDrandTimeout = time.Minute

type DrandInstance struct {
	Node        node.Node
	GossipRelay *lp2p.GossipRelayNode

	stateDir string
}

func (d *DrandInstance) Cleanup() error {
	return os.RemoveAll(d.stateDir)
}

func runDrandNode(t *TestEnvironment) error {
	t.RecordMessage("running drand node")
	dr, err := prepareDrandNode(t)
	if err != nil {
		return err
	}
	defer dr.Cleanup()

	// TODO add ability to halt / recover on demand
	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

// prepareDrandNode starts a drand instance and runs a DKG with the other members of the composition group.
// Once the chain is running, the leader publishes the chain info needed by lotus nodes on
// drandConfigTopic
func prepareDrandNode(t *TestEnvironment) (*DrandInstance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareDrandTimeout)
	defer cancel()

	startTime := time.Now()

	seq := t.GroupSeq
	isLeader := seq == 1
	nNodes := t.TestGroupInstanceCount

	myAddr := t.NetClient.MustGetDataNetworkIP()
	period := t.DurationParam("drand_period")
	threshold := t.IntParam("drand_threshold")
	runGossipRelay := t.BooleanParam("drand_gossip_relay")

	beaconOffset := 3

	stateDir, err := ioutil.TempDir("", fmt.Sprintf("drand-%d", t.GroupSeq))
	if err != nil {
		return nil, err
	}

	// TODO(maybe): use TLS?
	n := node.NewLocalNode(int(seq), period.String(), stateDir, false, myAddr.String())

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
	_, sub := t.SyncClient.MustPublishSubscribe(ctx, addrTopic, &NodeAddr{
		PrivateAddr: n.PrivateAddr(),
		PublicAddr:  n.PublicAddr(),
		IsLeader:    isLeader,
	}, ch)
	for i := 0; i < nNodes; i++ {
		select {
		case msg := <-ch:
			publicAddrs = append(publicAddrs, fmt.Sprintf("http://%s", msg.PublicAddr))
			if msg.IsLeader {
				leaderAddr = msg.PrivateAddr
			}
		case err := <-sub.Done():
			return nil, fmt.Errorf("unable to read drand addrs from sync service: %w", err)
		}
	}
	if leaderAddr == "" {
		return nil, fmt.Errorf("got %d drand addrs, but no leader", len(publicAddrs))
	}

	t.SyncClient.MustSignalAndWait(ctx, "drand-start", nNodes)
	t.RecordMessage("Starting drand sharing ceremony")
	if err := n.Start(stateDir); err != nil {
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
	if !isLeader {
		time.Sleep(time.Second)
	}
	grp := n.RunDKG(nNodes, threshold, period.String(), isLeader, leaderAddr, beaconOffset)
	if grp == nil {
		return nil, fmt.Errorf("drand dkg failed")
	}
	t.R().RecordPoint("drand_dkg_complete", time.Now().Sub(startTime).Seconds())

	t.RecordMessage("drand dkg complete, waiting for chain start")
	// wait for chain to begin
	to := time.Until(time.Unix(grp.GenesisTime, 0).Add(3 * time.Second).Add(grp.Period))
	time.Sleep(to)

	t.RecordMessage("drand beacon chain started, fetching initial round via http")
	// verify that we can get a round of randomness from the chain using an http client
	info := chain.NewChainInfo(grp)
	myPublicAddr := fmt.Sprintf("http://%s", n.PublicAddr())
	client, err := hclient.NewWithInfo(myPublicAddr, info, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create drand http client: %w", err)
	}

	_, err = client.Get(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("unable to get initial drand round: %w", err)
	}

	// start gossip relay (unless disabled via testplan parameter)
	var gossipRelay *lp2p.GossipRelayNode
	var relayAddrs []peer.AddrInfo

	if runGossipRelay {
		gossipDir := path.Join(stateDir, "gossip-relay")
		listenAddr := fmt.Sprintf("/ip4/%s/tcp/7777", myAddr.String())
		relayCfg := lp2p.GossipRelayConfig{
			ChainHash:    hex.EncodeToString(info.Hash()),
			Addr:         listenAddr,
			DataDir:      gossipDir,
			IdentityPath: path.Join(gossipDir, "identity.key"),
			Insecure:     true,
			Client:       client,
		}
		t.RecordMessage("starting drand gossip relay")
		gossipRelay, err = lp2p.NewGossipRelayNode(log.DefaultLogger, &relayCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to construct drand gossip relay: %w", err)
		}

		t.RecordMessage("sharing gossip relay addrs")
		// share the gossip relay addrs so we can publish them in DrandRuntimeInfo
		relayInfo, err := relayAddrInfo(gossipRelay.Multiaddrs(), myAddr)
		if err != nil {
			return nil, err
		}
		infoCh := make(chan *peer.AddrInfo, nNodes)
		infoTopic := sync.NewTopic("drand-gossip-addrs", &peer.AddrInfo{})

		_, sub := t.SyncClient.MustPublishSubscribe(ctx, infoTopic, relayInfo, infoCh)
		for i := 0; i < nNodes; i++ {
			select {
			case ai := <-infoCh:
				relayAddrs = append(relayAddrs, *ai)
			case err := <-sub.Done():
				return nil, fmt.Errorf("unable to get drand relay addr from sync service: %w", err)
			}
		}
	}

	// if we're the leader, publish the config to the sync service
	if isLeader {
		buf := bytes.Buffer{}
		if err := info.ToJSON(&buf); err != nil {
			return nil, fmt.Errorf("error marshaling chain info: %w", err)
		}
		cfg := DrandRuntimeInfo{
			Config: dtypes.DrandConfig{
				Servers:       publicAddrs,
				ChainInfoJSON: buf.String(),
			},
			GossipBootstrap: relayAddrs,
		}
		dump, _ := json.Marshal(cfg)
		t.RecordMessage("publishing drand config on sync topic: %s", string(dump))
		t.SyncClient.MustPublish(ctx, drandConfigTopic, &cfg)
	}

	return &DrandInstance{
		Node:        n,
		GossipRelay: gossipRelay,
		stateDir:    stateDir,
	}, nil
}

// waitForDrandConfig should be called by filecoin instances before constructing the lotus Node
// you can use the returned dtypes.DrandConfig to override the default production config.
func waitForDrandConfig(ctx context.Context, client sync.Client) (*DrandRuntimeInfo, error) {
	ch := make(chan *DrandRuntimeInfo, 1)
	sub := client.MustSubscribe(ctx, drandConfigTopic, ch)
	select {
	case cfg := <-ch:
		return cfg, nil
	case err := <-sub.Done():
		return nil, err
	}
}

func relayAddrInfo(addrs []ma.Multiaddr, dataIP net.IP) (*peer.AddrInfo, error) {
	for _, a := range addrs {
		if ip, _ := a.ValueForProtocol(ma.P_IP4); ip != dataIP.String() {
			continue
		}
		return peer.AddrInfoFromP2pAddr(a)
	}
	return nil, fmt.Errorf("no addr found with data ip %s in addrs: %v", dataIP, addrs)
}
