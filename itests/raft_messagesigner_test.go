package itests

import (
	"context"
	"crypto/rand"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	gorpc "github.com/libp2p/go-libp2p-gorpc"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagesigner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	consensus "github.com/filecoin-project/lotus/lib/consensus/raft"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
)

func generatePrivKey() (*kit.Libp2p, error) {
	privkey, _, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	peerId, err := peer.IDFromPrivateKey(privkey)
	if err != nil {
		return nil, err
	}

	return &kit.Libp2p{PeerID: peerId, PrivKey: privkey}, nil
}

func getRaftState(ctx context.Context, t *testing.T, node *kit.TestFullNode) *api.RaftStateData {
	raftState, err := node.RaftState(ctx)
	require.NoError(t, err)
	return raftState
}

func setup(ctx context.Context, t *testing.T, node0 *kit.TestFullNode, node1 *kit.TestFullNode, node2 *kit.TestFullNode, miner *kit.TestMiner) *kit.Ensemble {

	blockTime := 1 * time.Second

	pkey0, _ := generatePrivKey()
	pkey1, _ := generatePrivKey()
	pkey2, _ := generatePrivKey()

	pkeys := []*kit.Libp2p{pkey0, pkey1, pkey2}
	initPeerSet := []string{}
	for _, pkey := range pkeys {
		initPeerSet = append(initPeerSet, "/p2p/"+pkey.PeerID.String())
	}

	//initPeerSet := []peer.ID{pkey0.PeerID, pkey1.PeerID, pkey2.PeerID}

	raftOps := kit.ConstructorOpts(
		node.Override(new(*gorpc.Client), modules.NewRPCClient),
		node.Override(new(*consensus.ClusterRaftConfig), func() *consensus.ClusterRaftConfig {
			cfg := consensus.DefaultClusterRaftConfig()
			cfg.InitPeerset = initPeerSet
			return cfg
		}),
		node.Override(new(*consensus.Consensus), consensus.NewConsensusWithRPCClient(false)),
		node.Override(new(*messagesigner.MessageSignerConsensus), messagesigner.NewMessageSignerConsensus),
		node.Override(new(messagesigner.MsgSigner), func(ms *messagesigner.MessageSignerConsensus) *messagesigner.MessageSignerConsensus { return ms }),
		node.Override(new(*modules.RPCHandler), modules.NewRPCHandler),
		node.Override(node.GoRPCServer, modules.NewRPCServer),
	)
	//raftOps := kit.ConstructorOpts()

	ens := kit.NewEnsemble(t).FullNode(node0, raftOps, kit.ThroughRPC()).FullNode(node1, raftOps, kit.ThroughRPC()).FullNode(node2, raftOps, kit.ThroughRPC())
	node0.AssignPrivKey(pkey0)
	node1.AssignPrivKey(pkey1)
	node2.AssignPrivKey(pkey2)

	nodes := []*kit.TestFullNode{node0, node1, node2}
	wrappedFullNode := kit.MergeFullNodes(nodes)

	ens.MinerEnroll(miner, wrappedFullNode, kit.WithAllSubsystems(), kit.ThroughRPC())
	ens.Start()

	// Import miner wallet to all nodes
	addr0, err := node0.WalletImport(ctx, &miner.OwnerKey.KeyInfo)
	require.NoError(t, err)
	addr1, err := node1.WalletImport(ctx, &miner.OwnerKey.KeyInfo)
	require.NoError(t, err)
	addr2, err := node2.WalletImport(ctx, &miner.OwnerKey.KeyInfo)
	require.NoError(t, err)

	fmt.Println(addr0, addr1, addr2)

	ens.InterconnectAll()

	ens.AddInactiveMiner(miner)
	ens.Start()

	ens.InterconnectAll().BeginMining(blockTime)

	return ens
}

func TestRaftState(t *testing.T) {

	kit.QuietMiningLogs()
	ctx := context.Background()

	var (
		node0 kit.TestFullNode
		node1 kit.TestFullNode
		node2 kit.TestFullNode
		miner kit.TestMiner
	)

	setup(ctx, t, &node0, &node1, &node2, &miner)

	fmt.Println(node0.WalletList(context.Background()))
	fmt.Println(node1.WalletList(context.Background()))
	fmt.Println(node2.WalletList(context.Background()))

	bal, err := node0.WalletBalance(ctx, node0.DefaultKey.Address)
	require.NoError(t, err)

	msgHalfBal := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    node0.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}

	mu := uuid.New()
	smHalfBal, err := node0.MpoolPushMessage(ctx, msgHalfBal, &api.MessageSendSpec{
		MsgUuid: mu,
	})
	require.NoError(t, err)
	mLookup, err := node0.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	rstate0 := getRaftState(ctx, t, &node0)
	rstate1 := getRaftState(ctx, t, &node1)
	rstate2 := getRaftState(ctx, t, &node2)

	require.EqualValues(t, rstate0, rstate1)
	require.EqualValues(t, rstate0, rstate2)
}

func TestRaftStateLeaderDisconnects(t *testing.T) {

	kit.QuietMiningLogs()
	ctx := context.Background()

	var (
		node0 kit.TestFullNode
		node1 kit.TestFullNode
		node2 kit.TestFullNode
		miner kit.TestMiner
	)

	nodes := []*kit.TestFullNode{&node0, &node1, &node2}

	setup(ctx, t, &node0, &node1, &node2, &miner)

	peerToNode := make(map[peer.ID]*kit.TestFullNode)
	for _, n := range nodes {
		peerToNode[n.Pkey.PeerID] = n
	}

	bal, err := node0.WalletBalance(ctx, node0.DefaultKey.Address)
	require.NoError(t, err)

	msgHalfBal := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    node0.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	mu := uuid.New()
	smHalfBal, err := node0.MpoolPushMessage(ctx, msgHalfBal, &api.MessageSendSpec{
		MsgUuid: mu,
	})
	require.NoError(t, err)
	mLookup, err := node0.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	rstate0 := getRaftState(ctx, t, &node0)
	rstate1 := getRaftState(ctx, t, &node1)
	rstate2 := getRaftState(ctx, t, &node2)

	require.True(t, reflect.DeepEqual(rstate0, rstate1))
	require.True(t, reflect.DeepEqual(rstate0, rstate2))

	leader, err := node1.RaftLeader(ctx)
	require.NoError(t, err)
	leaderNode := peerToNode[leader]

	err = leaderNode.Stop(ctx)
	require.NoError(t, err)
	oldLeaderNode := leaderNode

	time.Sleep(5 * time.Second)

	newLeader := leader
	for _, n := range nodes {
		if n != leaderNode {
			newLeader, err = n.RaftLeader(ctx)
			require.NoError(t, err)
			require.NotEqual(t, newLeader, leader)
		}
	}

	require.NotEqual(t, newLeader, leader)
	leaderNode = peerToNode[newLeader]

	msg2 := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    leaderNode.DefaultKey.Address,
		Value: big.NewInt(100000),
	}
	mu2 := uuid.New()
	signedMsg2, err := leaderNode.MpoolPushMessage(ctx, msg2, &api.MessageSendSpec{
		MsgUuid: mu2,
	})
	require.NoError(t, err)
	mLookup, err = leaderNode.StateWaitMsg(ctx, signedMsg2.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	rstate := getRaftState(ctx, t, leaderNode)

	for _, n := range nodes {
		if n != oldLeaderNode {
			rs := getRaftState(ctx, t, n)
			require.True(t, reflect.DeepEqual(rs, rstate))
		}
	}
}

func TestRaftStateMiner(t *testing.T) {

	kit.QuietMiningLogs()
	ctx := context.Background()

	var (
		node0 kit.TestFullNode
		node1 kit.TestFullNode
		node2 kit.TestFullNode
		miner kit.TestMiner
	)

	setup(ctx, t, &node0, &node1, &node2, &miner)

	fmt.Println(node0.WalletList(context.Background()))
	fmt.Println(node1.WalletList(context.Background()))
	fmt.Println(node2.WalletList(context.Background()))

	bal, err := node0.WalletBalance(ctx, node0.DefaultKey.Address)
	require.NoError(t, err)

	msgHalfBal := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    node0.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	mu := uuid.New()
	smHalfBal, err := miner.FullNode.MpoolPushMessage(ctx, msgHalfBal, &api.MessageSendSpec{
		MsgUuid: mu,
	})
	require.NoError(t, err)
	mLookup, err := node0.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	rstate0 := getRaftState(ctx, t, &node0)
	rstate1 := getRaftState(ctx, t, &node1)
	rstate2 := getRaftState(ctx, t, &node2)

	require.EqualValues(t, rstate0, rstate1)
	require.EqualValues(t, rstate0, rstate2)
}

func TestRaftStateLeaderDisconnectsMiner(t *testing.T) {

	kit.QuietMiningLogs()
	ctx := context.Background()

	var (
		node0 kit.TestFullNode
		node1 kit.TestFullNode
		node2 kit.TestFullNode
		miner kit.TestMiner
	)

	nodes := []*kit.TestFullNode{&node0, &node1, &node2}

	setup(ctx, t, &node0, &node1, &node2, &miner)

	peerToNode := make(map[peer.ID]*kit.TestFullNode)
	for _, n := range nodes {
		peerToNode[n.Pkey.PeerID] = n
	}

	leader, err := node0.RaftLeader(ctx)
	require.NoError(t, err)
	leaderNode := peerToNode[leader]

	// Take leader node down
	err = leaderNode.Stop(ctx)
	require.NoError(t, err)
	oldLeaderNode := leaderNode

	time.Sleep(5 * time.Second)

	newLeader := leader
	for _, n := range nodes {
		if n != leaderNode {
			newLeader, err = n.RaftLeader(ctx)
			require.NoError(t, err)
			require.NotEqual(t, newLeader, leader)
		}
	}

	require.NotEqual(t, newLeader, leader)
	leaderNode = peerToNode[newLeader]

	msg2 := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    node0.DefaultKey.Address,
		Value: big.NewInt(100000),
	}
	mu2 := uuid.New()

	signedMsg2, err := miner.FullNode.MpoolPushMessage(ctx, msg2, &api.MessageSendSpec{
		MaxFee:  abi.TokenAmount(config.DefaultDefaultMaxFee),
		MsgUuid: mu2,
	})
	require.NoError(t, err)

	mLookup, err := leaderNode.StateWaitMsg(ctx, signedMsg2.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	rstate := getRaftState(ctx, t, leaderNode)

	for _, n := range nodes {
		if n != oldLeaderNode {
			rs := getRaftState(ctx, t, n)
			require.True(t, reflect.DeepEqual(rs, rstate))
		}
	}
}

// Miner sends message on leader
// Leader disconnects
// Call StateWaitMsg on new leader
func TestLeaderDisconnectsCheckMsgStateOnNewLeader(t *testing.T) {

	kit.QuietMiningLogs()
	ctx := context.Background()

	var (
		node0 kit.TestFullNode
		node1 kit.TestFullNode
		node2 kit.TestFullNode
		miner kit.TestMiner
	)

	nodes := []*kit.TestFullNode{&node0, &node1, &node2}

	setup(ctx, t, &node0, &node1, &node2, &miner)

	peerToNode := make(map[peer.ID]*kit.TestFullNode)
	for _, n := range nodes {
		peerToNode[n.Pkey.PeerID] = n
	}

	bal, err := node0.WalletBalance(ctx, node0.DefaultKey.Address)
	require.NoError(t, err)

	msgHalfBal := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    node0.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	mu := uuid.New()
	smHalfBal, err := miner.FullNode.MpoolPushMessage(ctx, msgHalfBal, &api.MessageSendSpec{
		MsgUuid: mu,
	})
	require.NoError(t, err)

	leader, err := node0.RaftLeader(ctx)
	require.NoError(t, err)
	leaderNode := peerToNode[leader]

	// Take leader node down
	err = leaderNode.Stop(ctx)
	require.NoError(t, err)
	oldLeaderNode := leaderNode

	time.Sleep(5 * time.Second)

	// Check if all active nodes update their leader
	newLeader := leader
	for _, n := range nodes {
		if n != leaderNode {
			newLeader, err = n.RaftLeader(ctx)
			require.NoError(t, err)
			require.NotEqual(t, newLeader, leader)
		}
	}

	require.NotEqual(t, newLeader, leader)
	leaderNode = peerToNode[newLeader]

	mLookup, err := leaderNode.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	rstate := getRaftState(ctx, t, leaderNode)

	// Check if Raft state is consistent on all active nodes
	for _, n := range nodes {
		if n != oldLeaderNode {
			rs := getRaftState(ctx, t, n)
			require.True(t, reflect.DeepEqual(rs, rstate))
		}
	}
}

func TestChainStoreSync(t *testing.T) {

	kit.QuietMiningLogs()
	ctx := context.Background()

	var (
		node0 kit.TestFullNode
		node1 kit.TestFullNode
		node2 kit.TestFullNode
		miner kit.TestMiner
	)

	nodes := []*kit.TestFullNode{&node0, &node1, &node2}

	setup(ctx, t, &node0, &node1, &node2, &miner)

	peerToNode := make(map[peer.ID]*kit.TestFullNode)
	for _, n := range nodes {
		peerToNode[n.Pkey.PeerID] = n
	}

	bal, err := node0.WalletBalance(ctx, node0.DefaultKey.Address)
	require.NoError(t, err)

	leader, err := node0.RaftLeader(ctx)
	require.NoError(t, err)
	leaderNode := peerToNode[leader]

	msgHalfBal := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    node0.DefaultKey.Address,
		Value: big.Div(bal, big.NewInt(2)),
	}
	mu := uuid.New()
	smHalfBal, err := miner.FullNode.MpoolPushMessage(ctx, msgHalfBal, &api.MessageSendSpec{
		MsgUuid: mu,
	})
	require.NoError(t, err)

	for _, n := range nodes {
		fmt.Println(n != leaderNode)
		if n != leaderNode {
			mLookup, err := n.StateWaitMsg(ctx, smHalfBal.Cid(), 3, api.LookbackNoLimit, true)
			require.NoError(t, err)
			require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)
			//break
		}
	}
}

func TestGoRPCAuth(t *testing.T) {
	// TODO Fix Raft, then enable this test. https://github.com/filecoin-project/lotus/issues/9888
	t.SkipNow()

	blockTime := 1 * time.Second

	kit.QuietMiningLogs()
	ctx := context.Background()

	var (
		node0 kit.TestFullNode
		node1 kit.TestFullNode
		node2 kit.TestFullNode
		node3 kit.TestFullNode
		miner kit.TestMiner
	)

	pkey0, _ := generatePrivKey()
	pkey1, _ := generatePrivKey()
	pkey2, _ := generatePrivKey()

	pkeys := []*kit.Libp2p{pkey0, pkey1, pkey2}
	initPeerSet := []string{}
	for _, pkey := range pkeys {
		initPeerSet = append(initPeerSet, "/p2p/"+pkey.PeerID.String())
	}

	raftOps := kit.ConstructorOpts(
		node.Override(new(*gorpc.Client), modules.NewRPCClient),
		node.Override(new(*consensus.ClusterRaftConfig), func() *consensus.ClusterRaftConfig {
			cfg := consensus.DefaultClusterRaftConfig()
			cfg.InitPeerset = initPeerSet
			return cfg
		}),
		node.Override(new(*consensus.Consensus), consensus.NewConsensusWithRPCClient(false)),
		node.Override(new(*messagesigner.MessageSignerConsensus), messagesigner.NewMessageSignerConsensus),
		node.Override(new(messagesigner.MsgSigner), func(ms *messagesigner.MessageSignerConsensus) *messagesigner.MessageSignerConsensus { return ms }),
		node.Override(new(*modules.RPCHandler), modules.NewRPCHandler),
		node.Override(node.GoRPCServer, modules.NewRPCServer),
	)
	//raftOps := kit.ConstructorOpts()

	ens := kit.NewEnsemble(t).FullNode(&node0, raftOps, kit.ThroughRPC()).FullNode(&node1, raftOps, kit.ThroughRPC()).FullNode(&node2, raftOps, kit.ThroughRPC()).FullNode(&node3, raftOps)
	node0.AssignPrivKey(pkey0)
	node1.AssignPrivKey(pkey1)
	node2.AssignPrivKey(pkey2)

	nodes := []*kit.TestFullNode{&node0, &node1, &node2}
	wrappedFullNode := kit.MergeFullNodes(nodes)

	ens.MinerEnroll(&miner, wrappedFullNode, kit.WithAllSubsystems(), kit.ThroughRPC())
	ens.Start()

	// Import miner wallet to all nodes
	addr0, err := node0.WalletImport(ctx, &miner.OwnerKey.KeyInfo)
	require.NoError(t, err)
	addr1, err := node1.WalletImport(ctx, &miner.OwnerKey.KeyInfo)
	require.NoError(t, err)
	addr2, err := node2.WalletImport(ctx, &miner.OwnerKey.KeyInfo)
	require.NoError(t, err)

	fmt.Println(addr0, addr1, addr2)

	ens.InterconnectAll()

	ens.AddInactiveMiner(&miner)
	ens.Start()

	ens.InterconnectAll().BeginMining(blockTime)

	leader, err := node0.RaftLeader(ctx)
	require.NoError(t, err)

	client := node3.FullNode.(*impl.FullNodeAPI).RaftAPI.MessageSigner.Consensus.RpcClient
	method := "MpoolPushMessage"

	msg := &types.Message{
		From:  miner.OwnerKey.Address,
		To:    node0.DefaultKey.Address,
		Value: big.NewInt(100000),
	}
	msgWhole := &api.MpoolMessageWhole{Msg: msg}
	var ret types.SignedMessage

	err = client.CallContext(ctx, leader, "Consensus", method, msgWhole, &ret)
	require.True(t, gorpc.IsAuthorizationError(err))

}
