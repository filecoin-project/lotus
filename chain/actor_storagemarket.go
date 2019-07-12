package chain

import (
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() {
	cbor.RegisterCborType(StorageMarketState{})
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
	Worker     address.Address
	SectorSize types.BigInt
	PeerID     peer.ID
}

func (p *CreateStorageMinerParams) UnmarshalCBOR(b []byte) (int, error) {
	if err := cbor.DecodeInto(b, p); err != nil {
		return 0, err
	}

	return len(b), nil
}

func (sma StorageMarketActor) CreateStorageMiner(act *types.Actor, vmctx types.VMContext, params *CreateStorageMinerParams) (InvokeRet, error) {
	if !SupportedSectorSize(params.SectorSize) {
		//Fatal("Unsupported sector size")
		return InvokeRet{
			returnCode: 1,
		}, nil
	}

	var smcp StorageMinerConstructorParams
	smcp.Worker = params.Worker
	smcp.SectorSize = params.SectorSize
	smcp.PeerID = params.PeerID

	encoded, err := cbor.DumpObject(smcp)
	if err != nil {
		return InvokeRet{}, err
	}

	ret, exit, err := vmctx.Send(InitActorAddress, 1, vmctx.Message().Value, encoded)
	if err != nil {
		return InvokeRet{}, err
	}

	naddr, err := address.NewFromBytes(ret)
	if err != nil {
		return InvokeRet{}, err
	}

	if exit != 0 {
		return InvokeRet{
			returnCode: 2,
		}, nil
	}

	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return InvokeRet{}, err
	}

	self.Miners[naddr] = struct{}{}

	nroot, err := vmctx.Storage().Put(self)
	if err != nil {
		return InvokeRet{}, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return InvokeRet{}, err
	}

	return InvokeRet{
		result: naddr.Bytes(),
	}, nil
}

func SupportedSectorSize(ssize types.BigInt) bool {
	if ssize.Uint64() == 1024 {
		return true
	}
	return false
}
