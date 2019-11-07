package smm

import (
    "bytes"
    "context"
    "encoding/binary"
    "github.com/filecoin-project/lotus/api"
    "github.com/filecoin-project/lotus/build"
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

func readState(headChanges []*store.HeadChange) (stateKey StateKey) {
    // TODO: Implement
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
    state := StateKey(buffer.Bytes())
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
            stateKey := readState(changes)
            adapter.notifyListeners(stateKey)

        case <-adapter.context.Done():
            return
        }
    }
}

func (adapter lotusAdapter) notifyListeners(stateKey StateKey) {
    for listener, _ := range adapter.listeners {
        // TODO: Send the proper epoch
        listener.OnChainStateChanged(Epoch(10), stateKey)
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
    state.Sectors = make(map[uint64]api.SectorInfo)
    for _, sectorInfo := range sectors {
        state.Sectors[sectorInfo.SectorID] = *sectorInfo
    }
    for _, sectorInfo := range provingSectors {
        state.ProvingSet[sectorInfo.SectorID] = struct{}{}
    }
    state.Address = Address(adapter.address.String())
    // fill in this information
    state.PreCommittedSectors = make(map[uint64]api.SectorInfo)
    state.StagedCommittedSectors = make(map[uint64]api.SectorInfo)

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
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorPreCommitment(ctx context.Context, miner Address, id SectorID, commR cid.Cid, deals []cid.Cid) {
    panic("implement me")
}

func (adapter lotusAdapter) GetSealSeed(ctx context.Context, miner Address, state StateKey, id SectorID) SealSeed {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitSectorCommitment(ctx context.Context, miner Address, id SectorID, proof Proof) {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitPoSt(ctx context.Context, miner Address, proof Proof) {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitDeclaredFaults(ctx context.Context, miner Address, faults types.BitField) {
    panic("implement me")
}

func (adapter lotusAdapter) SubmitDeclaredRecoveries(ctx context.Context, miner Address, recovered types.BitField) {
    panic("implement me")
}
