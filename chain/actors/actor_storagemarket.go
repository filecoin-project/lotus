package actors

import (
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
)

const SectorSize = 1024

func init() {
	cbor.RegisterCborType(StorageMarketState{})
	cbor.RegisterCborType(CreateStorageMinerParams{})
	cbor.RegisterCborType(IsMinerParam{})
	cbor.RegisterCborType(PowerLookupParams{})
	cbor.RegisterCborType(UpdateStorageParams{})
}

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
	Miners       map[address.Address]struct{}
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

	self.Miners[naddr] = struct{}{}

	nroot, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return nil, err
	}

	return naddr.Bytes(), nil
}

func SupportedSectorSize(ssize types.BigInt) bool {
	if ssize.Uint64() == SectorSize {
		return true
	}
	return false
}

type UpdateStorageParams struct {
	Delta types.BigInt
}

func (sma StorageMarketActor) UpdateStorage(act *types.Actor, vmctx types.VMContext, params *UpdateStorageParams) ([]byte, ActorError) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, err
	}

	_, ok := self.Miners[vmctx.Message().From]
	if !ok {
		return nil, aerrors.New(1, "update storage must only be called by a miner actor")
	}

	self.TotalStorage = types.BigAdd(self.TotalStorage, params.Delta)
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

	if _, ok := self.Miners[params.Miner]; !ok {
		return nil, aerrors.New(1, "miner not registered with storage market")
	}

	ret, err := vmctx.Send(params.Miner, MAMethods.GetPower, types.NewInt(0), EmptyStructCBOR)
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
	_, ok := self.Miners[param.Addr]
	out, err := SerializeParams(ok)
	if err != nil {
		return nil, err
	}
	return out, nil
}
