package main

import (
	"bytes"
	"context"
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"

	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func runBootstrapper(t *TestEnvironment) error {
	t.RecordMessage("running bootstrapper")
	_, err := prepareBootstrapper(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func prepareBootstrapper(t *TestEnvironment) (*Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	pubsubTracer, err := getPubsubTracerMaddr(ctx, t)
	if err != nil {
		return nil, err
	}

	clients := t.IntParam("clients")
	miners := t.IntParam("miners")
	nodes := clients + miners

	drandOpt, err := getDrandOpts(ctx, t)
	if err != nil {
		return nil, err
	}

	// the first duty of the boostrapper is to construct the genesis block
	// first collect all client and miner balances to assign initial funds
	balances, err := waitForBalances(t, ctx, nodes)
	if err != nil {
		return nil, err
	}

	// then collect all preseals from miners
	preseals, err := collectPreseals(t, ctx, miners)
	if err != nil {
		return nil, err
	}

	// now construct the genesis block
	var genesisActors []genesis.Actor
	var genesisMiners []genesis.Miner

	for _, bm := range balances {
		genesisActors = append(genesisActors,
			genesis.Actor{
				Type:    genesis.TAccount,
				Balance: big.Mul(big.NewInt(int64(bm.Balance)), types.NewInt(build.FilecoinPrecision)),
				Meta:    (&genesis.AccountMeta{Owner: bm.Addr}).ActorMeta(),
			})
	}

	for _, pm := range preseals {
		genesisMiners = append(genesisMiners, pm.Miner)
	}

	genesisTemplate := genesis.Template{
		Accounts:  genesisActors,
		Miners:    genesisMiners,
		Timestamp: uint64(time.Now().Unix()) - uint64(t.IntParam("genesis_timestamp_offset")), // this needs to be in the past
	}

	// dump the genesis block
	// var jsonBuf bytes.Buffer
	// jsonEnc := json.NewEncoder(&jsonBuf)
	// err := jsonEnc.Encode(genesisTemplate)
	// if err != nil {
	// 	panic(err)
	// }
	// runenv.RecordMessage(fmt.Sprintf("Genesis template: %s", string(jsonBuf.Bytes())))

	// this is horrendously disgusting, we use this contraption to side effect the construction
	// of the genesis block in the buffer -- yes, a side effect of dependency injection.
	// I remember when software was straightforward...
	var genesisBuffer bytes.Buffer

	bootstrapperIP := t.NetClient.MustGetDataNetworkIP().String()

	n := &Node{}
	stop, err := node.New(context.Background(),
		node.FullAPI(&n.fullApi),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genesisBuffer, genesisTemplate)),
		withApiEndpoint("/ip4/127.0.0.1/tcp/1234"),
		withListenAddress(bootstrapperIP),
		withBootstrapper(nil),
		withPubsubConfig(true, pubsubTracer),
		drandOpt,
	)
	if err != nil {
		return nil, err
	}
	n.stop = stop

	var bootstrapperAddr ma.Multiaddr

	bootstrapperAddrs, err := n.fullApi.NetAddrsListen(ctx)
	if err != nil {
		stop(context.TODO())
		return nil, err
	}
	for _, a := range bootstrapperAddrs.Addrs {
		ip, err := a.ValueForProtocol(ma.P_IP4)
		if err != nil {
			continue
		}
		if ip != bootstrapperIP {
			continue
		}
		addrs, err := peer.AddrInfoToP2pAddrs(&peer.AddrInfo{
			ID:    bootstrapperAddrs.ID,
			Addrs: []ma.Multiaddr{a},
		})
		if err != nil {
			panic(err)
		}
		bootstrapperAddr = addrs[0]
		break
	}

	if bootstrapperAddr == nil {
		panic("failed to determine bootstrapper address")
	}

	genesisMsg := &GenesisMsg{
		Genesis:      genesisBuffer.Bytes(),
		Bootstrapper: bootstrapperAddr.Bytes(),
	}
	t.SyncClient.MustPublish(ctx, genesisTopic, genesisMsg)

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, stateReady, t.TestInstanceCount)

	return n, nil
}
