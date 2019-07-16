package actors

import (
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
)

const SectorSize = 1024

func init() {
	cbor.RegisterCborType(StorageMarketState{})
	cbor.RegisterCborType(CreateStorageMinerParams{})
}

type StorageMarketActor struct{}

func (sma StorageMarketActor) Exports() []interface{} {
	return []interface{}{
		nil,
		sma.CreateStorageMiner,
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

func (sma StorageMarketActor) CreateStorageMiner(act *types.Actor, vmctx types.VMContext, params *CreateStorageMinerParams) (types.InvokeRet, error) {
	if !SupportedSectorSize(params.SectorSize) {
		//Fatal("Unsupported sector size")
		return types.InvokeRet{
			ReturnCode: 1,
		}, nil
	}

	encoded, err := CreateExecParams(StorageMinerCodeCid, &StorageMinerConstructorParams{
		Owner:      params.Owner,
		Worker:     params.Worker,
		SectorSize: params.SectorSize,
		PeerID:     params.PeerID,
	})
	if err != nil {
		return types.InvokeRet{}, err
	}

	ret, exit, err := vmctx.Send(InitActorAddress, 1, vmctx.Message().Value, encoded)
	if err != nil {
		return types.InvokeRet{}, err
	}

	naddr, err := address.NewFromBytes(ret)
	if err != nil {
		return types.InvokeRet{}, err
	}

	if exit != 0 {
		return types.InvokeRet{
			ReturnCode: 2,
		}, nil
	}

	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return types.InvokeRet{}, err
	}

	self.Miners[naddr] = struct{}{}

	nroot, err := vmctx.Storage().Put(self)
	if err != nil {
		return types.InvokeRet{}, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return types.InvokeRet{}, err
	}

	return types.InvokeRet{
		Result: naddr.Bytes(),
	}, nil
}

func SupportedSectorSize(ssize types.BigInt) bool {
	if ssize.Uint64() == SectorSize {
		return true
	}
	return false
}
