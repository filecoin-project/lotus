package smm

import (
    "context"
    "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/chain/address"
    "github.com/filecoin-project/lotus/chain/store"
    "github.com/filecoin-project/lotus/chain/types"
    "github.com/ipfs/go-cid"
    "time"
)

type lotusAdapter struct {
    // missing: ticket chain
    address address.Address
    fullNode api.FullNode
    listeners map[StorageMiningEvents]bool
    headChanges <-chan[]*store.HeadChange
    currentState StateID
    context context.Context
}

func NewNode(ctx context.Context, fullNode api.FullNode, address address.Address) Node {
    adapter := lotusAdapter{
        address: address,
        fullNode: fullNode,
        listeners: make(map[StorageMiningEvents]bool), // this needs synchronization
        context: ctx,
    }

    adapter.headChanges, _ = fullNode.ChainNotify(ctx)
    // ChainNotify is guaranteed to provide at least one notification
    initialState := <-adapter.headChanges
    adapter.currentState = readState(initialState)
    go adapter.eventHandler()
    return adapter
}

func readState(headChanges []*store.HeadChange) (stateID StateID) {
    // TODO: Implement
    return
}

func lotusStateID(ts *types.TipSet) (stateID StateID) {
    // TODO: Implement
    return
}

func stateID2TipSet(stateid StateID) (ts types.TipSet) {
    // TODO: Implement and rename
    return
}

func (adapter lotusAdapter) eventHandler() {
    var bufferState StateID
    stableDuration := 500 * time.Millisecond
    var timer *time.Timer

    for {
        select {
        case changes := <-adapter.headChanges:
            if timer != nil {
                timer.Reset(stableDuration)
            } else {
                timer = time.NewTimer(stableDuration)
            }
            bufferState = readState(changes)

        case <-timer.C:
            timer.Stop()
            adapter.currentState = bufferState
            go adapter.notifyListeners()

        case <-adapter.context.Done():
            return
        }
    }
}

func (adapter lotusAdapter) notifyListeners() {
    for listener, _ := range adapter.listeners {
        // TODO: Send the proper epoch
        listener.OnChainStateChanged(Epoch(10), adapter.currentState)
    }
}


func (adapter lotusAdapter) SubscribeMiner(ctx context.Context, cb StorageMiningEvents) error {
    adapter.listeners[cb] = true
    return nil
}

func (adapter lotusAdapter) UnsubscribeMiner(ctx context.Context, cb StorageMiningEvents) error {
    _, found := adapter.listeners[cb]
    if found {
        delete(adapter.listeners, cb)
    }
    return nil
}

func (adapter lotusAdapter) MostRecentStateId(ctx context.Context) (StateID, Epoch) {
    return adapter.currentState, Epoch(10)
    //var ts *types.TipSet
    //// TODO: Handle errors
    //ts, _ = adapter.fullNode.ChainHead(ctx)
    //return lotusStateID(ts), Epoch(ts.Height())
}

func (adapter lotusAdapter) GetMinerState(ctx context.Context, stateID StateID) (state MinerChainState) {
    ts := stateID2TipSet(stateID)
    sectors, _ := adapter.fullNode.StateMinerSectors(ctx, adapter.address, &ts)
    for idx, _ := range sectors {
        _ = sectors[idx]
        // update the state
    }
    return
}

func (adapter lotusAdapter) GetRandomness(ctx context.Context, stateid StateID, e Epoch, offset uint) []byte {
    // TODO: Keep track of the ticket chain
    tickets := make([]*types.Ticket, 0)
    ts := stateID2TipSet(stateid)
    // TODO: Handle errors
    result, _ := adapter.fullNode.ChainGetRandomness(ctx, &ts, tickets, int(offset))
    return result
}

func (adapter lotusAdapter) GetProvingPeriod(ctx context.Context, miner address.Address, state StateID) ProvingPeriod {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSelfDeal(ctx context.Context, size uint64) error {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorPreCommitment(ctx context.Context, miner address.Address, id SectorID, commR cid.Cid, deals []cid.Cid) {
    panic("implement me")
}

func (adapter lotusAdapter) GetSealSeed(ctx context.Context, miner address.Address, state StateID, id SectorID) SealSeed {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorCommitment(ctx context.Context, miner address.Address, id SectorID, proof Proof) {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitPoSt(ctx context.Context, miner address.Address, proof Proof) {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitDeclaredFaults(ctx context.Context, miner address.Address, faults types.BitField) {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitDeclaredRecoveries(ctx context.Context, miner address.Address, recovered types.BitField) {
    panic("implement me")
}
