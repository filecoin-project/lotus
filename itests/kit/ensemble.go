package kit

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-storedcounter"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/gen"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	sectorstorage "github.com/filecoin-project/lotus/extern/sector-storage"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/mock"
	"github.com/filecoin-project/lotus/genesis"
	lotusminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	testing2 "github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/mockstorage"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	"github.com/ipfs/go-datastore"
	libp2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
)

func init() {
	chain.BootstrapPeerThreshold = 1
	messagepool.HeadChangeCoalesceMinDelay = time.Microsecond
	messagepool.HeadChangeCoalesceMaxDelay = 2 * time.Microsecond
	messagepool.HeadChangeCoalesceMergeInterval = 100 * time.Nanosecond
}

// Ensemble is a collection of nodes instantiated within a test.
//
// Create a new ensemble with:
//
//   ens := kit.NewEnsemble()
//
// Create full nodes and miners:
//
//   var full TestFullNode
//   var miner TestMiner
//   ens.FullNode(&full, opts...)       // populates a full node
//   ens.Miner(&miner, &full, opts...)  // populates a miner, using the full node as its chain daemon
//
// It is possible to pass functional options to set initial balances,
// presealed sectors, owner keys, etc.
//
// After the initial nodes are added, call `ens.Start()` to forge genesis
// and start the network. Mining will NOT be started automatically. It needs
// to be started explicitly by calling `BeginMining`.
//
// Nodes also need to be connected with one another, either via `ens.Connect()`
// or `ens.InterconnectAll()`. A common inchantation for simple tests is to do:
//
//   ens.InterconnectAll().BeginMining(blocktime)
//
// You can continue to add more nodes, but you must always follow with
// `ens.Start()` to activate the new nodes.
//
// The API is chainable, so it's possible to do a lot in a very succinct way:
//
//   kit.NewEnsemble().FullNode(&full).Miner(&miner, &full).Start().InterconnectAll().BeginMining()
//
// You can also find convenient fullnode:miner presets, such as 1:1, 1:2,
// and 2:1, e.g.:
//
//   kit.EnsembleMinimal()
//   kit.EnsembleOneTwo()
//   kit.EnsembleTwoOne()
//
type Ensemble struct {
	t            *testing.T
	bootstrapped bool
	genesisBlock bytes.Buffer
	mn           mocknet.Mocknet
	options      *ensembleOpts

	inactive struct {
		fullnodes []*TestFullNode
		miners    []*TestMiner
	}
	active struct {
		fullnodes []*TestFullNode
		miners    []*TestMiner
		bms       map[*TestMiner]*BlockMiner
	}
	genesis struct {
		version  network.Version
		miners   []genesis.Miner
		accounts []genesis.Actor
	}
}

// NewEnsemble instantiates a new blank Ensemble.
func NewEnsemble(t *testing.T, opts ...EnsembleOpt) *Ensemble {
	options := DefaultEnsembleOpts
	for _, o := range opts {
		err := o(&options)
		require.NoError(t, err)
	}

	n := &Ensemble{t: t, options: &options}
	n.active.bms = make(map[*TestMiner]*BlockMiner)

	for _, up := range options.upgradeSchedule {
		if up.Height < 0 {
			n.genesis.version = up.Network
		}
	}

	// add accounts from ensemble options to genesis.
	for _, acc := range options.accounts {
		n.genesis.accounts = append(n.genesis.accounts, genesis.Actor{
			Type:    genesis.TAccount,
			Balance: acc.initialBalance,
			Meta:    (&genesis.AccountMeta{Owner: acc.key.Address}).ActorMeta(),
		})
	}

	return n
}

// FullNode enrolls a new full node.
func (n *Ensemble) FullNode(full *TestFullNode, opts ...NodeOpt) *Ensemble {
	options := DefaultNodeOpts
	for _, o := range opts {
		err := o(&options)
		require.NoError(n.t, err)
	}

	key, err := wallet.GenerateKey(types.KTBLS)
	require.NoError(n.t, err)

	if !n.bootstrapped && !options.balance.IsZero() {
		// if we still haven't forged genesis, create a key+address, and assign
		// it some FIL; this will be set as the default wallet when the node is
		// started.
		genacc := genesis.Actor{
			Type:    genesis.TAccount,
			Balance: options.balance,
			Meta:    (&genesis.AccountMeta{Owner: key.Address}).ActorMeta(),
		}

		n.genesis.accounts = append(n.genesis.accounts, genacc)
	}

	*full = TestFullNode{t: n.t, options: options, DefaultKey: key}
	n.inactive.fullnodes = append(n.inactive.fullnodes, full)
	return n
}

// Miner enrolls a new miner, using the provided full node for chain
// interactions.
func (n *Ensemble) Miner(minerNode *TestMiner, full *TestFullNode, opts ...NodeOpt) *Ensemble {
	require.NotNil(n.t, full, "full node required when instantiating miner")

	options := DefaultNodeOpts
	for _, o := range opts {
		err := o(&options)
		require.NoError(n.t, err)
	}

	privkey, _, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	require.NoError(n.t, err)

	peerId, err := peer.IDFromPrivateKey(privkey)
	require.NoError(n.t, err)

	tdir, err := ioutil.TempDir("", "preseal-memgen")
	require.NoError(n.t, err)

	minerCnt := len(n.inactive.miners) + len(n.active.miners)

	actorAddr, err := address.NewIDAddress(genesis2.MinerStart + uint64(minerCnt))
	require.NoError(n.t, err)

	if options.mainMiner != nil {
		actorAddr = options.mainMiner.ActorAddr
	}

	ownerKey := options.ownerKey
	if !n.bootstrapped {
		var (
			sectors = options.sectors
			k       *types.KeyInfo
			genm    *genesis.Miner
		)

		// Will use 2KiB sectors by default (default value of sectorSize).
		proofType, err := miner.SealProofTypeFromSectorSize(options.sectorSize, n.genesis.version)
		require.NoError(n.t, err)

		// Create the preseal commitment.
		if n.options.mockProofs {
			genm, k, err = mockstorage.PreSeal(proofType, actorAddr, sectors)
		} else {
			genm, k, err = seed.PreSeal(actorAddr, proofType, 0, sectors, tdir, []byte("make genesis mem random"), nil, true)
		}
		require.NoError(n.t, err)

		genm.PeerId = peerId

		// create an owner key, and assign it some FIL.
		ownerKey, err = wallet.NewKey(*k)
		require.NoError(n.t, err)

		genacc := genesis.Actor{
			Type:    genesis.TAccount,
			Balance: options.balance,
			Meta:    (&genesis.AccountMeta{Owner: ownerKey.Address}).ActorMeta(),
		}

		n.genesis.miners = append(n.genesis.miners, *genm)
		n.genesis.accounts = append(n.genesis.accounts, genacc)
	} else {
		require.NotNil(n.t, ownerKey, "worker key can't be null if initializing a miner after genesis")
	}

	rl, err := net.Listen("tcp", "127.0.0.1:")
	require.NoError(n.t, err)

	*minerNode = TestMiner{
		t:              n.t,
		ActorAddr:      actorAddr,
		OwnerKey:       ownerKey,
		FullNode:       full,
		PresealDir:     tdir,
		options:        options,
		RemoteListener: rl,
	}

	minerNode.Libp2p.PeerID = peerId
	minerNode.Libp2p.PrivKey = privkey

	n.inactive.miners = append(n.inactive.miners, minerNode)

	return n
}

// Start starts all enrolled nodes.
func (n *Ensemble) Start() *Ensemble {
	ctx := context.Background()

	var gtempl *genesis.Template
	if !n.bootstrapped {
		// We haven't been bootstrapped yet, we need to generate genesis and
		// create the networking backbone.
		gtempl = n.generateGenesis()
		n.mn = mocknet.New(ctx)
	}

	// ---------------------
	//  FULL NODES
	// ---------------------

	// Create all inactive full nodes.
	for i, full := range n.inactive.fullnodes {
		r := repo.NewMemory(nil)
		opts := []node.Option{
			node.FullAPI(&full.FullNode, node.Lite(full.options.lite)),
			node.Base(),
			node.Repo(r),
			node.MockHost(n.mn),
			node.Test(),

			// so that we subscribe to pubsub topics immediately
			node.Override(new(dtypes.Bootstrapper), dtypes.Bootstrapper(true)),

			// upgrades
			node.Override(new(stmgr.UpgradeSchedule), n.options.upgradeSchedule),
		}

		// append any node builder options.
		opts = append(opts, full.options.extraNodeOpts...)

		// Either generate the genesis or inject it.
		if i == 0 && !n.bootstrapped {
			opts = append(opts, node.Override(new(modules.Genesis), testing2.MakeGenesisMem(&n.genesisBlock, *gtempl)))
		} else {
			opts = append(opts, node.Override(new(modules.Genesis), modules.LoadGenesis(n.genesisBlock.Bytes())))
		}

		// Are we mocking proofs?
		if n.options.mockProofs {
			opts = append(opts,
				node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),
				node.Override(new(ffiwrapper.Prover), mock.MockProver),
			)
		}

		// Call option builders, passing active nodes as the parameter
		for _, bopt := range full.options.optBuilders {
			opts = append(opts, bopt(n.active.fullnodes))
		}

		// Construct the full node.
		stop, err := node.New(ctx, opts...)

		require.NoError(n.t, err)

		addr, err := full.WalletImport(context.Background(), &full.DefaultKey.KeyInfo)
		require.NoError(n.t, err)

		err = full.WalletSetDefault(context.Background(), addr)
		require.NoError(n.t, err)

		// Are we hitting this node through its RPC?
		if full.options.rpc {
			withRPC := fullRpc(n.t, full)
			n.inactive.fullnodes[i] = withRPC
		}

		n.t.Cleanup(func() { _ = stop(context.Background()) })

		n.active.fullnodes = append(n.active.fullnodes, full)
	}

	// If we are here, we have processed all inactive fullnodes and moved them
	// to active, so clear the slice.
	n.inactive.fullnodes = n.inactive.fullnodes[:0]

	// Link all the nodes.
	err := n.mn.LinkAll()
	require.NoError(n.t, err)

	// ---------------------
	//  MINERS
	// ---------------------

	// Create all inactive miners.
	for i, m := range n.inactive.miners {
		if n.bootstrapped {
			if m.options.mainMiner == nil {
				// this is a miner created after genesis, so it won't have a preseal.
				// we need to create it on chain.

				// we get the proof type for the requested sector size, for
				// the current network version.
				nv, err := m.FullNode.FullNode.StateNetworkVersion(ctx, types.EmptyTSK)
				require.NoError(n.t, err)

				proofType, err := miner.SealProofTypeFromSectorSize(m.options.sectorSize, nv)
				require.NoError(n.t, err)

				params, aerr := actors.SerializeParams(&power2.CreateMinerParams{
					Owner:         m.OwnerKey.Address,
					Worker:        m.OwnerKey.Address,
					SealProofType: proofType,
					Peer:          abi.PeerID(m.Libp2p.PeerID),
				})
				require.NoError(n.t, aerr)

				createStorageMinerMsg := &types.Message{
					From:  m.OwnerKey.Address,
					To:    power.Address,
					Value: big.Zero(),

					Method: power.Methods.CreateMiner,
					Params: params,
				}
				signed, err := m.FullNode.FullNode.MpoolPushMessage(ctx, createStorageMinerMsg, nil)
				require.NoError(n.t, err)

				mw, err := m.FullNode.FullNode.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
				require.NoError(n.t, err)
				require.Equal(n.t, exitcode.Ok, mw.Receipt.ExitCode)

				var retval power2.CreateMinerReturn
				err = retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return))
				require.NoError(n.t, err, "failed to create miner")

				m.ActorAddr = retval.IDAddress
			} else {
				params, err := actors.SerializeParams(&miner2.ChangePeerIDParams{NewID: abi.PeerID(m.Libp2p.PeerID)})
				require.NoError(n.t, err)

				msg := &types.Message{
					To:     m.options.mainMiner.ActorAddr,
					From:   m.options.mainMiner.OwnerKey.Address,
					Method: miner.Methods.ChangePeerID,
					Params: params,
					Value:  types.NewInt(0),
				}

				signed, err2 := m.FullNode.FullNode.MpoolPushMessage(ctx, msg, nil)
				require.NoError(n.t, err2)

				mw, err2 := m.FullNode.FullNode.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence, api.LookbackNoLimit, true)
				require.NoError(n.t, err2)
				require.Equal(n.t, exitcode.Ok, mw.Receipt.ExitCode)
			}
		}

		has, err := m.FullNode.WalletHas(ctx, m.OwnerKey.Address)
		require.NoError(n.t, err)

		// Only import the owner's full key into our companion full node, if we
		// don't have it still.
		if !has {
			_, err = m.FullNode.WalletImport(ctx, &m.OwnerKey.KeyInfo)
			require.NoError(n.t, err)
		}

		// // Set it as the default address.
		// err = m.FullNode.WalletSetDefault(ctx, m.OwnerAddr.Address)
		// require.NoError(n.t, err)

		r := repo.NewMemory(nil)

		lr, err := r.Lock(repo.StorageMiner)
		require.NoError(n.t, err)

		c, err := lr.Config()
		require.NoError(n.t, err)

		cfg, ok := c.(*config.StorageMiner)
		if !ok {
			n.t.Fatalf("invalid config from repo, got: %T", c)
		}
		cfg.Common.API.RemoteListenAddress = m.RemoteListener.Addr().String()
		cfg.Subsystems.EnableMarkets = m.options.subsystems.Has(SMarkets)
		cfg.Subsystems.EnableMining = m.options.subsystems.Has(SMining)
		cfg.Subsystems.EnableSealing = m.options.subsystems.Has(SSealing)
		cfg.Subsystems.EnableSectorStorage = m.options.subsystems.Has(SSectorStorage)
		cfg.Dealmaking.MaxStagingDealsBytes = m.options.maxStagingDealsBytes

		if m.options.mainMiner != nil {
			token, err := m.options.mainMiner.FullNode.AuthNew(ctx, api.AllPermissions)
			require.NoError(n.t, err)

			cfg.Subsystems.SectorIndexApiInfo = fmt.Sprintf("%s:%s", token, m.options.mainMiner.ListenAddr)
			cfg.Subsystems.SealerApiInfo = fmt.Sprintf("%s:%s", token, m.options.mainMiner.ListenAddr)

			fmt.Println("config for market node, setting SectorIndexApiInfo to: ", cfg.Subsystems.SectorIndexApiInfo)
			fmt.Println("config for market node, setting SealerApiInfo to: ", cfg.Subsystems.SealerApiInfo)
		}

		err = lr.SetConfig(func(raw interface{}) {
			rcfg := raw.(*config.StorageMiner)
			*rcfg = *cfg
		})
		require.NoError(n.t, err)

		ks, err := lr.KeyStore()
		require.NoError(n.t, err)

		pk, err := libp2pcrypto.MarshalPrivateKey(m.Libp2p.PrivKey)
		require.NoError(n.t, err)

		err = ks.Put("libp2p-host", types.KeyInfo{
			Type:       "libp2p-host",
			PrivateKey: pk,
		})
		require.NoError(n.t, err)

		ds, err := lr.Datastore(context.TODO(), "/metadata")
		require.NoError(n.t, err)

		err = ds.Put(datastore.NewKey("miner-address"), m.ActorAddr.Bytes())
		require.NoError(n.t, err)

		nic := storedcounter.New(ds, datastore.NewKey(modules.StorageCounterDSPrefix))
		for i := 0; i < m.options.sectors; i++ {
			_, err := nic.Next()
			require.NoError(n.t, err)
		}
		_, err = nic.Next()
		require.NoError(n.t, err)

		err = lr.Close()
		require.NoError(n.t, err)

		if m.options.mainMiner == nil {
			enc, err := actors.SerializeParams(&miner2.ChangePeerIDParams{NewID: abi.PeerID(m.Libp2p.PeerID)})
			require.NoError(n.t, err)

			msg := &types.Message{
				From:   m.OwnerKey.Address,
				To:     m.ActorAddr,
				Method: miner.Methods.ChangePeerID,
				Params: enc,
				Value:  types.NewInt(0),
			}

			_, err2 := m.FullNode.MpoolPushMessage(ctx, msg, nil)
			require.NoError(n.t, err2)
		}

		var mineBlock = make(chan lotusminer.MineReq)
		opts := []node.Option{
			node.StorageMiner(&m.StorageMiner, cfg.Subsystems),
			node.Base(),
			node.Repo(r),
			node.Test(),

			node.If(!m.options.disableLibp2p, node.MockHost(n.mn)),

			node.Override(new(v1api.FullNode), m.FullNode.FullNode),
			node.Override(new(*lotusminer.Miner), lotusminer.NewTestMiner(mineBlock, m.ActorAddr)),

			// disable resource filtering so that local worker gets assigned tasks
			// regardless of system pressure.
			node.Override(new(sectorstorage.SealerConfig), func() sectorstorage.SealerConfig {
				scfg := config.DefaultStorageMiner()
				scfg.Storage.ResourceFiltering = sectorstorage.ResourceFilteringDisabled
				return scfg.Storage
			}),

			// upgrades
			node.Override(new(stmgr.UpgradeSchedule), n.options.upgradeSchedule),
		}

		// append any node builder options.
		opts = append(opts, m.options.extraNodeOpts...)

		idAddr, err := address.IDFromAddress(m.ActorAddr)
		require.NoError(n.t, err)

		// preload preseals if the network still hasn't bootstrapped.
		var presealSectors []abi.SectorID
		if !n.bootstrapped {
			sectors := n.genesis.miners[i].Sectors
			for _, sector := range sectors {
				presealSectors = append(presealSectors, abi.SectorID{
					Miner:  abi.ActorID(idAddr),
					Number: sector.SectorID,
				})
			}
		}

		if n.options.mockProofs {
			opts = append(opts,
				node.Override(new(*mock.SectorMgr), func() (*mock.SectorMgr, error) {
					return mock.NewMockSectorMgr(presealSectors), nil
				}),
				node.Override(new(sectorstorage.SectorManager), node.From(new(*mock.SectorMgr))),
				node.Override(new(sectorstorage.Unsealer), node.From(new(*mock.SectorMgr))),
				node.Override(new(sectorstorage.PieceProvider), node.From(new(*mock.SectorMgr))),

				node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),
				node.Override(new(ffiwrapper.Prover), mock.MockProver),
				node.Unset(new(*sectorstorage.Manager)),
			)
		}

		// start node
		stop, err := node.New(ctx, opts...)
		require.NoError(n.t, err)

		// using real proofs, therefore need real sectors.
		if !n.bootstrapped && !n.options.mockProofs {
			err := m.StorageAddLocal(ctx, m.PresealDir)
			require.NoError(n.t, err)
		}

		n.t.Cleanup(func() { _ = stop(context.Background()) })

		// Are we hitting this node through its RPC?
		if m.options.rpc {
			withRPC := minerRpc(n.t, m)
			n.inactive.miners[i] = withRPC
		}

		mineOne := func(ctx context.Context, req lotusminer.MineReq) error {
			select {
			case mineBlock <- req:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		m.MineOne = mineOne
		m.Stop = stop

		n.active.miners = append(n.active.miners, m)
	}

	// If we are here, we have processed all inactive miners and moved them
	// to active, so clear the slice.
	n.inactive.miners = n.inactive.miners[:0]

	// Link all the nodes.
	err = n.mn.LinkAll()
	require.NoError(n.t, err)

	if !n.bootstrapped && len(n.active.miners) > 0 {
		// We have *just* bootstrapped, so mine 2 blocks to setup some CE stuff in some actors
		var wait sync.Mutex
		wait.Lock()

		observer := n.active.fullnodes[0]

		bm := NewBlockMiner(n.t, n.active.miners[0])
		n.t.Cleanup(bm.Stop)

		bm.MineUntilBlock(ctx, observer, func(epoch abi.ChainEpoch) {
			wait.Unlock()
		})
		wait.Lock()
		bm.MineUntilBlock(ctx, observer, func(epoch abi.ChainEpoch) {
			wait.Unlock()
		})
		wait.Lock()
	}

	n.bootstrapped = true
	return n
}

// InterconnectAll connects all miners and full nodes to one another.
func (n *Ensemble) InterconnectAll() *Ensemble {
	// connect full nodes to miners.
	for _, from := range n.active.fullnodes {
		for _, to := range n.active.miners {
			// []*TestMiner to []api.CommonAPI type coercion not possible
			// so cannot use variadic form.
			n.Connect(from, to)
		}
	}

	// connect full nodes between each other, skipping ourselves.
	last := len(n.active.fullnodes) - 1
	for i, from := range n.active.fullnodes {
		if i == last {
			continue
		}
		for _, to := range n.active.fullnodes[i+1:] {
			n.Connect(from, to)
		}
	}
	return n
}

// Connect connects one full node to the provided full nodes.
func (n *Ensemble) Connect(from api.Net, to ...api.Net) *Ensemble {
	addr, err := from.NetAddrsListen(context.Background())
	require.NoError(n.t, err)

	for _, other := range to {
		err = other.NetConnect(context.Background(), addr)
		require.NoError(n.t, err)
	}
	return n
}

// BeginMining kicks off mining for the specified miners. If nil or 0-length,
// it will kick off mining for all enrolled and active miners. It also adds a
// cleanup function to stop all mining operations on test teardown.
func (n *Ensemble) BeginMining(blocktime time.Duration, miners ...*TestMiner) []*BlockMiner {
	ctx := context.Background()

	// wait one second to make sure that nodes are connected and have handshaken.
	// TODO make this deterministic by listening to identify events on the
	//  libp2p eventbus instead (or something else).
	time.Sleep(1 * time.Second)

	var bms []*BlockMiner
	if len(miners) == 0 {
		// no miners have been provided explicitly, instantiate block miners
		// for all active miners that aren't still mining.
		for _, m := range n.active.miners {
			if _, ok := n.active.bms[m]; ok {
				continue // skip, already have a block miner
			}
			miners = append(miners, m)
		}
	}

	for _, m := range miners {
		bm := NewBlockMiner(n.t, m)
		bm.MineBlocks(ctx, blocktime)
		n.t.Cleanup(bm.Stop)

		bms = append(bms, bm)

		n.active.bms[m] = bm
	}

	return bms
}

func (n *Ensemble) generateGenesis() *genesis.Template {
	var verifRoot = gen.DefaultVerifregRootkeyActor
	if k := n.options.verifiedRoot.key; k != nil {
		verifRoot = genesis.Actor{
			Type:    genesis.TAccount,
			Balance: n.options.verifiedRoot.initialBalance,
			Meta:    (&genesis.AccountMeta{Owner: k.Address}).ActorMeta(),
		}
	}

	templ := &genesis.Template{
		NetworkVersion:   n.genesis.version,
		Accounts:         n.genesis.accounts,
		Miners:           n.genesis.miners,
		NetworkName:      "test",
		Timestamp:        uint64(time.Now().Unix() - int64(n.options.pastOffset.Seconds())),
		VerifregRootKey:  verifRoot,
		RemainderAccount: gen.DefaultRemainderAccountActor,
	}

	return templ
}
