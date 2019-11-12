package smm

import (
    "bytes"
    "context"
    "encoding/binary"
    "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/build"
    "github.com/filecoin-project/lotus/chain/actors"
    "github.com/filecoin-project/lotus/chain/actors/aerrors"
    laddress "github.com/filecoin-project/lotus/chain/address"
    "github.com/filecoin-project/lotus/chain/store"
    "github.com/filecoin-project/lotus/chain/types"
    "github.com/ipfs/go-cid"
)

type lotusAdapter struct {
    address      laddress.Address
    fullNode     api.FullNode
    listeners    map[StorageMiningEvents]struct{}
    headChanges  <-chan[]*store.HeadChange
    context      context.Context
}

func NewNode(ctx context.Context, fullNode api.FullNode, address Address) (Node, error) {
    adapter := lotusAdapter{
        fullNode: fullNode,
        listeners: make(map[StorageMiningEvents]struct{}), // this needs synchronization
        context: ctx,
    }
    var err error
    adapter.address, err = laddress.NewFromString(string(address))
    if err != nil {
        return nil, err
    }
    adapter.headChanges, _ = fullNode.ChainNotify(ctx)
    go adapter.eventHandler()
    return adapter, nil
}

func stateChangesFromHeadChanges(changes []*store.HeadChange) (stateChanges []*StateChange, err error) {
    for _, change := range changes {
        epoch := Epoch(change.Val.Height())
        stateKey, marshallErr := tipset2statekey(change.Val)
        if marshallErr != nil {
            err = marshallErr
            return
        }
        stateChange := new(StateChange)
        stateChange.Type = change.Type
        stateChange.Epoch = epoch
        stateChange.StateKey = stateKey
        stateChanges = append(stateChanges, stateChange)
    }
    return
}

func tipset2statekey(ts *types.TipSet) (stateKey *StateKey, err error) {
    buffer := new(bytes.Buffer)
    cids := ts.Cids()
    length := uint32(len(cids))
    err = binary.Write(buffer, binary.LittleEndian, length)
    if err != nil {
        return
    }
    for _, blkcid := range cids {
        b := blkcid.Bytes()
        l := uint32(len(b))
        err = binary.Write(buffer, binary.LittleEndian, l)
        if err != nil {
            return
        }
        var cidbytes []byte
        cidbytes, err = blkcid.MarshalBinary()
        if err != nil {
            return
        }
        err = binary.Write(buffer, binary.LittleEndian, cidbytes)
        if err != nil {
            return
        }
    }
    state := StateKey(buffer.String())
    stateKey = &state
    return
}

// This belongs in statekey2tipset.
// It was factored out for ease of unit testing (no FullNode calls)
func statekey2cids(stateKey *StateKey) (output []cid.Cid, err error) {
    buffer := bytes.NewReader([]byte(*stateKey))
    var length uint32
    err = binary.Read(buffer, binary.LittleEndian, &length)
    if err != nil {
        return
    }

    for idx := uint32(0); idx < length; idx++ {
        var l uint32
        err = binary.Read(buffer, binary.LittleEndian, &l)
        if err != nil {
            return
        }
        cidbuffer := make([]byte, l)
        err = binary.Read(buffer, binary.LittleEndian, &cidbuffer)
        if err != nil {
            return
        }
        var blockCid cid.Cid
        err = blockCid.UnmarshalBinary(cidbuffer)
        if err != nil {
            return
        }
        output = append(output, blockCid)
    }
    return
}

func statekey2tipset(ctx context.Context, stateKey *StateKey, fullNode api.FullNode) (ts *types.TipSet, err error) {
    cids, err := statekey2cids(stateKey)
    if err != nil {
        return
    }
    blockHeaders := make([]*types.BlockHeader, 0, len(cids))
    for idx, blockCid := range cids {
        block, apiErr := fullNode.ChainGetBlock(ctx, blockCid)
        if apiErr != nil {
            err = apiErr
            return
        }
        blockHeaders[idx] = block
    }
    ts, err = types.NewTipSet(blockHeaders)
    return
}

func (adapter lotusAdapter) eventHandler() {
    for {
        select {
        case changes := <-adapter.headChanges:
            adapter.notifyListeners(changes)

        case <-adapter.context.Done():
            return
        }
    }
}

func (adapter lotusAdapter) notifyListeners(changes []*store.HeadChange) {
    stateChanges, err := stateChangesFromHeadChanges(changes)
    if err != nil {
        // TODO: Log, nothing else to do
        return
    }

    for _, stateChange := range stateChanges {
        for listener, _ := range adapter.listeners {
            listener.OnChainStateChanged(stateChange)
        }
    }
}

func (adapter lotusAdapter) SubscribeMiner(ctx context.Context, cb StorageMiningEvents) error {
    adapter.listeners[cb] = struct{}{}
    return nil
}

func (adapter lotusAdapter) UnsubscribeMiner(ctx context.Context, cb StorageMiningEvents) error {
    _, found := adapter.listeners[cb]
    if found {
        delete(adapter.listeners, cb)
    }
    return nil
}

func (adapter lotusAdapter) callMinerActorMethod(ctx context.Context, method uint64, payload []byte) (cid cid.Cid, err error) {
    // TODO: Validate that using the miner/worker address for both To and From is allowed
    msg := types.Message{
        To:       adapter.address,
        From:     adapter.address,
        Method:   method,
        Params:   payload,
        Value:    types.NewInt(1000), // currently hard-coded late fee in actor, returned if not late
        GasLimit: types.NewInt(1000000 /* i dont know help */),
        GasPrice: types.NewInt(1),
    }

    var msgErr error
    signedMsg, msgErr := adapter.fullNode.MpoolPushMessage(ctx, &msg)

    if msgErr != nil {
        err = msgErr
        return
    }
    cid = signedMsg.Cid()
    return
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
    // fill in this information
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
    // Not sure what goes in these fields
    _ = pp.ChallengeSeed
    _ = pp.ChallengeStart
    _ = pp.ChallengeEnd

    return
}

func (adapter lotusAdapter) SubmitSelfDeal(ctx context.Context, size uint64) error {
    // TODO: Not sure how to implement this
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorPreCommitment(ctx context.Context, id SectorID, commR cid.Cid, deals []cid.Cid) (cid cid.Cid, err error) {
    params := actors.SectorPreCommitInfo{
        SectorNumber: uint64(id),
        CommR: commR.Bytes(),
        SealEpoch: 0, // TODO: does this need to be passed in?!
    }
    params.DealIDs = make([]uint64, len(deals))
    for idx, deal := range deals {
        // TODO: How to extract the deal id from the deal cid?
        // params.DealIDs[idx] = deal
        _ = deal
        _ = idx
    }
    var actorError aerrors.ActorError
    var enc []byte
    enc, actorError = actors.SerializeParams(&params)
    if actorError != nil {
        err = actorError
        return
    }


    return adapter.callMinerActorMethod(ctx, actors.MAMethods.PreCommitSector, enc)
}

func (adapter lotusAdapter) GetSealSeed(ctx context.Context, state *StateKey, id SectorID) SealSeed {
    // TODO: Not sure how to implement this
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorCommitment(ctx context.Context, id SectorID, proof Proof) (cid cid.Cid, err error) {
    params := actors.SectorProveCommitInfo{
        Proof: proof,
        SectorID: uint64(id),
    }
    // TODO: Pass in the deals
    params.DealIDs = make([]uint64, 0)
    var actorError aerrors.ActorError
    var enc []byte
    enc, actorError = actors.SerializeParams(&params)
    if actorError != nil {
        err = actorError
        return
    }


    return adapter.callMinerActorMethod(ctx, actors.MAMethods.ProveCommitSector, enc)
}

func (adapter lotusAdapter) SubmitPoSt(ctx context.Context, proof Proof) (cid cid.Cid, err error) {
    params := actors.SubmitPoStParams{
        Proof:   proof,
        DoneSet: types.BitFieldFromSet(nil),
    }
    enc, actorError := actors.SerializeParams(&params)
    if actorError != nil {
        err = actorError
        return
    }

    return adapter.callMinerActorMethod(ctx, actors.MAMethods.SubmitPoSt, enc)
}

func (adapter lotusAdapter) SubmitDeclaredFaults(ctx context.Context, faults BitField) (cid cid.Cid, err error) {
    params := actors.DeclareFaultsParams{}
    for k, _ := range faults {
        params.Faults.Set(k)
    }
    var actorError aerrors.ActorError
    var enc []byte
    // TODO: Add MarshalCBOR and UnmarshalCBOR for DeclareFaultsParams
    //enc, actorError = actors.SerializeParams(&params)
    if actorError != nil {
        err = actorError
        return
    }

    return adapter.callMinerActorMethod(ctx, actors.MAMethods.DeclareFaults, enc)
}

func (adapter lotusAdapter) SubmitDeclaredRecoveries(ctx context.Context, recovered BitField) (cid cid.Cid, err error) {
    // TODO: The method is missing on the miner actor
    panic("implement me")
}
