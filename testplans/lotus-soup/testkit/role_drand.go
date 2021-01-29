package testkit

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"time"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/client"
	hclient "github.com/drand/drand/client/http"
	"github.com/drand/drand/core"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/lp2p"
	dnet "github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	dtest "github.com/drand/drand/test"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/testground/sdk-go/sync"

	"github.com/filecoin-project/lotus/testplans/lotus-soup/statemachine"
)

var (
	PrepareDrandTimeout = 3 * time.Minute
	secretDKG           = "dkgsecret"
)

type DrandInstance struct {
	daemon      *core.Drand
	httpClient  client.Client
	ctrlClient  *dnet.ControlClient
	gossipRelay *lp2p.GossipRelayNode

	t        *TestEnvironment
	stateDir string
	priv     *key.Pair
	pubAddr  string
	privAddr string
	ctrlAddr string
}

func (dr *DrandInstance) Start() error {
	opts := []core.ConfigOption{
		core.WithLogLevel(getLogLevel(dr.t)),
		core.WithConfigFolder(dr.stateDir),
		core.WithPublicListenAddress(dr.pubAddr),
		core.WithPrivateListenAddress(dr.privAddr),
		core.WithControlPort(dr.ctrlAddr),
		core.WithInsecure(),
	}
	conf := core.NewConfig(opts...)
	fs := key.NewFileStore(conf.ConfigFolder())
	fs.SaveKeyPair(dr.priv)
	key.Save(path.Join(dr.stateDir, "public.toml"), dr.priv.Public, false)
	if dr.daemon == nil {
		drand, err := core.NewDrand(fs, conf)
		if err != nil {
			return err
		}
		dr.daemon = drand
	} else {
		drand, err := core.LoadDrand(fs, conf)
		if err != nil {
			return err
		}
		drand.StartBeacon(true)
		dr.daemon = drand
	}
	return nil
}

func (dr *DrandInstance) Ping() bool {
	cl := dr.ctrl()
	if err := cl.Ping(); err != nil {
		return false
	}
	return true
}

func (dr *DrandInstance) Close() error {
	dr.gossipRelay.Shutdown()
	dr.daemon.Stop(context.Background())
	return os.RemoveAll(dr.stateDir)
}

func (dr *DrandInstance) ctrl() *dnet.ControlClient {
	if dr.ctrlClient != nil {
		return dr.ctrlClient
	}
	cl, err := dnet.NewControlClient(dr.ctrlAddr)
	if err != nil {
		dr.t.RecordMessage("drand can't instantiate control client: %w", err)
		return nil
	}
	dr.ctrlClient = cl
	return cl
}

func (dr *DrandInstance) RunDKG(nodes, thr int, timeout string, leader bool, leaderAddr string, beaconOffset int) *key.Group {
	cl := dr.ctrl()
	p := dr.t.DurationParam("drand_period")
	catchupPeriod := dr.t.DurationParam("drand_catchup_period")
	t, _ := time.ParseDuration(timeout)
	var grp *drand.GroupPacket
	var err error
	if leader {
		grp, err = cl.InitDKGLeader(nodes, thr, p, catchupPeriod, t, nil, secretDKG, beaconOffset)
	} else {
		leader := dnet.CreatePeer(leaderAddr, false)
		grp, err = cl.InitDKG(leader, nil, secretDKG)
	}
	if err != nil {
		dr.t.RecordMessage("drand dkg run failed: %w", err)
		return nil
	}
	kg, _ := key.GroupFromProto(grp)
	return kg
}

func (dr *DrandInstance) Halt() {
	dr.t.RecordMessage("drand node #%d halting", dr.t.GroupSeq)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dr.daemon.Stop(ctx)
}

func (dr *DrandInstance) Resume() {
	dr.t.RecordMessage("drand node #%d resuming", dr.t.GroupSeq)
	dr.Start()
	// block until we can fetch the round corresponding to the current time
	startTime := time.Now()
	round := dr.httpClient.RoundAt(startTime)
	timeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{}, 1)
	go func() {
		for {
			res, err := dr.httpClient.Get(ctx, round)
			if err == nil {
				dr.t.RecordMessage("drand chain caught up to round %d", res.Round())
				done <- struct{}{}
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()

	select {
	case <-ctx.Done():
		dr.t.RecordMessage("drand chain failed to catch up after %s", timeout.String())
	case <-done:
		dr.t.RecordMessage("drand chain resumed after %s catchup time", time.Since(startTime))
	}
}

func (dr *DrandInstance) RunDefault() error {
	dr.t.RecordMessage("running drand node")

	if dr.t.IsParamSet("suspend_events") {
		suspender := statemachine.NewSuspender(dr, dr.t.RecordMessage)
		suspender.RunEvents(dr.t.StringParam("suspend_events"))
	}

	dr.t.WaitUntilAllDone()
	return nil
}

// prepareDrandNode starts a drand instance and runs a DKG with the other members of the composition group.
// Once the chain is running, the leader publishes the chain info needed by lotus nodes on
// drandConfigTopic
func PrepareDrandInstance(t *TestEnvironment) (*DrandInstance, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareDrandTimeout)
	defer cancel()

	ApplyNetworkParameters(t)

	startTime := time.Now()

	seq := t.GroupSeq
	isLeader := seq == 1
	nNodes := t.TestGroupInstanceCount

	myAddr := t.NetClient.MustGetDataNetworkIP()
	threshold := t.IntParam("drand_threshold")
	runGossipRelay := t.BooleanParam("drand_gossip_relay")

	beaconOffset := 3

	stateDir, err := ioutil.TempDir("/tmp", fmt.Sprintf("drand-%d", t.GroupSeq))
	if err != nil {
		return nil, err
	}

	dr := DrandInstance{
		t:        t,
		stateDir: stateDir,
		pubAddr:  dtest.FreeBind(myAddr.String()),
		privAddr: dtest.FreeBind(myAddr.String()),
		ctrlAddr: dtest.FreeBind("localhost"),
	}
	dr.priv = key.NewKeyPair(dr.privAddr)

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
		PrivateAddr: dr.privAddr,
		PublicAddr:  dr.pubAddr,
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
	if err := dr.Start(); err != nil {
		return nil, err
	}

	alive := false
	waitSecs := 10
	for i := 0; i < waitSecs; i++ {
		if !dr.Ping() {
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
		time.Sleep(3 * time.Second)
	}
	grp := dr.RunDKG(nNodes, threshold, "10s", isLeader, leaderAddr, beaconOffset)
	if grp == nil {
		return nil, fmt.Errorf("drand dkg failed")
	}
	t.R().RecordPoint("drand_dkg_complete", time.Now().Sub(startTime).Seconds())

	t.RecordMessage("drand dkg complete, waiting for chain start: %v", time.Until(time.Unix(grp.GenesisTime, 0).Add(grp.Period)))

	// wait for chain to begin
	to := time.Until(time.Unix(grp.GenesisTime, 0).Add(5 * time.Second).Add(grp.Period))
	time.Sleep(to)

	t.RecordMessage("drand beacon chain started, fetching initial round via http")
	// verify that we can get a round of randomness from the chain using an http client
	info := chain.NewChainInfo(grp)
	myPublicAddr := fmt.Sprintf("http://%s", dr.pubAddr)
	dr.httpClient, err = hclient.NewWithInfo(myPublicAddr, info, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create drand http client: %w", err)
	}

	_, err = dr.httpClient.Get(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("unable to get initial drand round: %w", err)
	}

	// start gossip relay (unless disabled via testplan parameter)
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
			Client:       dr.httpClient,
		}
		t.RecordMessage("starting drand gossip relay")
		dr.gossipRelay, err = lp2p.NewGossipRelayNode(log.NewLogger(nil, getLogLevel(t)), &relayCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to construct drand gossip relay: %w", err)
		}

		t.RecordMessage("sharing gossip relay addrs")
		// share the gossip relay addrs so we can publish them in DrandRuntimeInfo
		relayInfo, err := relayAddrInfo(dr.gossipRelay.Multiaddrs(), myAddr)
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
		t.DebugSpew("publishing drand config on sync topic: %v", cfg)
		t.SyncClient.MustPublish(ctx, DrandConfigTopic, &cfg)
	}

	// signal ready state
	t.SyncClient.MustSignalAndWait(ctx, StateReady, t.TestInstanceCount)
	return &dr, nil
}

// waitForDrandConfig should be called by filecoin instances before constructing the lotus Node
// you can use the returned dtypes.DrandConfig to override the default production config.
func waitForDrandConfig(ctx context.Context, client sync.Client) (*DrandRuntimeInfo, error) {
	ch := make(chan *DrandRuntimeInfo, 1)
	sub := client.MustSubscribe(ctx, DrandConfigTopic, ch)
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

func getLogLevel(t *TestEnvironment) int {
	switch t.StringParam("drand_log_level") {
	case "info":
		return log.LogInfo
	case "debug":
		return log.LogDebug
	default:
		return log.LogNone
	}
}
