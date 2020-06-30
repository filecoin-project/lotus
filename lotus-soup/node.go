package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	logging "github.com/ipfs/go-log/v2"
	influxdb "github.com/kpacha/opencensus-influxdb"

	"github.com/libp2p/go-libp2p-core/peer"
	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

func init() {
	logging.SetLogLevel("*", "ERROR")

	os.Setenv("BELLMAN_NO_GPU", "1")

	build.InsecurePoStValidation = true
	build.DisableBuiltinAssets = true

	power.ConsensusMinerMinPower = big.NewInt(2048)
	saminer.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)
}

var PrepareNodeTimeout = time.Minute

type TestEnvironment struct {
	*runtime.RunEnv
	*run.InitContext
}

// workaround for default params being wrapped in quote chars
func (t *TestEnvironment) StringParam(name string) string {
	return strings.Trim(t.RunEnv.StringParam(name), "\"")
}

func (t *TestEnvironment) DurationParam(name string) time.Duration {
	d, err := time.ParseDuration(t.StringParam(name))
	if err != nil {
		panic(fmt.Errorf("invalid duration value for param '%s': %w", name, err))
	}
	return d
}

type Node struct {
	fullApi  api.FullNode
	minerApi api.StorageMiner
	stop     node.StopFunc
	MineOne  func(context.Context, func(bool)) error
}

func (n *Node) setWallet(ctx context.Context, walletKey *wallet.Key) error {
	_, err := n.fullApi.WalletImport(ctx, &walletKey.KeyInfo)
	if err != nil {
		return err
	}

	err = n.fullApi.WalletSetDefault(ctx, walletKey.Address)
	if err != nil {
		return err
	}

	return nil
}

func waitForBalances(t *TestEnvironment, ctx context.Context, nodes int) ([]*InitialBalanceMsg, error) {
	ch := make(chan *InitialBalanceMsg)
	sub := t.SyncClient.MustSubscribe(ctx, balanceTopic, ch)

	balances := make([]*InitialBalanceMsg, 0, nodes)
	for i := 0; i < nodes; i++ {
		select {
		case m := <-ch:
			balances = append(balances, m)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for balances: %w", err)
		}
	}

	return balances, nil
}

func collectPreseals(t *TestEnvironment, ctx context.Context, miners int) ([]*PresealMsg, error) {
	ch := make(chan *PresealMsg)
	sub := t.SyncClient.MustSubscribe(ctx, presealTopic, ch)

	preseals := make([]*PresealMsg, 0, miners)
	for i := 0; i < miners; i++ {
		select {
		case m := <-ch:
			preseals = append(preseals, m)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for preseals: %w", err)
		}
	}

	sort.Slice(preseals, func(i, j int) bool {
		return preseals[i].Seqno < preseals[j].Seqno
	})

	return preseals, nil
}

func waitForGenesis(t *TestEnvironment, ctx context.Context) (*GenesisMsg, error) {
	genesisCh := make(chan *GenesisMsg)
	sub := t.SyncClient.MustSubscribe(ctx, genesisTopic, genesisCh)

	select {
	case genesisMsg := <-genesisCh:
		return genesisMsg, nil
	case err := <-sub.Done():
		return nil, fmt.Errorf("error while waiting for genesis msg: %w", err)
	}
}

func collectMinerAddrs(t *TestEnvironment, ctx context.Context, miners int) ([]MinerAddressesMsg, error) {
	ch := make(chan MinerAddressesMsg)
	sub := t.SyncClient.MustSubscribe(ctx, minersAddrsTopic, ch)

	addrs := make([]MinerAddressesMsg, 0, miners)
	for i := 0; i < miners; i++ {
		select {
		case a := <-ch:
			addrs = append(addrs, a)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for miners addrs: %w", err)
		}
	}

	return addrs, nil
}

func collectClientAddrs(t *TestEnvironment, ctx context.Context, clients int) ([]peer.AddrInfo, error) {
	ch := make(chan peer.AddrInfo)
	sub := t.SyncClient.MustSubscribe(ctx, clientsAddrsTopic, ch)

	addrs := make([]peer.AddrInfo, 0, clients)
	for i := 0; i < clients; i++ {
		select {
		case a := <-ch:
			addrs = append(addrs, a)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for clients addrs: %w", err)
		}
	}

	return addrs, nil
}

func getPubsubTracerMaddr(ctx context.Context, t *TestEnvironment) (string, error) {
	if !t.BooleanParam("enable_pubsub_tracer") {
		return "", nil
	}

	ch := make(chan *PubsubTracerMsg)
	sub := t.SyncClient.MustSubscribe(ctx, pubsubTracerTopic, ch)

	select {
	case m := <-ch:
		return m.Multiaddr, nil
	case err := <-sub.Done():
		return "", fmt.Errorf("got error while waiting for pubsub tracer config: %w", err)
	}
}

func getDrandOpts(ctx context.Context, t *TestEnvironment) (node.Option, error) {
	beaconType := t.StringParam("random_beacon_type")
	switch beaconType {
	case "external-drand":
		noop := func(settings *node.Settings) error {
			return nil
		}
		return noop, nil

	case "local-drand":
		cfg, err := waitForDrandConfig(ctx, t.SyncClient)
		if err != nil {
			t.RecordMessage("error getting drand config: %w", err)
			return nil, err

		}
		t.RecordMessage("setting drand config: %v", cfg)
		return node.Options(
			node.Override(new(dtypes.DrandConfig), cfg.Config),
			node.Override(new(dtypes.DrandBootstrap), cfg.GossipBootstrap),
		), nil

	case "mock":
		return node.Options(
			node.Override(new(beacon.RandomBeacon), modtest.RandomBeacon),
			node.Override(new(dtypes.DrandConfig), dtypes.DrandConfig{
				ChainInfoJSON: "{\"Hash\":\"wtf\"}",
			}),
			node.Override(new(dtypes.DrandBootstrap), dtypes.DrandBootstrap{}),
		), nil

	default:
		return nil, fmt.Errorf("unknown random_beacon_type: %s", beaconType)
	}
}

func startServer(repo *repo.MemRepo, srv *http.Server) error {
	endpoint, err := repo.APIEndpoint()
	if err != nil {
		return err
	}

	lst, err := manet.Listen(endpoint)
	if err != nil {
		return fmt.Errorf("could not listen: %w", err)
	}

	go func() {
		_ = srv.Serve(manet.NetListener(lst))
	}()

	return nil
}

func registerAndExportMetrics(instanceName string) {
	// Register all Lotus metric views
	err := view.Register(metrics.DefaultViews...)
	if err != nil {
		panic(err)
	}

	// Set the metric to one so it is published to the exporter
	stats.Record(context.Background(), metrics.LotusInfo.M(1))

	// Register our custom exporter to opencensus
	e, err := influxdb.NewExporter(context.Background(), influxdb.Options{
		Database:     "testground",
		Address:      os.Getenv("INFLUXDB_URL"),
		Username:     "",
		Password:     "",
		InstanceName: instanceName,
	})
	if err != nil {
		panic(err)
	}
	view.RegisterExporter(e)
	view.SetReportingPeriod(5 * time.Second)
}
