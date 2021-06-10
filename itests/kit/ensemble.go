package kit

import (
	"bytes"
	"context"
	"crypto/rand"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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
	"github.com/filecoin-project/lotus/api/client"
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
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"
)

func init() {
	chain.BootstrapPeerThreshold = 1
	messagepool.HeadChangeCoalesceMinDelay = time.Microsecond
	messagepool.HeadChangeCoalesceMaxDelay = 2 * time.Microsecond
	messagepool.HeadChangeCoalesceMergeInterval = 100 * time.Nanosecond
}

type BuilderOpt func(opts *BuilderOpts) error

type BuilderOpts struct {
	pastOffset time.Duration
	spt        abi.RegisteredSealProof
}

var DefaultBuilderOpts = BuilderOpts{
	pastOffset: 10000 * time.Second,
	spt:        abi.RegisteredSealProof_StackedDrg2KiBV1,
}

func ProofType(proofType abi.RegisteredSealProof) BuilderOpt {
	return func(opts *BuilderOpts) error {
		opts.spt = proofType
		return nil
	}
}

// Ensemble is a collection of nodes instantiated within a test. Ensemble
// supports building full nodes and miners.
type Ensemble struct {
	t            *testing.T
	bootstrapped bool
	genesisBlock bytes.Buffer
	mn           mocknet.Mocknet
	options      *BuilderOpts

	inactive struct {
		fullnodes []*TestFullNode
		miners    []*TestMiner
	}
	active struct {
		fullnodes []*TestFullNode
		miners    []*TestMiner
	}
	genesis struct {
		miners   []genesis.Miner
		accounts []genesis.Actor
	}
}

// NewEnsemble
func NewEnsemble(t *testing.T, opts ...BuilderOpt) *Ensemble {
	options := DefaultBuilderOpts
	for _, o := range opts {
		err := o(&options)
		require.NoError(t, err)
	}
	return &Ensemble{t: t, options: &options}
}

type NodeOpts struct {
	balance       abi.TokenAmount
	lite          bool
	sectors       int
	mockProofs    bool
	rpc           bool
	ownerKey      *wallet.Key
	extraNodeOpts []node.Option
}

const DefaultPresealsPerBootstrapMiner = 2

var DefaultNodeOpts = NodeOpts{
	balance: big.Mul(big.NewInt(100000000), types.NewInt(build.FilecoinPrecision)),
	sectors: DefaultPresealsPerBootstrapMiner,
}

type NodeOpt func(opts *NodeOpts) error

// OwnerBalance specifies the balance to be attributed to a miner's owner account.
//
// Only used when creating a miner.
func OwnerBalance(balance abi.TokenAmount) NodeOpt {
	return func(opts *NodeOpts) error {
		opts.balance = balance
		return nil
	}
}

// LiteNode specifies that this node will be a lite node.
//
// Only used when creating a fullnode.
func LiteNode() NodeOpt {
	return func(opts *NodeOpts) error {
		opts.lite = true
		return nil
	}
}

// PresealSectors specifies the amount of preseal sectors to give to a miner
// at genesis.
//
// Only used when creating a miner.
func PresealSectors(sectors int) NodeOpt {
	return func(opts *NodeOpts) error {
		opts.sectors = sectors
		return nil
	}
}

// MockProofs activates mock proofs for the entire ensemble.
func MockProofs() NodeOpt {
	return func(opts *NodeOpts) error {
		opts.mockProofs = true
		return nil
	}
}

func ThroughRPC() NodeOpt {
	return func(opts *NodeOpts) error {
		opts.rpc = true
		return nil
	}
}

func OwnerAddr(wk *wallet.Key) NodeOpt {
	return func(opts *NodeOpts) error {
		opts.ownerKey = wk
		return nil
	}
}

func ExtraNodeOpts(extra ...node.Option) NodeOpt {
	return func(opts *NodeOpts) error {
		opts.extraNodeOpts = extra
		return nil
	}
}

// FullNode enrolls a new full node.
func (n *Ensemble) FullNode(full *TestFullNode, opts ...NodeOpt) *Ensemble {
	options := DefaultNodeOpts
	for _, o := range opts {
		err := o(&options)
		require.NoError(n.t, err)
	}

	var key *wallet.Key
	if !n.bootstrapped && !options.balance.IsZero() {
		// create a key+ddress, and assign it some FIL.
		// this will be set as the default wallet.
		var err error
		key, err = wallet.GenerateKey(types.KTBLS)
		require.NoError(n.t, err)

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
func (n *Ensemble) Miner(miner *TestMiner, full *TestFullNode, opts ...NodeOpt) *Ensemble {
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

	ownerKey := options.ownerKey
	if !n.bootstrapped {
		var (
			sectors = options.sectors
			k       *types.KeyInfo
			genm    *genesis.Miner
		)

		// create the preseal commitment.
		if options.mockProofs {
			genm, k, err = mockstorage.PreSeal(abi.RegisteredSealProof_StackedDrg2KiBV1, actorAddr, sectors)
		} else {
			genm, k, err = seed.PreSeal(actorAddr, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, sectors, tdir, []byte("make genesis mem random"), nil, true)
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

	*miner = TestMiner{
		t:          n.t,
		ActorAddr:  actorAddr,
		OwnerKey:   ownerKey,
		FullNode:   full,
		PresealDir: tdir,
		options:    options,
	}

	miner.Libp2p.PeerID = peerId
	miner.Libp2p.PrivKey = privkey

	n.inactive.miners = append(n.inactive.miners, miner)

	return n
}

// Start starts all enrolled nodes.
func (n *Ensemble) Start() *Ensemble {
	ctx, cancel := context.WithCancel(context.Background())
	n.t.Cleanup(cancel)

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
		opts := []node.Option{
			node.FullAPI(&full.FullNode, node.Lite(full.options.lite)),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			node.MockHost(n.mn),
			node.Test(),

			// so that we subscribe to pubsub topics immediately
			node.Override(new(dtypes.Bootstrapper), dtypes.Bootstrapper(true)),
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
		if full.options.mockProofs {
			opts = append(opts,
				node.Override(new(ffiwrapper.Verifier), mock.MockVerifier),
				node.Override(new(ffiwrapper.Prover), mock.MockProver),
			)
		}

		// Construct the full node.
		stop, err := node.New(ctx, opts...)

		// fullOpts[i].Opts(fulls),
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
			// this is a miner created after genesis, so it won't have a preseal.
			// we need to create it on chain.
			params, aerr := actors.SerializeParams(&power2.CreateMinerParams{
				Owner:         m.OwnerKey.Address,
				Worker:        m.OwnerKey.Address,
				SealProofType: n.options.spt,
				Peer:          abi.PeerID(m.Libp2p.PeerID),
			})
			require.NoError(n.t, aerr)

			createStorageMinerMsg := &types.Message{
				From:  m.OwnerKey.Address,
				To:    power.Address,
				Value: big.Zero(),

				Method: power.Methods.CreateMiner,
				Params: params,

				GasLimit:   0,
				GasPremium: big.NewInt(5252),
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

		ks, err := lr.KeyStore()
		require.NoError(n.t, err)

		pk, err := m.Libp2p.PrivKey.Bytes()
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

		enc, err := actors.SerializeParams(&miner2.ChangePeerIDParams{NewID: abi.PeerID(m.Libp2p.PeerID)})
		require.NoError(n.t, err)

		msg := &types.Message{
			From:   m.OwnerKey.Address,
			To:     m.ActorAddr,
			Method: miner.Methods.ChangePeerID,
			Params: enc,
			Value:  types.NewInt(0),
		}

		_, err = m.FullNode.MpoolPushMessage(ctx, msg, nil)
		require.NoError(n.t, err)

		var mineBlock = make(chan lotusminer.MineReq)
		opts := []node.Option{
			node.StorageMiner(&m.StorageMiner),
			node.Online(),
			node.Repo(r),
			node.Test(),

			node.MockHost(n.mn),

			node.Override(new(v1api.FullNode), m.FullNode),
			node.Override(new(*lotusminer.Miner), lotusminer.NewTestMiner(mineBlock, m.ActorAddr)),
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

		if m.options.mockProofs {
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
		if !n.bootstrapped && !m.options.mockProofs {
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
func (n *Ensemble) Connect(from api.Common, to ...api.Common) *Ensemble {
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
		miners = n.active.miners
	}

	for _, m := range miners {
		bm := NewBlockMiner(n.t, m)
		bm.MineBlocks(ctx, blocktime)
		n.t.Cleanup(bm.Stop)

		bms = append(bms, bm)
	}

	return bms
}

func (n *Ensemble) generateGenesis() *genesis.Template {
	templ := &genesis.Template{
		Accounts:         n.genesis.accounts,
		Miners:           n.genesis.miners,
		NetworkName:      "test",
		Timestamp:        uint64(time.Now().Unix() - 10000), // some time sufficiently far in the past
		VerifregRootKey:  gen.DefaultVerifregRootkeyActor,
		RemainderAccount: gen.DefaultRemainderAccountActor,
	}

	return templ
}

func CreateRPCServer(t *testing.T, handler http.Handler) (*httptest.Server, multiaddr.Multiaddr) {
	testServ := httptest.NewServer(handler)
	t.Cleanup(testServ.Close)
	t.Cleanup(testServ.CloseClientConnections)

	addr := testServ.Listener.Addr()
	maddr, err := manet.FromNetAddr(addr)
	require.NoError(t, err)
	return testServ, maddr
}

func fullRpc(t *testing.T, f *TestFullNode) *TestFullNode {
	handler, err := node.FullNodeHandler(f.FullNode, false)
	require.NoError(t, err)

	srv, maddr := CreateRPCServer(t, handler)

	cl, stop, err := client.NewFullNodeRPCV1(context.Background(), "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil)
	require.NoError(t, err)
	t.Cleanup(stop)
	f.ListenAddr, f.FullNode = maddr, cl

	return f
}

func minerRpc(t *testing.T, m *TestMiner) *TestMiner {
	handler, err := node.MinerHandler(m.StorageMiner, false)
	require.NoError(t, err)

	srv, maddr := CreateRPCServer(t, handler)

	cl, stop, err := client.NewStorageMinerRPCV0(context.Background(), "ws://"+srv.Listener.Addr().String()+"/rpc/v0", nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	m.ListenAddr, m.StorageMiner = maddr, cl
	return m
}

func LatestActorsAt(upgradeHeight abi.ChainEpoch) node.Option {
	// Attention: Update this when introducing new actor versions or your tests will be sad
	return NetworkUpgradeAt(network.Version13, upgradeHeight)
}

func NetworkUpgradeAt(version network.Version, upgradeHeight abi.ChainEpoch) node.Option {
	fullSchedule := stmgr.UpgradeSchedule{{
		// prepare for upgrade.
		Network:   network.Version9,
		Height:    1,
		Migration: stmgr.UpgradeActorsV2,
	}, {
		Network:   network.Version10,
		Height:    2,
		Migration: stmgr.UpgradeActorsV3,
	}, {
		Network:   network.Version12,
		Height:    3,
		Migration: stmgr.UpgradeActorsV4,
	}, {
		Network:   network.Version13,
		Height:    4,
		Migration: stmgr.UpgradeActorsV5,
	}}

	schedule := stmgr.UpgradeSchedule{}
	for _, upgrade := range fullSchedule {
		if upgrade.Network > version {
			break
		}

		schedule = append(schedule, upgrade)
	}

	if upgradeHeight > 0 {
		schedule[len(schedule)-1].Height = upgradeHeight
	}

	return node.Override(new(stmgr.UpgradeSchedule), schedule)
}

func SDRUpgradeAt(calico, persian abi.ChainEpoch) node.Option {
	return node.Override(new(stmgr.UpgradeSchedule), stmgr.UpgradeSchedule{{
		Network:   network.Version6,
		Height:    1,
		Migration: stmgr.UpgradeActorsV2,
	}, {
		Network:   network.Version7,
		Height:    calico,
		Migration: stmgr.UpgradeCalico,
	}, {
		Network: network.Version8,
		Height:  persian,
	}})

}
