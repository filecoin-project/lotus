package actors

import (
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() {
	cbor.RegisterCborType(StorageMinerActorState{})
	cbor.RegisterCborType(StorageMinerConstructorParams{})
}

type StorageMinerActor struct{}

type StorageMinerActorState struct {
	// Account that owns this miner.
	// - Income and returned collateral are paid to this address.
	// - This address is also allowed to change the worker address for the miner.
	Owner address.Address

	// Worker account for this miner.
	// This will be the key that is used to sign blocks created by this miner, and
	// sign messages sent on behalf of this miner to commit sectors, submit PoSts, and
	// other day to day miner activities.
	Worker address.Address

	// Libp2p identity that should be used when connecting to this miner.
	PeerID peer.ID

	// Amount of space in each sector committed to the network by this miner.
	SectorSize types.BigInt

	// Collateral currently committed to live storage.
	ActiveCollateral types.BigInt

	// Collateral that is waiting to be withdrawn.
	DePledgedCollateral types.BigInt

	// Time at which the depledged collateral may be withdrawn.
	DePledgeTime types.BigInt

	// All sectors this miner has committed.
	//Sectors SectorSet

	// Sectors this miner is currently mining. It is only updated
	// when a PoSt is submitted (not as each new sector commitment is added).
	//ProvingSet SectorSet

	// Sectors reported during the last PoSt submission as being 'done'. The collateral
	// for them is still being held until the next PoSt submission in case early sector
	// removal penalization is needed.
	//NextDoneSet BitField

	// Deals this miner has been slashed for since the last post submission.
	ArbitratedDeals map[cid.Cid]struct{}

	// Amount of power this miner has.
	Power types.BigInt

	// List of sectors that this miner was slashed for.
	//SlashedSet SectorSet

	// The height at which this miner was slashed at.
	SlashedAt types.BigInt

	// The amount of storage collateral that is owed to clients, and cannot be used for collateral anymore.
	OwedStorageCollateral types.BigInt
}

type StorageMinerConstructorParams struct {
	Worker     address.Address
	SectorSize types.BigInt
	PeerID     peer.ID
}

func (sma StorageMinerActor) StorageMinerActor(act *types.Actor, vmctx types.VMContext, params *StorageMinerConstructorParams) (types.InvokeRet, error) {
	var self StorageMinerActorState
	self.Owner = vmctx.Message().From
	self.Worker = params.Worker
	self.PeerID = params.PeerID
	self.SectorSize = params.SectorSize

	storage := vmctx.Storage()
	c, err := storage.Put(self)
	if err != nil {
		return types.InvokeRet{}, err
	}

	if err := storage.Commit(cid.Undef, c); err != nil {
		return types.InvokeRet{}, err
	}

	return types.InvokeRet{}, nil
}
