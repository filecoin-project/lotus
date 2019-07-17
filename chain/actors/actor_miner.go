package actors

import (
	"context"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
)

func init() {
	cbor.RegisterCborType(StorageMinerActorState{})
	cbor.RegisterCborType(StorageMinerConstructorParams{})
	cbor.RegisterCborType(CommitSectorParams{})
}

var ProvingPeriodDuration = uint64(2 * 60) // an hour, for now
const POST_SECTORS_COUNT = 8192

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

	// Collateral that is waiting to be withdrawn.
	DePledgedCollateral types.BigInt

	// Time at which the depledged collateral may be withdrawn.
	DePledgeTime types.BigInt

	// All sectors this miner has committed.
	Sectors       cid.Cid // TODO: Using a HAMT for now, needs to be an AMT once we implement it
	SectorSetSize uint64  // TODO: the AMT should be able to tell us how many items are in it. This field won't be needed at that point

	// Sectors this miner is currently mining. It is only updated
	// when a PoSt is submitted (not as each new sector commitment is added).
	ProvingSet     cid.Cid
	ProvingSetSize uint64

	// Sectors reported during the last PoSt submission as being 'done'. The collateral
	// for them is still being held until the next PoSt submission in case early sector
	// removal penalization is needed.
	//NextDoneSet BitField

	// Deals this miner has been slashed for since the last post submission.
	//TODO: unsupported map key type "Cid" (if you want to use struct keys, your atlas needs a transform to string)
	//ArbitratedDeals map[cid.Cid]struct{}

	// Amount of power this miner has.
	Power types.BigInt

	ProvingPeriodEnd uint64

	// List of sectors that this miner was slashed for.
	//SlashedSet SectorSet

	// The height at which this miner was slashed at.
	SlashedAt types.BigInt

	// The amount of storage collateral that is owed to clients, and cannot be used for collateral anymore.
	OwedStorageCollateral types.BigInt
}

type StorageMinerConstructorParams struct {
	Owner      address.Address
	Worker     address.Address
	SectorSize types.BigInt
	PeerID     peer.ID
}

func (sma StorageMinerActor) Exports() []interface{} {
	return []interface{}{
		0: sma.StorageMinerConstructor,
		1: sma.CommitSector,
		//2:  sma.SubmitPost,
		//3:  sma.SlashStorageFault,
		//4:  sma.GetCurrentProvingSet,
		//5:  sma.ArbitrateDeal,
		//6:  sma.DePledge,
		//7:  sma.GetOwner,
		//8:  sma.GetWorkerAddr,
		9: sma.GetPower,
		//10: sma.GetPeerID,
		//11: sma.GetSectorSize,
		//12: sma.UpdatePeerID,
		//13: sma.ChangeWorker,
	}
}

func (sma StorageMinerActor) StorageMinerConstructor(act *types.Actor, vmctx types.VMContext, params *StorageMinerConstructorParams) (types.InvokeRet, error) {
	var self StorageMinerActorState
	self.Owner = params.Owner
	self.Worker = params.Worker
	self.PeerID = params.PeerID
	self.SectorSize = params.SectorSize

	nd := hamt.NewNode(vmctx.Ipld())
	sectors, err := vmctx.Ipld().Put(context.TODO(), nd)
	if err != nil {
		return types.InvokeRet{}, err
	}
	self.Sectors = sectors
	self.ProvingSet = sectors

	storage := vmctx.Storage()
	c, err := storage.Put(self)
	if err != nil {
		return types.InvokeRet{}, err
	}

	if err := storage.Commit(EmptyCBOR, c); err != nil {
		return types.InvokeRet{}, err
	}

	return types.InvokeRet{}, nil
}

type CommitSectorParams struct {
	SectorId  types.BigInt
	CommD     []byte
	CommR     []byte
	CommRStar []byte
	Proof     []byte
}

func (sma StorageMinerActor) CommitSector(act *types.Actor, vmctx types.VMContext, params *CommitSectorParams) (types.InvokeRet, error) {
	var self StorageMinerActorState
	oldstate := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(oldstate, &self); err != nil {
		return types.InvokeRet{}, err
	}

	if !ValidatePoRep(self.SectorSize, params) {
		//Fatal("bad proof!")
		return types.InvokeRet{
			ReturnCode: 1,
		}, nil
	}

	// make sure the miner isnt trying to submit a pre-existing sector
	unique, err := SectorIsUnique(vmctx.Ipld(), self.Sectors, params.SectorId)
	if err != nil {
		return types.InvokeRet{}, err
	}
	if !unique {
		//Fatal("sector already committed!")
		return types.InvokeRet{
			ReturnCode: 2,
		}, nil
	}

	// Power of the miner after adding this sector
	futurePower := types.BigAdd(self.Power, self.SectorSize)
	collateralRequired := CollateralForPower(futurePower)

	if types.BigCmp(collateralRequired, act.Balance) < 0 {
		//Fatal("not enough collateral")
		return types.InvokeRet{
			ReturnCode: 3,
		}, nil
	}

	// ensure that the miner cannot commit more sectors than can be proved with a single PoSt
	if self.SectorSetSize >= POST_SECTORS_COUNT {
		// Fatal("too many sectors")
		return types.InvokeRet{
			ReturnCode: 4,
		}, nil
	}

	// Note: There must exist a unique index in the miner's sector set for each
	// sector ID. The `faults`, `recovered`, and `done` parameters of the
	// SubmitPoSt method express indices into this sector set.
	nssroot, err := AddToSectorSet(self.Sectors, params.SectorId, params.CommR, params.CommD)
	if err != nil {
		return types.InvokeRet{}, err
	}
	self.Sectors = nssroot

	// if miner is not mining, start their proving period now
	// Note: As written here, every miners first PoSt will only be over one sector.
	// We could set up a 'grace period' for starting mining that would allow miners
	// to submit several sectors for their first proving period. Alternatively, we
	// could simply make the 'CommitSector' call take multiple sectors at a time.
	//
	// Note: Proving period is a function of sector size; small sectors take less
	// time to prove than large sectors do. Sector size is selected when pledging.
	if self.ProvingSetSize == 0 {
		self.ProvingSet = self.Sectors
		self.ProvingSetSize = self.SectorSetSize
		self.ProvingPeriodEnd = vmctx.BlockHeight() + ProvingPeriodDuration
	}

	nstate, err := vmctx.Storage().Put(self)
	if err != nil {
		return types.InvokeRet{}, err
	}
	if err := vmctx.Storage().Commit(oldstate, nstate); err != nil {
		return types.InvokeRet{}, err
	}

	return types.InvokeRet{}, nil
}

func (sma StorageMinerActor) GetPower(act *types.Actor, vmctx types.VMContext, params *struct{}) (types.InvokeRet, error) {
	var self StorageMinerActorState
	state := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(state, &self); err != nil {
		return types.InvokeRet{}, err
	}
	return types.InvokeRet{
		Result: self.Power.Bytes(),
	}, nil
}

func SectorIsUnique(cst *hamt.CborIpldStore, sroot cid.Cid, sid types.BigInt) (bool, error) {
	nd, err := hamt.LoadNode(context.TODO(), cst, sroot)
	if err != nil {
		return false, err
	}

	if _, err := nd.Find(context.TODO(), sid.String()); err != nil {
		if err == hamt.ErrNotFound {
			return true, nil
		}
		return false, err
	}

	return false, nil
}

func AddToSectorSet(ss cid.Cid, sectorID types.BigInt, commR, commD []byte) (cid.Cid, error) {
	panic("NYI")
}

func ValidatePoRep(ssize types.BigInt, params *CommitSectorParams) bool {
	return true
}

func CollateralForPower(power types.BigInt) types.BigInt {
	return types.BigMul(power, types.NewInt(10))
	/* TODO: this
	availableFil = FakeGlobalMethods.GetAvailableFil()
	totalNetworkPower = StorageMinerActor.GetTotalStorage()
	numMiners = StorageMarket.GetMinerCount()
	powerCollateral = availableFil * NetworkConstants.POWER_COLLATERAL_PROPORTION * power / totalNetworkPower
	perCapitaCollateral = availableFil * NetworkConstants.PER_CAPITA_COLLATERAL_PROPORTION / numMiners
	collateralRequired = math.Ceil(minerPowerCollateral + minerPerCapitaCollateral)
	return collateralRequired
	*/
}
