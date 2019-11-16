package smm

import (
    "bytes"
    "context"
    "crypto/rand"
    "github.com/filecoin-project/lotus/api/test"
    "github.com/filecoin-project/lotus/chain/address"
    "github.com/filecoin-project/lotus/miner"
    "github.com/filecoin-project/lotus/node"
    "github.com/filecoin-project/lotus/node/modules"
    modtest "github.com/filecoin-project/lotus/node/modules/testing"
    "github.com/filecoin-project/lotus/node/repo"
    "github.com/ipfs/go-cid"
    "github.com/libp2p/go-libp2p-core/crypto"
    "github.com/libp2p/go-libp2p-core/peer"
    mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
    "github.com/multiformats/go-multihash"
    "testing"
)

type testListener struct {
    Called bool
}

func (t testListener) OnChainStateChanged(*StateChange) {
    t.Called = true
}

func create(ctx context.Context) (*test.TestNode, error) {
    var testNode test.TestNode
    mn := mocknet.New(ctx)

    pk, _, err := crypto.GenerateEd25519Key(rand.Reader)
    if err != nil {
        return nil, err
    }
    minerPid, err := peer.IDFromPrivateKey(pk)
    var genbuf bytes.Buffer
    mineBlock := make(chan struct{})
    _, err = node.New(ctx,
        node.FullAPI(&testNode.FullNode),
        node.Online(),
        node.Repo(repo.NewMemory(nil)),
        node.MockHost(mn),
        node.Test(),
        node.Override(new(*miner.Miner), miner.NewTestMiner(mineBlock)),
        node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genbuf, minerPid)),
    )
    if err != nil {
        return nil, err
    }
    testNode.MineOne = func(ctx context.Context) error {
        select {
        case mineBlock <- struct{}{}:
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    err = mn.LinkAll()
    if err != nil {
        return nil, err
    }
    return &testNode, nil
}

func Test_NodeCreation(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    miner, err := address.NewFromString("t0101")
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    node, initialState, err := NewNode(ctx, fullnode, Address(actor.String()), Address(miner.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    if node == nil {
        t.Fatalf("invalid node")
    }
    if initialState == nil || initialState.StateKey == "" {
        t.Fatalf("invalid initial state")
    }
}

func Test_MinerState(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    miner, err := address.NewFromString("t0101")
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    node, initialState, err := NewNode(ctx, fullnode, Address(actor.String()), Address(miner.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    if initialState == nil || initialState.StateKey == "" {
        t.Fatalf("invalid initial state")
    }
    state, err := node.GetMinerState(ctx, initialState.StateKey)
    if err != nil {
        t.Fatal(err)
    }
    _ = state

    pp, err := node.GetProvingPeriod(ctx, initialState.StateKey)
    if err != nil {
        t.Fatal(err)
    }
    if pp.Start != pp.End {
        t.Fatalf("invalid proving period after genesis")
    }
}

func Test_Randomness(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    miner, err := address.NewFromString("t0101")
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    node, initialState, err := NewNode(ctx, fullnode, Address(actor.String()), Address(miner.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    randomness, err := node.GetRandomness(ctx, initialState.StateKey, 0)
    if err != nil {
        t.Fatal(err)
    }
    if 32 != len(randomness) {
        t.Fatalf("invalid length for randomness")
    }
}

func Test_RandomPoSt(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    node, _, err := NewNode(ctx, fullnode, Address(actor.String()), Address(actor.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    proof := make([]byte, 32)
    bytesRead, err := rand.Read(proof)
    if err != nil {
        t.Fatal(err)
    }
    if bytesRead != len(proof) {
        t.Fatalf("invalid proof length")
    }
    cid, err := node.SubmitPoSt(ctx, proof)
    if err != nil {
        t.Fatal(err)
    }
    _ = cid
}

func Test_SubmitPoStFail(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    miner, err := address.NewFromString("t0101")
    if err != nil {
      t.Fatal(err)
    }
    node, _, err := NewNode(ctx, fullnode, Address(actor.String()), Address(miner.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    proof := make([]byte, 32)
    bytesRead, err := rand.Read(proof)
    if bytesRead != len(proof) {
        t.Fatalf("invalid proof length")
    }
    if bytesRead != len(proof) {
        t.Fatalf("invalid proof length")
    }
    _, err = node.SubmitPoSt(ctx, proof)
    if err == nil {
        t.Fatalf("expecting SubmitPoSt to fail")
    }
}

func Test_SubmitSelfDeals(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    // Using the same address here as the miner one doesn't yet have a key in the test node's wallet
    node, _, err := NewNode(ctx, fullnode, Address(actor.String()), Address(actor.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    cid, err := node.SubmitSelfDeals(ctx, []uint64{})
    if err != nil {
        t.Fatal(err)
    }
    _ = cid
}

func Test_SubmitSectorCommitment(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    // Using the same address here as the miner one doesn't yet have a key in the test node's wallet
    node, _, err := NewNode(ctx, fullnode, Address(actor.String()), Address(actor.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    proof := make([]byte, 32)
    bytesRead, err := rand.Read(proof)
    if bytesRead != len(proof) {
        t.Fatalf("invalid proof length")
    }
    if bytesRead != len(proof) {
        t.Fatalf("invalid proof length")
    }
    cid, err := node.SubmitSectorCommitment(ctx, 1, proof, []uint64{2, 3, 4, 5})
    if err != nil {
        t.Fatal(err)
    }
    _ = cid
}

func Test_SubmitSectorPreCommitment(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    // Using the same address here as the miner one doesn't yet have a key in the test node's wallet
    node, _, err := NewNode(ctx, fullnode, Address(actor.String()), Address(actor.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    cb := cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN + 31}
    commR, err := cb.Sum([]byte("hello world"))
    if err != nil {
        t.Fatal(err)
    }
    _, err = node.SubmitSectorPreCommitment(ctx, 1, commR, []uint64{5, 6, 7, 8})
    if err != nil {
        t.Fatal(err)
    }
}

func Test_SubmitDeclaredFaults(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    listener := testListener{false}
    // Using the same address here as the miner one doesn't yet have a key in the test node's wallet
    node, _, err := NewNode(ctx, fullnode, Address(actor.String()), Address(actor.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    faults := make(BitField)
    for i := uint64(10); i < 20; i++ {
        faults[i] = struct{}{}
    }
    _, err = node.SubmitDeclaredFaults(ctx, faults)
    if err != nil {
        t.Fatal(err)
    }
}

func Test_MostRecentState(t *testing.T) {
    ctx := context.Background()
    fullnode, err := create(ctx)
    if err != nil {
        t.Fatal(err)
    }
    actor, err := fullnode.WalletDefaultAddress(ctx)
    if err != nil {
        t.Fatal(err)
    }
    miner, err := address.NewFromString("t0101")
    listener := testListener{false}
    // Using the same address here as the miner one doesn't yet have a key in the test node's wallet
    node, _, err := NewNode(ctx, fullnode, Address(actor.String()), Address(miner.String()), listener)
    if err != nil {
        t.Fatal(err)
    }
    _, err = node.MostRecentState(ctx)
    if err != nil {
        t.Fatal(err)
    }
}

//func Test_StateChangeNotifications(t *testing.T) {
//   ctx := context.Background()
//   fullnode, err := create(ctx)
//   if err != nil {
//       t.Fatal(err)
//   }
//   actor, err := fullnode.WalletDefaultAddress(ctx)
//   if err != nil {
//       t.Fatal(err)
//   }
//   miner, err := address.NewFromString("t0101")
//   if err != nil {
//       t.Fatal(err)
//   }
//   listener := testListener{false}
//   node, initialState, err := NewNode(ctx, fullnode, Address(actor.String()), Address(miner.String()), listener)
//   if err != nil {
//       t.Fatal(err)
//   }
//   if node == nil {
//       t.Fatalf("invalid node")
//   }
//   if initialState == nil || initialState.StateKey == "" {
//       t.Fatalf("invalid initial state")
//   }
//   err = fullnode.MineOne(ctx)
//   if err != nil {
//       t.Fatal(err)
//   }
//   node.Start()
//   if listener.Called == false {
//       t.Fatalf("state change notification failed")
//   }
//}
