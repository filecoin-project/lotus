package smm

import (
    "context"
    "fmt"
    "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/build"
    "github.com/filecoin-project/lotus/chain/actors"
    "github.com/filecoin-project/lotus/chain/address"
    "github.com/filecoin-project/lotus/chain/store"
    "github.com/filecoin-project/lotus/chain/types"
    "github.com/ipfs/go-cid"
    "github.com/multiformats/go-multihash"
    "github.com/stretchr/testify/require"
    "testing"
    "time"
)

type testListener struct {
    Cancel context.CancelFunc
}

func (t testListener) OnChainStateChanged(*StateChange) {
    t.Cancel()
}

func build_head_change(size, height int, t string) []*store.HeadChange {
    a, _ := address.NewFromString("t0101")
    b, _ := address.NewFromString("t0102")
    cid, _ := cid.Parse("bafkqaaa")
    result := make([]*store.HeadChange, size)
    for i := 0; i < size; i++ {
        headChange := new(store.HeadChange)
        headChange.Type = t
        headChange.Val, _ = types.NewTipSet([]*types.BlockHeader{
            {
                Height: uint64(height),
                Miner:  a,
                Ticket: &types.Ticket{[]byte{byte(height % 2)}},
                ParentStateRoot:       cid,
                Messages:              cid,
                ParentMessageReceipts: cid,
                BlockSig:     &types.Signature{Type: types.KTBLS},
                BLSAggregate: types.Signature{Type: types.KTBLS},
            },
            {
                Height: uint64(height),
                Miner:  b,
                Ticket: &types.Ticket{[]byte{byte((height + 1) % 2)}},
                ParentStateRoot:       cid,
                Messages:              cid,
                ParentMessageReceipts: cid,
                BlockSig:     &types.Signature{Type: types.KTBLS},
                BLSAggregate: types.Signature{Type: types.KTBLS},
            },
        })
        result[i] = headChange
    }
    return result
}

func Test_Creation(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    t.Run("Listener is called", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        events <- build_head_change(1, 2, store.HCCurrent)
        var out <-chan []*store.HeadChange
        out = events

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.Start(ctx)
        require.NoError(t, err)
        events <- build_head_change(3, 3, store.HCApply)
        called := false
        select {
        case <- ctx.Done():
            called = true
        case <- time.After(100 * time.Millisecond):
        }
        require.Equal(t, true, called, "event handling failed to call the listener")
    })

    t.Run("ChainNotify fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        listener := testListener{cancel}
        mockapi := FullNode{}
        mockapi.On("ChainNotify", ctx).Return(nil, fmt.Errorf("Fake error")).Once()
        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.Start(ctx)
    })

    t.Run("Invalid actor address", func(t *testing.T) {
        _, cancel := context.WithCancel(context.Background())
        listener := testListener{cancel}
        mockapi := FullNode{}
        _, err := NewStorageMinerAdapter(&mockapi, Address("a"), Address(worker.String()), listener)
        require.Error(t, err)
    })

    t.Run("Invalid worker address", func(t *testing.T) {
        _, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        _, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address("w"), listener)
        require.Error(t, err)
    })

    t.Run("Initial head change is too long", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        events <- build_head_change(3, 2, store.HCCurrent)
        var out <-chan []*store.HeadChange
        out = events
        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.Start(ctx)
        require.Error(t, err)
    })

    t.Run("Initial head change has wrong type", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        events <- build_head_change(1, 2, store.HCRevert)
        var out <-chan []*store.HeadChange
        out = events
        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.Start(ctx)
        require.Error(t, err)
    })
}

func Test_MostRecentState(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    t.Run("Succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        initialChange := build_head_change(1, 2, store.HCCurrent)
        ts := initialChange[0].Val
        mockapi.On("ChainHead", ctx).Return(ts, nil).Once()
        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, _, err = node.MostRecentState(ctx)
        require.NoError(t, err)
    })

    t.Run("ChainHead fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        mockapi.On("ChainHead", ctx).Return(nil, fmt.Errorf("ChainHead failed")).Once()
        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, _,  err = node.MostRecentState(ctx)
        require.Error(t, err)
    })
}

func Test_GetMinerState(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    t.Run("Succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        sectorInfo := api.ChainSectorInfo{1, make([]byte, 32), make([]byte, 32)}
        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]
        block1 := ts.Blocks()[1]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("StateMinerSectors", ctx, worker, ts).Return([]*api.ChainSectorInfo{&sectorInfo}, nil).Once()
        mockapi.On("StateMinerProvingSet", ctx, worker, ts).Return([]*api.ChainSectorInfo{&sectorInfo}, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(block1, nil).Once()

        node, _ := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetMinerState(ctx, initialState.StateKey)
        require.NoError(t, err)
    })

    t.Run("StateMinerProvingSet fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        sectorInfo := api.ChainSectorInfo{1, make([]byte, 32), make([]byte, 32)}
        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]
        block1 := ts.Blocks()[1]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("StateMinerSectors", ctx, worker, ts).Return([]*api.ChainSectorInfo{&sectorInfo}, nil).Once()
        mockapi.On("StateMinerProvingSet", ctx, worker, ts).Return(nil, fmt.Errorf("StateMinerProvingSet failed")).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(block1, nil).Once()

        node, _ := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetMinerState(ctx, initialState.StateKey)
        require.Error(t, err)
    })

    t.Run("StateMinerSectors fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]
        block1 := ts.Blocks()[1]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("StateMinerSectors", ctx, worker, ts).Return(nil, fmt.Errorf("StateMinerSectors failed")).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(block1, nil).Once()

        node, _ := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetMinerState(ctx, initialState.StateKey)
        require.Error(t, err)
    })

    t.Run("invalid block cid", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(nil, fmt.Errorf("invalid block")).Once()

        node, _ := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetMinerState(ctx, initialState.StateKey)
        require.Error(t, err)
    })

    t.Run("invalid state key", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        invalidStateKey := []byte{10, 32, 20, 20}
        _, err = node.GetMinerState(ctx, StateKey(invalidStateKey))
        require.Error(t, err)
    })
}

func Test_GetRandomness(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    t.Run("succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        tsk := types.NewTipSetKey(ts.Cids()...)
        block0 := ts.Blocks()[0]
        block1 := ts.Blocks()[1]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(block1, nil).Once()
        mockapi.On("ChainGetRandomness", ctx, tsk, int64(1)).Return(make([]byte, 32), nil).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetRandomness(ctx, initialState.StateKey, 1)
        require.NoError(t, err)
    })

    t.Run("ChainGetRandomness fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        tsk := types.NewTipSetKey(ts.Cids()...)
        block0 := ts.Blocks()[0]
        block1 := ts.Blocks()[1]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(block1, nil).Once()
        mockapi.On("ChainGetRandomness", ctx, tsk, int64(1)).Return(nil, fmt.Errorf("ChainGetRandomness failed")).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetRandomness(ctx, initialState.StateKey, 1)
        require.Error(t, err)
    })

    t.Run("invalid block", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(nil, fmt.Errorf("invalid block")).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetRandomness(ctx, initialState.StateKey, 1)
        require.Error(t, err)
    })
}

func Test_GetProvingPeriod(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    t.Run("succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]
        block1 := ts.Blocks()[1]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(block1, nil).Once()
        mockapi.On("StateMinerElectionPeriodStart", ctx, worker, ts).Return(uint64(20 + build.FallbackPoStDelay), nil).Once()

        node, _ := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        pp, err := node.GetProvingPeriod(ctx, initialState.StateKey)
        require.NoError(t, err)
        require.True(t, pp.Start > 0)
    })

    t.Run("StateMinerProvingPeriodEnd fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]
        block1 := ts.Blocks()[1]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(block1, nil).Once()
        mockapi.On("StateMinerElectionPeriodStart", ctx, worker, ts).Return(uint64(0), fmt.Errorf("StateMinerProvingPeriodEnd failed")).Once()

        node, _ := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetProvingPeriod(ctx, initialState.StateKey)
        require.Error(t, err)
    })

    t.Run("invalid block", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}
        events := make(chan []*store.HeadChange, 1)
        initialChange := build_head_change(1, 2, store.HCCurrent)
        events <- initialChange
        var out <-chan []*store.HeadChange
        out = events

        ts := initialChange[0].Val
        block0 := ts.Blocks()[0]

        mockapi.On("ChainNotify", ctx).Return(out, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[0]).Return(block0, nil).Once()
        mockapi.On("ChainGetBlock", ctx, ts.Cids()[1]).Return(nil, fmt.Errorf("invalid block")).Once()

        node, _ := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        initialState, err := node.Start(ctx)
        require.NoError(t, err)
        _, err = node.GetProvingPeriod(ctx, initialState.StateKey)
        require.Error(t, err)
    })
}

func signedMessage(to, from address.Address) *types.SignedMessage {
    result := types.SignedMessage{
        Message: types.Message{
            To:       to,
            From:     from,
            Params:   []byte("some bytes, idk"),
            Method:   1235126,
            Value:    types.NewInt(123123),
            GasPrice: types.NewInt(1234),
            GasLimit: types.NewInt(9992969384),
            Nonce:    123123,
        },
    }
    result.Signature.Type = types.KTBLS
    return &result
}

func Test_SubmitSectorPreCommitment(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    cb := cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN + 31}
    commR, err := cb.Sum([]byte("hello world"))
    require.NoError(t, err)
    epoch := Epoch(3)
    dealIDs := []uint64{5, 6, 7, 8}

    params := actors.SectorPreCommitInfo{
        SectorNumber: 1,
        CommR: commR.Bytes(),
        SealEpoch: uint64(epoch),
        DealIDs: dealIDs,
    }
    enc, err := actors.SerializeParams(&params)
    require.NoError(t, err)
    msg := types.Message{
        To:       actor,
        From:     worker,
        Method:   actors.MAMethods.PreCommitSector,
        Params:   enc,
        Value:    types.NewInt(0),
        GasLimit: types.NewInt(1000000),
        GasPrice: types.NewInt(1),
    }

    t.Run("succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(signedMessage(actor, worker), nil).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitSectorPreCommitment(ctx, 1, epoch, commR, dealIDs)
        require.NoError(t, err)
    })

    t.Run("MpoolPushMessage fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(nil, fmt.Errorf("MpoolPushMessage failed")).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitSectorPreCommitment(ctx, 1, epoch, commR, dealIDs)
        require.Error(t, err)
    })
}

func Test_SubmitSectorCommitment(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    dealIDs := []uint64{5, 6, 7, 8}
    proof := make([]byte, 32)

    params := actors.SectorProveCommitInfo{
        Proof: proof,
        SectorID: uint64(3),
        DealIDs: dealIDs,
    }
    enc, err := actors.SerializeParams(&params)
    require.NoError(t, err)
    msg := types.Message{
        To:       actor,
        From:     worker,
        Method:   actors.MAMethods.ProveCommitSector,
        Params:   enc,
        Value:    types.NewInt(0),
        GasLimit: types.NewInt(1000000),
        GasPrice: types.NewInt(1),
    }

    t.Run("succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(signedMessage(actor, worker), nil).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitSectorCommitment(ctx, 3, proof, dealIDs)
        require.NoError(t, err)
    })

    t.Run("MpoolPushMessage fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(nil, fmt.Errorf("MpoolPushMessage failed")).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitSectorCommitment(ctx, 3, proof, dealIDs)
        require.Error(t, err)
    })
}

func Test_SubmitPoSt(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")

    proof := make([]byte, 32)

    params := actors.SubmitFallbackPoStParams{
        Proof: proof,
        Candidates: []types.EPostTicket{{	[]byte{'a', 'b'},
            uint64(1024), uint64(2048)}},
    }
    enc, err := actors.SerializeParams(&params)
    require.NoError(t, err)
    msg := types.Message{
        To:       actor,
        From:     worker,
        Method:   actors.MAMethods.SubmitFallbackPoSt,
        Params:   enc,
        Value:    types.NewInt(0),
        GasLimit: types.NewInt(1000000),
        GasPrice: types.NewInt(1),
    }

    t.Run("succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(signedMessage(actor, worker), nil).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitPoSt(ctx, proof, []EPostTicket{{[]byte{'a', 'b'}, uint64(1024), uint64(2048)}})
        require.NoError(t, err)
    })

    t.Run("MpoolPushMessage fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(nil, fmt.Errorf("MpoolPushMessage failed")).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitPoSt(ctx, proof, []EPostTicket{{[]byte{'a', 'b'}, uint64(1024), uint64(2048)}})
        require.Error(t, err)
    })
}

func Test_SubmitDeclaredFaults(t *testing.T) {
    worker, _ := address.NewFromString("t0101")
    actor, _  := address.NewFromString("t0102")
    faultySector := uint64(123)
    faults := make(BitField)
    faults[faultySector] = struct{}{}
    params := actors.DeclareFaultsParams{
        Faults: types.NewBitField(),
    }
    params.Faults.Set(faultySector)
    enc, err := actors.SerializeParams(&params)
    require.NoError(t, err)
    msg := types.Message{
        To:       actor,
        From:     worker,
        Method:   actors.MAMethods.DeclareFaults,
        Params:   enc,
        Value:    types.NewInt(0),
        GasLimit: types.NewInt(1000000),
        GasPrice: types.NewInt(1),
    }

    t.Run("succeeds", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(signedMessage(actor, worker), nil).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitDeclaredFaults(ctx, faults)
        require.NoError(t, err)
    })

    t.Run("MpoolPushMessage fails", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockapi := FullNode{}
        listener := testListener{cancel}

        mockapi.On("MpoolPushMessage", ctx, &msg).Return(nil, fmt.Errorf("MpoolPushMessage failed")).Once()

        node, err := NewStorageMinerAdapter(&mockapi, Address(actor.String()), Address(worker.String()), listener)
        require.NoError(t, err)
        _, err = node.SubmitDeclaredFaults(ctx, faults)
        require.Error(t, err)
    })
}
