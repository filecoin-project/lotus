package actors

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"

	"github.com/filecoin-project/go-amt-ipld"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

const POST_SECTORS_COUNT = 8192

type StorageMinerActor struct{}

type StorageMinerActorState struct {
	// Contains mostly static info about this miner
	Info cid.Cid

	// Collateral that is waiting to be withdrawn.
	DePledgedCollateral types.BigInt

	// Time at which the depledged collateral may be withdrawn.
	DePledgeTime types.BigInt

	// All sectors this miner has committed.
	Sectors cid.Cid

	// Sectors this miner is currently mining. It is only updated
	// when a PoSt is submitted (not as each new sector commitment is added).
	ProvingSet cid.Cid

	// Faulty sectors reported since last SubmitPost,
	// up to the current proving period's challenge time.
	CurrentFaultSet types.BitField

	// Faults submitted after the current proving period's challenge time,
	// but before the PoSt for that period is submitted.
	// These become the currentFaultSet when a PoSt is submitted.
	NextFaultSet types.BitField

	// Sectors reported during the last PoSt submission as being 'done'.
	// The collateral for them is still being held until
	// the next PoSt submission in case early sector
	// removal penalization is needed.
	NextDoneSet types.BitField

	// Deals this miner has been slashed for since the last post submission.
	//TODO: unsupported map key type "Cid" (if you want to use struct keys, your atlas needs a transform to string)
	//ArbitratedDeals map[cid.Cid]struct{}

	// Amount of power this miner has.
	Power types.BigInt

	// List of sectors that this miner was slashed for.
	//SlashedSet SectorSet

	// The height at which this miner was slashed at.
	SlashedAt types.BigInt

	// The amount of storage collateral that is owed to clients, and cannot be used for collateral anymore.
	OwedStorageCollateral types.BigInt

	ProvingPeriodEnd uint64
}

type MinerInfo struct {
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
}

type StorageMinerConstructorParams struct {
	Owner      address.Address
	Worker     address.Address
	SectorSize types.BigInt
	PeerID     peer.ID
}

type maMethods struct {
	Constructor            uint64
	CommitSector           uint64
	SubmitPoSt             uint64
	SlashStorageFault      uint64
	GetCurrentProvingSet   uint64
	ArbitrateDeal          uint64
	DePledge               uint64
	GetOwner               uint64
	GetWorkerAddr          uint64
	GetPower               uint64
	GetPeerID              uint64
	GetSectorSize          uint64
	UpdatePeerID           uint64
	ChangeWorker           uint64
	IsSlashed              uint64
	IsLate                 uint64
	PaymentVerifyInclusion uint64
	PaymentVerifySector    uint64
	AddFaults              uint64
	SlashConsensusFault    uint64
}

var MAMethods = maMethods{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

func (sma StorageMinerActor) Exports() []interface{} {
	return []interface{}{
		1: sma.StorageMinerConstructor,
		2: sma.CommitSector,
		3: sma.SubmitPoSt,
		//4:  sma.SlashStorageFault,
		//5: sma.GetCurrentProvingSet,
		//6:  sma.ArbitrateDeal,
		//7:  sma.DePledge,
		8:  sma.GetOwner,
		9:  sma.GetWorkerAddr,
		10: sma.GetPower,
		11: sma.GetPeerID,
		12: sma.GetSectorSize,
		13: sma.UpdatePeerID,
		//14: sma.ChangeWorker,
		//15: sma.IsSlashed,
		//16: sma.IsLate,
		17: sma.PaymentVerifyInclusion,
		18: sma.PaymentVerifySector,
		19: sma.AddFaults,
		20: sma.SlashConsensusFault,
	}
}

func loadState(vmctx types.VMContext) (cid.Cid, *StorageMinerActorState, ActorError) {
	var self StorageMinerActorState
	oldstate := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(oldstate, &self); err != nil {
		return cid.Undef, nil, err
	}

	return oldstate, &self, nil
}

func loadMinerInfo(vmctx types.VMContext, m *StorageMinerActorState) (*MinerInfo, ActorError) {
	var mi MinerInfo
	if err := vmctx.Storage().Get(m.Info, &mi); err != nil {
		return nil, err
	}

	return &mi, nil
}

func (sma StorageMinerActor) StorageMinerConstructor(act *types.Actor, vmctx types.VMContext, params *StorageMinerConstructorParams) ([]byte, ActorError) {
	minerInfo := &MinerInfo{
		Owner:      params.Owner,
		Worker:     params.Worker,
		PeerID:     params.PeerID,
		SectorSize: params.SectorSize,
	}

	minfocid, err := vmctx.Storage().Put(minerInfo)
	if err != nil {
		return nil, err
	}

	var self StorageMinerActorState
	sectors := amt.NewAMT(types.WrapStorage(vmctx.Storage()))
	scid, serr := sectors.Flush()
	if serr != nil {
		return nil, aerrors.Escalate(serr, "initializing AMT")
	}

	self.Sectors = scid
	self.ProvingSet = scid
	self.Info = minfocid

	storage := vmctx.Storage()
	c, err := storage.Put(&self)
	if err != nil {
		return nil, err
	}

	if err := storage.Commit(EmptyCBOR, c); err != nil {
		return nil, err
	}

	return nil, nil
}

type CommitSectorParams struct {
	SectorID  uint64
	CommD     []byte
	CommR     []byte
	CommRStar []byte
	Proof     []byte
}

func (sma StorageMinerActor) CommitSector(act *types.Actor, vmctx types.VMContext, params *CommitSectorParams) ([]byte, ActorError) {
	ctx := context.TODO()
	oldstate, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	// TODO: this needs to get normalized to either the ID address or the actor address
	maddr := vmctx.Message().To

	if ok, err := ValidatePoRep(maddr, mi.SectorSize, params); err != nil {
		return nil, err
	} else if !ok {
		return nil, aerrors.New(1, "bad proof!")
	}

	// make sure the miner isnt trying to submit a pre-existing sector
	unique, err := SectorIsUnique(ctx, vmctx.Storage(), self.Sectors, params.SectorID)
	if err != nil {
		return nil, err
	}
	if !unique {
		return nil, aerrors.New(2, "sector already committed!")
	}

	// Power of the miner after adding this sector
	futurePower := types.BigAdd(self.Power, mi.SectorSize)
	collateralRequired := CollateralForPower(futurePower)

	if act.Balance.LessThan(collateralRequired) {
		return nil, aerrors.New(3, "not enough collateral")
	}

	// Note: There must exist a unique index in the miner's sector set for each
	// sector ID. The `faults`, `recovered`, and `done` parameters of the
	// SubmitPoSt method express indices into this sector set.
	nssroot, err := AddToSectorSet(ctx, vmctx.Storage(), self.Sectors, params.SectorID, params.CommR, params.CommD)
	if err != nil {
		return nil, err
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
	pss, lerr := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingSet)
	if lerr != nil {
		return nil, aerrors.Escalate(lerr, "could not load proving set node")
	}

	if pss.Count == 0 {
		self.ProvingSet = self.Sectors
		self.ProvingPeriodEnd = vmctx.BlockHeight() + build.ProvingPeriodDuration
	}

	nstate, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}
	if err := vmctx.Storage().Commit(oldstate, nstate); err != nil {
		return nil, err
	}

	return nil, nil
}

type SubmitPoStParams struct {
	Proof   []byte
	DoneSet types.BitField
	// TODO: once the spec changes finish, we have more work to do here...
}

// TODO: this is a dummy method that allows us to plumb in other parts of the
// system for now.
func (sma StorageMinerActor) SubmitPoSt(act *types.Actor, vmctx types.VMContext, params *SubmitPoStParams) ([]byte, ActorError) {
	oldstate, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	if vmctx.Message().From != mi.Worker {
		return nil, aerrors.New(1, "not authorized to submit post for miner")
	}

	provingPeriodOffset := self.ProvingPeriodEnd % build.ProvingPeriodDuration
	provingPeriod := (vmctx.BlockHeight() - provingPeriodOffset - 1) / build.ProvingPeriodDuration + 1
	currentProvingPeriodEnd := provingPeriod * build.ProvingPeriodDuration + provingPeriodOffset

	feesRequired := types.NewInt(0)

	if currentProvingPeriodEnd > self.ProvingPeriodEnd {
		//TODO late fee calc
		feesRequired = types.BigAdd(feesRequired, types.NewInt(1000))
	}

	//TODO temporary sector failure fees

	msgVal := vmctx.Message().Value
	if msgVal.LessThan(feesRequired) {
		return nil, aerrors.New(2, "not enough funds to pay post submission fees")
	}

	if msgVal.GreaterThan(feesRequired) {
		_, err := vmctx.Send(vmctx.Message().From, 0,
			types.BigSub(msgVal, feesRequired), nil)
		if err != nil {
			return nil, aerrors.Wrap(err, "could not refund excess fees")
		}
	}

	var seed [sectorbuilder.CommLen]byte
	{
		randHeight := currentProvingPeriodEnd - build.PoSTChallangeTime
		if vmctx.BlockHeight() <= randHeight {
			// TODO: spec, retcode
			return nil, aerrors.New(1, "submit PoSt called outside submission window")
		}

		rand, err := vmctx.GetRandomness(randHeight)

		if err != nil {
			return nil, aerrors.Wrap(err, "could not get randomness for PoST")
		}
		if len(rand) < len(seed) {
			return nil, aerrors.Escalate(fmt.Errorf("randomness too small (%d < %d)",
				len(rand), len(seed)), "improper randomness")
		}
		copy(seed[:], rand)
	}

	pss, lerr := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingSet)
	if lerr != nil {
		return nil, aerrors.Escalate(lerr, "could not load proving set node")
	}

	var sectorInfos []sectorbuilder.SectorInfo
	if err := pss.ForEach(func(id uint64, v *cbg.Deferred) error {
		var comms [][]byte
		if err := cbor.DecodeInto(v.Raw, &comms); err != nil {
			return xerrors.New("could not decode comms")
		}
		si := sectorbuilder.SectorInfo{
			SectorID: id,
		}
		commR := comms[0]
		if len(commR) != len(si.CommR) {
			return xerrors.Errorf("commR length is wrong: %d", len(commR))
		}
		copy(si.CommR[:], commR)

		sectorInfos = append(sectorInfos, si)

		return nil
	}); err != nil {
		return nil, aerrors.Absorb(err, 3, "could not decode sectorset")
	}

	faults := self.CurrentFaultSet.All()

	if ok, lerr := sectorbuilder.VerifyPost(mi.SectorSize.Uint64(),
		sectorbuilder.NewSortedSectorInfo(sectorInfos), seed, params.Proof,
		faults); !ok || lerr != nil {
		if lerr != nil {
			// TODO: study PoST errors
			return nil, aerrors.Absorb(lerr, 4, "PoST error")
		}
		if !ok {
			return nil, aerrors.New(4, "PoST invalid")
		}
	}
	self.CurrentFaultSet = self.NextFaultSet
	self.NextFaultSet = types.NewBitField()

	ss, lerr := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingSet)
	if lerr != nil {
		return nil, aerrors.Escalate(lerr, "could not load proving set node")
	}

	if err := ss.BatchDelete(params.DoneSet.All()); err != nil {
		// TODO: this could fail for system reasons (block not found) or for
		// bad user input reasons (e.g. bad doneset). The latter should be a
		// non-fatal error
		return nil, aerrors.Escalate(err, "failed to delete sectors in done set")
	}

	self.ProvingSet, lerr = ss.Flush()
	if lerr != nil {
		return nil, aerrors.Escalate(lerr, "could not flush AMT")
	}

	oldPower := self.Power
	self.Power = types.BigMul(types.NewInt(pss.Count-uint64(len(faults))),
		mi.SectorSize)

	enc, err := SerializeParams(&UpdateStorageParams{Delta: types.BigSub(self.Power, oldPower)})
	if err != nil {
		return nil, err
	}

	_, err = vmctx.Send(StorageMarketAddress, SMAMethods.UpdateStorage, types.NewInt(0), enc)
	if err != nil {
		return nil, err
	}

	self.ProvingSet = self.Sectors
	self.ProvingPeriodEnd = currentProvingPeriodEnd + build.ProvingPeriodDuration
	self.NextDoneSet = params.DoneSet

	c, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(oldstate, c); err != nil {
		return nil, err
	}

	return nil, nil
}

func (sma StorageMinerActor) GetPower(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}
	return self.Power.Bytes(), nil
}

func SectorIsUnique(ctx context.Context, s types.Storage, sroot cid.Cid, sid uint64) (bool, ActorError) {
	found, _, _, err := GetFromSectorSet(ctx, s, sroot, sid)
	if err != nil {
		return false, err
	}

	return !found, nil
}

func AddToSectorSet(ctx context.Context, s types.Storage, ss cid.Cid, sectorID uint64, commR, commD []byte) (cid.Cid, ActorError) {
	ssr, err := amt.LoadAMT(types.WrapStorage(s), ss)
	if err != nil {
		return cid.Undef, aerrors.Escalate(err, "could not load sector set node")
	}

	if err := ssr.Set(sectorID, [][]byte{commR, commD}); err != nil {
		return cid.Undef, aerrors.Escalate(err, "failed to set commitment in sector set")
	}

	ncid, err := ssr.Flush()
	if err != nil {
		return cid.Undef, aerrors.Escalate(err, "failed to flush sector set")
	}

	return ncid, nil
}

func GetFromSectorSet(ctx context.Context, s types.Storage, ss cid.Cid, sectorID uint64) (bool, []byte, []byte, ActorError) {
	ssr, err := amt.LoadAMT(types.WrapStorage(s), ss)
	if err != nil {
		return false, nil, nil, aerrors.Escalate(err, "could not load sector set node")
	}

	var comms [][]byte
	err = ssr.Get(sectorID, &comms)
	if err != nil {
		if _, ok := err.(*amt.ErrNotFound); ok {
			return false, nil, nil, nil
		}
		return false, nil, nil, aerrors.Escalate(err, "failed to find sector in sector set")
	}

	if len(comms) != 2 {
		return false, nil, nil, aerrors.Escalate(xerrors.New("sector set entry should only have 2 elements"), "")
	}

	return true, comms[0], comms[1], nil
}

func ValidatePoRep(maddr address.Address, ssize types.BigInt, params *CommitSectorParams) (bool, ActorError) {
	ok, err := sectorbuilder.VerifySeal(ssize.Uint64(), params.CommR, params.CommD, params.CommRStar, maddr, params.SectorID, params.Proof)
	if err != nil {
		return false, aerrors.Escalate(err, "verify seal failed")
	}

	return ok, nil
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

func (sma StorageMinerActor) GetWorkerAddr(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	return mi.Worker.Bytes(), nil
}

func (sma StorageMinerActor) GetOwner(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	return mi.Owner.Bytes(), nil
}

func (sma StorageMinerActor) GetPeerID(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	return []byte(mi.PeerID), nil
}

type UpdatePeerIDParams struct {
	PeerID peer.ID
}

func (sma StorageMinerActor) UpdatePeerID(act *types.Actor, vmctx types.VMContext, params *UpdatePeerIDParams) ([]byte, ActorError) {
	oldstate, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	if vmctx.Message().From != mi.Worker {
		return nil, aerrors.New(2, "only the mine worker may update the peer ID")
	}

	mi.PeerID = params.PeerID

	mic, err := vmctx.Storage().Put(mi)
	if err != nil {
		return nil, err
	}

	self.Info = mic

	c, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(oldstate, c); err != nil {
		return nil, err
	}

	return nil, nil
}

func (sma StorageMinerActor) GetSectorSize(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	return mi.SectorSize.Bytes(), nil
}

type PaymentVerifyParams struct {
	Extra []byte
	Proof []byte
}

type PieceInclVoucherData struct { // TODO: Update spec at https://github.com/filecoin-project/specs/blob/master/actors.md#paymentverify
	CommP     []byte
	PieceSize types.BigInt
}

type InclusionProof struct {
	Sector uint64 // for CommD, also verifies the sector is in sector set
	Proof  []byte
}

func (sma StorageMinerActor) PaymentVerifyInclusion(act *types.Actor, vmctx types.VMContext, params *PaymentVerifyParams) ([]byte, ActorError) {
	// params.Extra - PieceInclVoucherData
	// params.Proof - InclusionProof

	_, self, aerr := loadState(vmctx)
	if aerr != nil {
		return nil, aerr
	}
	mi, aerr := loadMinerInfo(vmctx, self)
	if aerr != nil {
		return nil, aerr
	}

	var voucherData PieceInclVoucherData
	if err := cbor.DecodeInto(params.Extra, &voucherData); err != nil {
		return nil, aerrors.Escalate(err, "failed to decode storage voucher data for verification")
	}
	var proof InclusionProof
	if err := cbor.DecodeInto(params.Proof, &proof); err != nil {
		return nil, aerrors.Escalate(err, "failed to decode storage payment proof")
	}

	ok, _, commD, aerr := GetFromSectorSet(context.TODO(), vmctx.Storage(), self.Sectors, proof.Sector)
	if aerr != nil {
		return nil, aerr
	}
	if !ok {
		return nil, aerrors.New(1, "miner does not have required sector")
	}

	ok, err := sectorbuilder.VerifyPieceInclusionProof(mi.SectorSize.Uint64(), voucherData.PieceSize.Uint64(), voucherData.CommP, commD, proof.Proof)
	if err != nil {
		return nil, aerrors.Escalate(err, "verify piece inclusion proof failed")
	}
	if !ok {
		return nil, aerrors.New(2, "piece inclusion proof was invalid")
	}

	return nil, nil
}

func (sma StorageMinerActor) PaymentVerifySector(act *types.Actor, vmctx types.VMContext, params *PaymentVerifyParams) ([]byte, ActorError) {
	// params.Extra - BigInt - sector id
	// params.Proof - nil

	_, self, aerr := loadState(vmctx)
	if aerr != nil {
		return nil, aerr
	}

	// TODO: ensure no sector ID reusability within related deal lifetime
	sector := types.BigFromBytes(params.Extra)

	if len(params.Proof) > 0 {
		return nil, aerrors.New(1, "unexpected proof bytes")
	}

	ok, _, _, aerr := GetFromSectorSet(context.TODO(), vmctx.Storage(), self.Sectors, sector.Uint64())
	if aerr != nil {
		return nil, aerr
	}
	if !ok {
		return nil, aerrors.New(2, "miner does not have required sector")
	}

	return nil, nil
}

type AddFaultsParams struct {
	Faults types.BitField
}

func (sma StorageMinerActor) AddFaults(act *types.Actor, vmctx types.VMContext, params *AddFaultsParams) ([]byte, ActorError) {
	oldstate, self, aerr := loadState(vmctx)
	if aerr != nil {
		return nil, aerr
	}

	challengeHeight := self.ProvingPeriodEnd - build.PoSTChallangeTime

	if vmctx.BlockHeight() < challengeHeight {
		// TODO: optimized bitfield methods
		for _, v := range params.Faults.All() {
			self.CurrentFaultSet.Set(v)
		}
	} else {
		for _, v := range params.Faults.All() {
			self.NextFaultSet.Set(v)
		}
	}

	nstate, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}
	if err := vmctx.Storage().Commit(oldstate, nstate); err != nil {
		return nil, err
	}

	return nil, nil
}

type MinerSlashConsensusFault struct {
	Slasher           address.Address
	AtHeight          uint64
	SlashedCollateral types.BigInt
}

func (sma StorageMinerActor) SlashConsensusFault(act *types.Actor, vmctx types.VMContext, params *MinerSlashConsensusFault) ([]byte, ActorError) {
	if vmctx.Message().From != StorageMarketAddress {
		return nil, aerrors.New(1, "SlashConsensusFault may only be called by the storage market actor")
	}

	slashedCollateral := params.SlashedCollateral
	if slashedCollateral.LessThan(act.Balance) {
		slashedCollateral = act.Balance
	}

	// Some of the slashed collateral should be paid to the slasher
	// GROWTH_RATE determines how fast the slasher share of slashed collateral will increase as block elapses
	// current GROWTH_RATE results in SLASHER_SHARE reaches 1 after 30 blocks
	// TODO: define arithmetic precision and rounding for this operation
	blockElapsed := vmctx.BlockHeight() - params.AtHeight

	slasherShare := slasherShare(params.SlashedCollateral, blockElapsed)

	burnPortion := types.BigSub(slashedCollateral, slasherShare)

	_, err := vmctx.Send(vmctx.Message().From, 0, slasherShare, nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to pay slasher")
	}

	_, err = vmctx.Send(BurntFundsAddress, 0, burnPortion, nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to burn funds")
	}

	// TODO: this still allows the miner to commit sectors and submit posts,
	// their users could potentially be unaffected, but the miner will never be
	// able to mine a block again
	// One potential issue: the miner will have to pay back the slashed
	// collateral to continue submitting PoSts, which includes pledge
	// collateral that they no longer really 'need'

	return nil, nil
}

func slasherShare(total types.BigInt, elapsed uint64) types.BigInt {
	// [int(pow(1.26, n) * 10) for n in range(30)]
	fracs := []uint64{10, 12, 15, 20, 25, 31, 40, 50, 63, 80, 100, 127, 160, 201, 254, 320, 403, 508, 640, 807, 1017, 1281, 1614, 2034, 2563, 3230, 4070, 5128, 6462, 8142}
	const precision = 10000

	var frac uint64
	if elapsed >= uint64(len(fracs)) {
		return total
	} else {
		frac = fracs[elapsed]
	}

	return types.BigDiv(
		types.BigMul(
			types.NewInt(frac),
			total,
		),
		types.NewInt(precision),
	)
}
