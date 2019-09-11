package actors

import (
	"context"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type StorageMarketActor struct{}

type smaMethods struct {
	Constructor         uint64
	CreateStorageMiner  uint64
	SlashConsensusFault uint64
	UpdateStorage       uint64
	GetTotalStorage     uint64
	PowerLookup         uint64
	IsMiner             uint64
}

var SMAMethods = smaMethods{1, 2, 3, 4, 5, 6, 7}

func (sma StorageMarketActor) Exports() []interface{} {
	return []interface{}{
		//1: sma.StorageMarketConstructor,
		2: sma.CreateStorageMiner,
		//3: sma.SlashConsensusFault,
		4: sma.UpdateStorage,
		5: sma.GetTotalStorage,
		6: sma.PowerLookup,
		7: sma.IsMiner,
		//8: sma.StorageCollateralForSize,
	}
}

type StorageMarketState struct {
	Miners       cid.Cid
	TotalStorage types.BigInt
}

type CreateStorageMinerParams struct {
	Owner      address.Address
	Worker     address.Address
	SectorSize types.BigInt
	PeerID     peer.ID
}

func (sma StorageMarketActor) CreateStorageMiner(act *types.Actor, vmctx types.VMContext, params *CreateStorageMinerParams) ([]byte, ActorError) {
	if !SupportedSectorSize(params.SectorSize) {
		return nil, aerrors.New(1, "Unsupported sector size")
	}

	encoded, err := CreateExecParams(StorageMinerCodeCid, &StorageMinerConstructorParams{
		Owner:      params.Owner,
		Worker:     params.Worker,
		SectorSize: params.SectorSize,
		PeerID:     params.PeerID,
	})
	if err != nil {
		return nil, err
	}

	ret, err := vmctx.Send(InitActorAddress, IAMethods.Exec, vmctx.Message().Value, encoded)
	if err != nil {
		return nil, err
	}

	naddr, nerr := address.NewFromBytes(ret)
	if nerr != nil {
		return nil, aerrors.Absorb(nerr, 1, "could not read address of new actor")
	}

	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	ncid, err := MinerSetAdd(context.TODO(), vmctx, self.Miners, naddr)
	if err != nil {
		return nil, err
	}
	self.Miners = ncid

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return nil, err
	}

	return naddr.Bytes(), nil
}

func SupportedSectorSize(ssize types.BigInt) bool {
	if ssize.Uint64() == build.SectorSize {
		return true
	}
	return false
}

type UpdateStorageParams struct {
	Delta types.BigInt
}

func (sma StorageMarketActor) UpdateStorage(act *types.Actor, vmctx types.VMContext, params *UpdateStorageParams) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	has, err := MinerSetHas(context.TODO(), vmctx, self.Miners, vmctx.Message().From)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, aerrors.New(1, "update storage must only be called by a miner actor")
	}

	self.TotalStorage = types.BigAdd(self.TotalStorage, params.Delta)

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return nil, err
	}

	return nil, nil
}

func (sma StorageMarketActor) GetTotalStorage(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, err
	}

	return self.TotalStorage.Bytes(), nil
}

type PowerLookupParams struct {
	Miner address.Address
}

func (sma StorageMarketActor) PowerLookup(act *types.Actor, vmctx types.VMContext, params *PowerLookupParams) ([]byte, ActorError) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, aerrors.Wrap(err, "getting head")
	}

	has, err := MinerSetHas(context.TODO(), vmctx, self.Miners, params.Miner)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, aerrors.New(1, "miner not registered with storage market")
	}

	ret, err := vmctx.Send(params.Miner, MAMethods.GetPower, types.NewInt(0), nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "invoke Miner.GetPower")
	}

	return ret, nil
}

type IsMinerParam struct {
	Addr address.Address
}

func (sma StorageMarketActor) IsMiner(act *types.Actor, vmctx types.VMContext, param *IsMinerParam) ([]byte, ActorError) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, err
	}

	has, err := MinerSetHas(context.TODO(), vmctx, self.Miners, param.Addr)
	if err != nil {
		return nil, err
	}

	return cbg.EncodeBool(has), nil
}

func MinerSetHas(ctx context.Context, vmctx types.VMContext, rcid cid.Cid, maddr address.Address) (bool, aerrors.ActorError) {
	nd, err := hamt.LoadNode(ctx, vmctx.Ipld(), rcid)
	if err != nil {
		return false, aerrors.Escalate(err, "failed to load miner set")
	}

	err = nd.Find(ctx, string(maddr.Bytes()), nil)
	switch err {
	case hamt.ErrNotFound:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, aerrors.Escalate(err, "failed to do set lookup")
	}
}

func MinerSetAdd(ctx context.Context, vmctx types.VMContext, rcid cid.Cid, maddr address.Address) (cid.Cid, aerrors.ActorError) {
	nd, err := hamt.LoadNode(ctx, vmctx.Ipld(), rcid)
	if err != nil {
		return cid.Undef, aerrors.Escalate(err, "failed to load miner set")
	}

	if err := nd.Set(ctx, string(maddr.Bytes()), uint64(1)); err != nil {
		return cid.Undef, aerrors.Escalate(err, "adding miner address to set failed")
	}

	if err := nd.Flush(ctx); err != nil {
		return cid.Undef, aerrors.Escalate(err, "failed to flush miner set")
	}

	c, err := vmctx.Ipld().Put(ctx, nd)
	if err != nil {
		return cid.Undef, aerrors.Escalate(err, "failed to persist miner set to storage")
	}

	return c, nil
}
