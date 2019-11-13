package smm

import (
    "context"
    "fmt"
    "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/build"
    "github.com/filecoin-project/lotus/chain/actors"
    laddress "github.com/filecoin-project/lotus/chain/address"
    "github.com/filecoin-project/lotus/chain/store"
    "github.com/filecoin-project/lotus/chain/types"
    "github.com/ipfs/go-cid"
    "sync"
    "time"
)

type stateListeners struct {
    sync.RWMutex
    listeners map[StateChangeHandler]struct{}
}

func NewStateListeners() stateListeners {
    return stateListeners{
        listeners: make(map[StateChangeHandler]struct{}),
    }
}

func (sl stateListeners) Add(handler StateChangeHandler) {
    sl.Lock()
    defer sl.Unlock()

    sl.listeners[handler] = struct{}{}
}

func (sl stateListeners) Remove(handler StateChangeHandler) {
    sl.Lock()
    defer sl.Unlock()

    _, found := sl.listeners[handler]
    if found {
        delete(sl.listeners, handler)
    }
}

func (sl stateListeners) Notify(changes []*StateChange) {
    for _, change := range changes {
        sl.RLock()
        for handler, _ := range sl.listeners {
            handler.OnChainStateChanged(change)
        }
        sl.RUnlock()
    }
}

type lotusAdapter struct {
    address      laddress.Address
    fullNode     api.FullNode
    listeners    stateListeners
    headChanges  <-chan[]*store.HeadChange
    context      context.Context
}

func NewNode(ctx context.Context, fullNode api.FullNode, address Address) (Node, *StateChange, error) {
    adapter := lotusAdapter{
        fullNode: fullNode,
        listeners: NewStateListeners(),
        context: ctx,
    }
    var err error
    adapter.address, err = laddress.NewFromString(string(address))
    if err != nil {
        return nil, nil, err
    }
    adapter.headChanges, err = fullNode.ChainNotify(ctx)
    if err != nil {
        return nil, nil, err
    }
    var initialState *StateChange
    select {
    case initialNotification := <-adapter.headChanges:
        if len(initialNotification) != 1 {
            return nil, nil, fmt.Errorf("unexpected initial head notification length: %d", len(initialNotification))
        }
        if initialNotification[0].Type != store.HCCurrent {
            return nil, nil, fmt.Errorf("expected first head notification type to be 'current', was '%s'", initialNotification[0].Type)
        }
        stateChanges, err := stateChangesFromHeadChanges(initialNotification)
        if err != nil {
            return nil, nil, err
        }
        initialState = stateChanges[0]

    case <-time.After(time.Second):
        return nil, nil, fmt.Errorf("timeout while waiting for the initial state")
    }
    go adapter.eventHandler()
    return adapter, initialState, nil
}

// TODO: Implement this for TipSetKey
func stateChangesFromHeadChanges(changes []*store.HeadChange) (stateChanges []*StateChange, err error) {
    return
}

// TODO: Remove this after TipSetKey appears
func tipset2statekey(ts *types.TipSet) (stateKey *StateKey, err error) {
    return
}

// TODO: Remove this after TipSetKey appears
func statekey2tipset(ctx context.Context, stateKey *StateKey, fullNode api.FullNode) (ts *types.TipSet, err error) {
    return
}

func (adapter lotusAdapter) eventHandler() {
    for {
        select {
        case changes := <-adapter.headChanges:
            stateChanges, err := stateChangesFromHeadChanges(changes)
            if err != nil {
                // TODO: Log, nothing else to do
            }
            adapter.listeners.Notify(stateChanges)

        case <-adapter.context.Done():
            return
        }
    }
}

func (adapter lotusAdapter) SubscribeMiner(ctx context.Context, cb StateChangeHandler) error {
    adapter.listeners.Add(cb)
    return nil
}

func (adapter lotusAdapter) UnsubscribeMiner(ctx context.Context, cb StateChangeHandler) error {
    adapter.listeners.Remove(cb)
    return nil
}

func (adapter lotusAdapter) callMinerActorMethod(ctx context.Context, method uint64, payload []byte) (cid.Cid, error) {
    // TODO: Validate that using the miner/worker address for both To and From is allowed
    msg := types.Message{
        To:       adapter.address,
        From:     adapter.address,
        Method:   method,
        Params:   payload,
        // TODO: Add ability to control these 'costs'
        Value:    types.NewInt(0),
        GasLimit: types.NewInt(1000000),
        GasPrice: types.NewInt(1),
    }
    signedMsg, err := adapter.fullNode.MpoolPushMessage(ctx, &msg)
    if err != nil {
        return cid.Undef, err
    }
    return signedMsg.Cid(), nil
}

func (adapter lotusAdapter) MostRecentState(ctx context.Context) (stateKey *StateKey, epoch Epoch, err error) {
    var ts *types.TipSet
    ts, err = adapter.fullNode.ChainHead(ctx)
    if err != nil {
        return
    }
    epoch = Epoch(ts.Height())
    stateKey, err = tipset2statekey(ts)
    return
}

func (adapter lotusAdapter) GetMinerState(ctx context.Context, stateKey *StateKey) (state MinerChainState, err error) {
    var ts *types.TipSet
    ts, err = statekey2tipset(ctx, stateKey, adapter.fullNode)
    if err != nil {
        return
    }
    sectors, apiErr := adapter.fullNode.StateMinerSectors(ctx, adapter.address, ts)
    if apiErr != nil {
        err = apiErr
        return
    }
    provingSectors, apiErr := adapter.fullNode.StateMinerProvingSet(ctx, adapter.address, ts)
    if apiErr != nil {
        err = apiErr
        return
    }
    state.Sectors = make(map[uint64]api.ChainSectorInfo)
    for _, sectorInfo := range sectors {
        state.Sectors[sectorInfo.SectorID] = *sectorInfo
    }
    for _, sectorInfo := range provingSectors {
        state.ProvingSet[sectorInfo.SectorID] = struct{}{}
    }
    state.Address = Address(adapter.address.String())
    state.PreCommittedSectors = make(map[uint64]api.ChainSectorInfo)
    state.StagedCommittedSectors = make(map[uint64]api.ChainSectorInfo)

    return
}

func (adapter lotusAdapter) GetRandomness(ctx context.Context, stateKey *StateKey, e Epoch, offset uint) (buffer []byte, err error) {
    var ts *types.TipSet
    ts, err = statekey2tipset(ctx, stateKey, adapter.fullNode)
    if err != nil {
        return
    }
    buffer, err = adapter.fullNode.ChainGetRandomness(ctx, ts, ts.Blocks()[0].Tickets, int(offset))
    return
}

func (adapter lotusAdapter) GetProvingPeriod(ctx context.Context, stateKey *StateKey) (pp ProvingPeriod, err error) {
    var ts *types.TipSet
    ts, err = statekey2tipset(ctx, stateKey, adapter.fullNode)
    if err != nil {
        return
    }
    end, apiErr := adapter.fullNode.StateMinerProvingPeriodEnd(ctx, adapter.address, ts)
    if apiErr != nil {
        err = apiErr
        return
    }
    pp.End = Epoch(end)
    pp.Start = Epoch(end - build.ProvingPeriodDuration)

    return
}

func (adapter lotusAdapter) SubmitSelfDeal(ctx context.Context, size uint64) error {
    // TODO: Not sure how to implement this
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorPreCommitment(ctx context.Context, id SectorID, commR cid.Cid, dealIDs []uint64) (cid.Cid, error) {
    params := actors.SectorPreCommitInfo{
        SectorNumber: uint64(id),
        CommR: commR.Bytes(),
        SealEpoch: 0, // TODO: does this need to be passed in?!
        DealIDs: dealIDs,
    }
    payload, aerr := actors.SerializeParams(&params)
    if aerr != nil {
        return cid.Undef, aerr
    }
    return adapter.callMinerActorMethod(ctx, actors.MAMethods.PreCommitSector, payload)
}

func (adapter lotusAdapter) GetSealSeed(ctx context.Context, state *StateKey, id SectorID) SealSeed {
    // TODO: Not sure how to implement this
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorCommitment(ctx context.Context, id SectorID, proof Proof, dealIDs []uint64) (cid.Cid, error) {
    params := actors.SectorProveCommitInfo{
        Proof: proof,
        SectorID: uint64(id),
        DealIDs: dealIDs,
    }
    payload, aerr := actors.SerializeParams(&params)
    if aerr != nil {
        return cid.Undef, aerr
    }
    return adapter.callMinerActorMethod(ctx, actors.MAMethods.ProveCommitSector, payload)
}

func (adapter lotusAdapter) SubmitPoSt(ctx context.Context, proof Proof) (cid.Cid, error) {
    params := actors.SubmitPoStParams{
        Proof:   proof,
        DoneSet: types.BitFieldFromSet(nil),
    }
    payload, aerr := actors.SerializeParams(&params)
    if aerr != nil {
        return cid.Undef, aerr
    }
    return adapter.callMinerActorMethod(ctx, actors.MAMethods.SubmitPoSt, payload)
}

func (adapter lotusAdapter) SubmitDeclaredFaults(ctx context.Context, faults BitField) (cid.Cid, error) {
    params := actors.DeclareFaultsParams{}
    for k, _ := range faults {
        params.Faults.Set(k)
    }
    payload := make([]byte, 0)
    // TODO: Add MarshalCBOR and UnmarshalCBOR for DeclareFaultsParams
    return adapter.callMinerActorMethod(ctx, actors.MAMethods.DeclareFaults, payload)
}

func (adapter lotusAdapter) SubmitDeclaredRecoveries(ctx context.Context, recovered BitField) (cid cid.Cid, err error) {
    // TODO: The method is missing on the miner actor
    panic("implement me")
}
