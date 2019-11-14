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
)

type lotusAdapter struct {
    actor        laddress.Address
    miner        laddress.Address
    fullNode     api.FullNode
    listener     StateChangeHandler
    headChanges  <-chan[]*store.HeadChange
    context      context.Context
}

func NewNode(ctx context.Context, fullNode api.FullNode, actor, miner Address, listener StateChangeHandler) (Node, *StateChange, error) {
    adapter := lotusAdapter{
        fullNode: fullNode,
        context: ctx,
        listener: listener,
    }
    var err error
    adapter.actor, err = laddress.NewFromString(string(actor))
    if err != nil {
        return nil, nil, err
    }
    adapter.miner, err = laddress.NewFromString(string(miner))
    if err != nil {
        return nil, nil, err
    }
    adapter.headChanges, err = fullNode.ChainNotify(ctx)
    if err != nil {
        return nil, nil, err
    }
    // read current state
    initialNotification := <-adapter.headChanges
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
    return adapter, stateChanges[0], nil
}

func stateChangesFromHeadChanges(changes []*store.HeadChange) ([]*StateChange, error) {
    stateChanges := make([]*StateChange, len(changes))
    for idx, change := range changes {
        tsk := types.NewTipSetKey(change.Val.Cids()...)
        stateChanges[idx] = &StateChange{
            Epoch: Epoch(change.Val.Height()),
            StateKey: StateKey(tsk.Bytes()),
        }
    }
    return stateChanges, nil
}

// this is expensive, it will be removed as FullNode transitions to use TipSetKey instead of TipSet
func statekey2tipset(ctx context.Context, stateKey StateKey, fullNode api.FullNode) (*types.TipSet, error) {
    tsk, err := types.TipSetKeyFromBytes([]byte(stateKey))
    if err != nil {
        return nil, err
    }
    cids := tsk.Cids()
    blockHeaders := make([]*types.BlockHeader, len(cids))
    for idx, blockCid := range cids {
        block, err := fullNode.ChainGetBlock(ctx, blockCid)
        if err != nil {
            return nil, err
        }
        blockHeaders[idx] = block
    }
    return types.NewTipSet(blockHeaders)
}

func (adapter lotusAdapter) eventHandler() {
    for {
        select {
        case changes := <-adapter.headChanges:
            stateChanges, err := stateChangesFromHeadChanges(changes)
            if err != nil {
                // TODO: Log, nothing else to do
            }
            for _, stateChange := range stateChanges {
                adapter.listener.OnChainStateChanged(stateChange)
            }

        case <-adapter.context.Done():
            return
        }
    }
}

func (adapter lotusAdapter) callMinerActorMethod(ctx context.Context, method uint64, payload []byte) (cid.Cid, error) {
    // TODO: Validate that using the miner/worker address for both To and From is allowed
    msg := types.Message{
        To:       adapter.actor,
        From:     adapter.miner,
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

func (adapter lotusAdapter) Start() {
    go adapter.eventHandler()
}

func (adapter lotusAdapter) MostRecentState(ctx context.Context) (StateKey, error) {
    ts, err := adapter.fullNode.ChainHead(ctx)
    if err != nil {
        return "", err
    }
    tsk := types.NewTipSetKey(ts.Cids()...)
    return StateKey(tsk.Bytes()), nil
}

func (adapter lotusAdapter) GetMinerState(ctx context.Context, stateKey StateKey) (*MinerChainState, error) {
    state := new(MinerChainState)
    ts, err := statekey2tipset(ctx, stateKey, adapter.fullNode)
    if err != nil {
        return nil, err
    }
    sectors, err := adapter.fullNode.StateMinerSectors(ctx, adapter.miner, ts)
    if err != nil {
        return nil, err
    }
    provingSectors, err := adapter.fullNode.StateMinerProvingSet(ctx, adapter.miner, ts)
    if err != nil {
        return nil, err
    }
    state.Sectors = make(map[uint64]api.ChainSectorInfo)
    for _, sectorInfo := range sectors {
        state.Sectors[sectorInfo.SectorID] = *sectorInfo
    }
    for _, sectorInfo := range provingSectors {
        state.ProvingSet[sectorInfo.SectorID] = struct{}{}
    }
    state.Address = Address(adapter.miner.String())
    state.PreCommittedSectors = make(map[uint64]api.ChainSectorInfo)
    state.StagedCommittedSectors = make(map[uint64]api.ChainSectorInfo)
    state.ProvingSet = make(map[uint64]struct{})
    return state, nil
}

func (adapter lotusAdapter) GetRandomness(ctx context.Context, stateKey StateKey, offset uint) ([]byte, error) {
    ts, err := statekey2tipset(ctx, stateKey, adapter.fullNode)
    if err != nil {
        return nil, err
    }
    tsk := types.NewTipSetKey(ts.Cids()...)
    return adapter.fullNode.ChainGetRandomness(ctx, tsk, ts.Blocks()[0].Tickets, int(offset))
}

func (adapter lotusAdapter) GetProvingPeriod(ctx context.Context, stateKey StateKey) (*ProvingPeriod, error) {
    pp := new(ProvingPeriod)
    ts, err := statekey2tipset(ctx, stateKey, adapter.fullNode)
    if err != nil {
        return nil, err
    }
    end, err := adapter.fullNode.StateMinerProvingPeriodEnd(ctx, adapter.miner, ts)
    if err != nil {
        return nil, err
    }
    pp.End = Epoch(end)
    if end > build.ProvingPeriodDuration {
        pp.Start = Epoch(end - build.ProvingPeriodDuration)
    } else {
        pp.Start = 0
    }

    return pp, nil
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

func (adapter lotusAdapter) GetSealSeed(ctx context.Context, state StateKey, id SectorID) SealSeed {
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
