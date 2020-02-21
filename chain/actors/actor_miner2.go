package actors

import (
	"bytes"
	"context"
	"fmt"

	amt2 "github.com/filecoin-project/go-amt-ipld/v2"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

type StorageMinerActor2 struct{}

func (sma StorageMinerActor2) Exports() []interface{} {
	return []interface{}{
		1: sma.StorageMinerConstructor,
		2: sma.PreCommitSector,
		3: sma.ProveCommitSector,
		4: sma.SubmitFallbackPoSt,
		//5: sma.SlashStorageFault,
		//6: sma.GetCurrentProvingSet,
		//7:  sma.ArbitrateDeal,
		//8:  sma.DePledge,
		9:  sma.GetOwner,
		10: sma.GetWorkerAddr,
		11: withUpdates(
			update{0, sma.GetPower},
			update{build.ForkMissingSnowballs, sma.GetPower2},
		), // FORK
		12: sma.GetPeerID,
		13: sma.GetSectorSize,
		14: sma.UpdatePeerID,
		//15: sma.ChangeWorker,
		16: sma.IsSlashed,
		17: sma.CheckMiner,
		18: sma.DeclareFaults,
		19: sma.SlashConsensusFault,
		20: sma.SubmitElectionPoSt,
	}
}

func (sma StorageMinerActor2) StorageMinerConstructor(act *types.Actor, vmctx types.VMContext, params *StorageMinerConstructorParams) ([]byte, ActorError) {
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
	sectors := amt2.NewAMT(types.WrapStorage(vmctx.Storage()))
	scid, serr := sectors.Flush()
	if serr != nil {
		return nil, aerrors.HandleExternalError(serr, "initializing AMT")
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

func (sma StorageMinerActor2) PreCommitSector(act *types.Actor, vmctx types.VMContext, params *SectorPreCommitInfo) ([]byte, ActorError) {

	ctx := vmctx.Context()
	oldstate, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	if params.SealEpoch >= vmctx.BlockHeight()+build.SealRandomnessLookback {
		return nil, aerrors.Newf(1, "sector commitment must be based off past randomness (%d >= %d)", params.SealEpoch, vmctx.BlockHeight()+build.SealRandomnessLookback)
	}

	if vmctx.BlockHeight()-params.SealEpoch+build.SealRandomnessLookback > build.SealRandomnessLookbackLimit {
		return nil, aerrors.Newf(2, "sector commitment must be recent enough (was %d)", vmctx.BlockHeight()-params.SealEpoch+build.SealRandomnessLookback)
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	if vmctx.Message().From != mi.Worker {
		return nil, aerrors.New(1, "not authorized to precommit sector for miner")
	}

	// make sure the miner isnt trying to submit a pre-existing sector
	unique, err := SectorIsUnique(ctx, vmctx.Storage(), self.Sectors, params.SectorNumber)
	if err != nil {
		return nil, err
	}
	if !unique {
		return nil, aerrors.New(3, "sector already committed!")
	}

	// Power of the miner after adding this sector
	futurePower := types.BigAdd(self.Power, types.NewInt(mi.SectorSize))
	collateralRequired := CollateralForPower(futurePower)

	// TODO: grab from market?
	if act.Balance.LessThan(collateralRequired) {
		return nil, aerrors.New(4, "not enough collateral")
	}

	self.PreCommittedSectors[uintToStringKey(params.SectorNumber)] = &PreCommittedSector{
		Info:          *params,
		ReceivedEpoch: vmctx.BlockHeight(),
	}

	if len(self.PreCommittedSectors) > 4096 {
		return nil, aerrors.New(5, "too many precommitted sectors")
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

func (sma StorageMinerActor2) ProveCommitSector(act *types.Actor, vmctx types.VMContext, params *SectorProveCommitInfo) ([]byte, ActorError) {
	ctx := vmctx.Context()
	oldstate, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	if vmctx.Message().From != mi.Worker {
		return nil, aerrors.New(1, "not authorized to submit sector proof for miner")
	}

	us, ok := self.PreCommittedSectors[uintToStringKey(params.SectorID)]
	if !ok {
		return nil, aerrors.New(1, "no pre-commitment found for sector")
	}

	if us.ReceivedEpoch+build.InteractivePoRepDelay >= vmctx.BlockHeight() {
		return nil, aerrors.New(2, "too early for proof submission")
	}

	delete(self.PreCommittedSectors, uintToStringKey(params.SectorID))

	// TODO: ensure normalization to ID address
	maddr := vmctx.Message().To

	if vmctx.BlockHeight()-us.Info.SealEpoch > build.MaxSealLookback {
		return nil, aerrors.Newf(5, "source randomness for sector SealEpoch too far in past (epoch %d)", us.Info.SealEpoch)
	}

	if vmctx.BlockHeight()-us.ReceivedEpoch > build.MaxSealLookback {
		return nil, aerrors.Newf(6, "source randomness for sector ReceivedEpoch too far in past (epoch %d)", us.ReceivedEpoch)
	}

	ticket, err := vmctx.GetRandomness(us.Info.SealEpoch - build.SealRandomnessLookback)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to get ticket randomness")
	}

	seed, err := vmctx.GetRandomness(us.ReceivedEpoch + build.InteractivePoRepDelay)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to get randomness for prove sector commitment")
	}

	enc, err := SerializeParams(&ComputeDataCommitmentParams{
		DealIDs:    params.DealIDs,
		SectorSize: mi.SectorSize,
	})
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to serialize ComputeDataCommitmentParams")
	}

	commD, err := vmctx.Send(StorageMarketAddress, SMAMethods.ComputeDataCommitment, types.NewInt(0), enc)
	if err != nil {
		return nil, aerrors.Wrapf(err, "failed to compute data commitment (sector %d, deals: %v)", params.SectorID, params.DealIDs)
	}

	if ok, err := vmctx.Sys().ValidatePoRep(ctx, maddr, mi.SectorSize, commD, us.Info.CommR, ticket, params.Proof, seed, params.SectorID); err != nil {
		return nil, err
	} else if !ok {
		return nil, aerrors.Newf(2, "porep proof was invalid (t:%x; s:%x(%d); p:%s)", ticket, seed, us.ReceivedEpoch+build.InteractivePoRepDelay, truncateHexPrint(params.Proof))
	}

	// Note: There must exist a unique index in the miner's sector set for each
	// sector ID. The `faults`, `recovered`, and `done` parameters of the
	// SubmitPoSt method express indices into this sector set.
	nssroot, err := AddToSectorSet2(ctx, types.WrapStorage(vmctx.Storage()), self.Sectors, params.SectorID, us.Info.CommR, commD)
	if err != nil {
		return nil, err
	}
	self.Sectors = nssroot

	// if miner is not mining, start their proving period now
	// Note: As written here, every miners first PoSt will only be over one sector.
	// We could set up a 'grace period' for starting mining that would allow miners
	// to submit several sectors for their first proving period. Alternatively, we
	// could simply make the 'PreCommitSector' call take multiple sectors at a time.
	//
	// Note: Proving period is a function of sector size; small sectors take less
	// time to prove than large sectors do. Sector size is selected when pledging.
	pss, lerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingSet)
	if lerr != nil {
		return nil, aerrors.HandleExternalError(lerr, "could not load proving set node")
	}

	if pss.Count == 0 && !self.Active {
		self.ProvingSet = self.Sectors
		// TODO: probably want to wait until the miner is above a certain
		//  threshold before starting this
		self.ElectionPeriodStart = vmctx.BlockHeight()
	}

	nstate, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}
	if err := vmctx.Storage().Commit(oldstate, nstate); err != nil {
		return nil, err
	}

	activateParams, err := SerializeParams(&ActivateStorageDealsParams{
		Deals: params.DealIDs,
	})
	if err != nil {
		return nil, err
	}

	_, err = vmctx.Send(StorageMarketAddress, SMAMethods.ActivateStorageDeals, types.NewInt(0), activateParams)
	return nil, aerrors.Wrapf(err, "calling ActivateStorageDeals failed")
}

func (sma StorageMinerActor2) SubmitFallbackPoSt(act *types.Actor, vmctx types.VMContext, params *SubmitFallbackPoStParams) ([]byte, ActorError) {
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

	/*
		// TODO: handle fees
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
	*/

	var seed [sectorbuilder.CommLen]byte
	{
		randHeight := self.ElectionPeriodStart + build.FallbackPoStDelay
		if vmctx.BlockHeight() <= randHeight {
			// TODO: spec, retcode
			return nil, aerrors.Newf(1, "submit fallback PoSt called too early (%d < %d)", vmctx.BlockHeight(), randHeight)
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

	pss, lerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingSet)
	if lerr != nil {
		return nil, aerrors.HandleExternalError(lerr, "could not load proving set node")
	}

	ss, lerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Sectors)
	if lerr != nil {
		return nil, aerrors.HandleExternalError(lerr, "could not load proving set node")
	}

	faults, nerr := self.FaultSet.AllMap(2 * ss.Count)
	if nerr != nil {
		return nil, aerrors.Absorb(err, 5, "RLE+ invalid")
	}

	activeFaults := uint64(0)
	var sectorInfos []types.PublicSectorInfo
	if err := pss.ForEach(func(id uint64, v *cbg.Deferred) error {
		if faults[id] {
			activeFaults++
			return nil
		}

		var comms [][]byte
		if err := cbor.DecodeInto(v.Raw, &comms); err != nil {
			return xerrors.New("could not decode comms")
		}
		si := types.PublicSectorInfo{
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

	proverID := vmctx.Message().To // TODO: normalize to ID address

	var candidates []types.Candidate
	for _, t := range params.Candidates {
		var partial [32]byte
		copy(partial[:], t.Partial)
		candidates = append(candidates, types.Candidate{
			PartialTicket:        partial,
			SectorID:             t.SectorID,
			SectorChallengeIndex: t.ChallengeIndex,
		})
	}

	if ok, lerr := vmctx.Sys().VerifyFallbackPost(vmctx.Context(), mi.SectorSize,
		sectorInfos, seed[:], params.Proof, candidates, proverID, activeFaults); !ok || lerr != nil {
		if lerr != nil {
			// TODO: study PoST errors
			return nil, aerrors.Absorb(lerr, 4, "PoST error")
		}
		if !ok {
			return nil, aerrors.New(4, "PoST invalid")
		}
	}

	// Post submission is successful!
	if err := onSuccessfulPoSt2(self, vmctx, activeFaults); err != nil {
		return nil, err
	}

	c, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(oldstate, c); err != nil {
		return nil, err
	}

	return nil, nil
}

func (sma StorageMinerActor2) GetPower(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	return self.Power.Bytes(), nil
}

func (sma StorageMinerActor2) GetPower2(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	if self.SlashedAt != 0 {
		return types.NewInt(0).Bytes(), nil
	}

	return self.Power.Bytes(), nil
}

func SectorIsUnique2(ctx context.Context, s types.Storage, sroot cid.Cid, sid uint64) (bool, ActorError) {
	found, _, _, err := GetFromSectorSet2(ctx, s, sroot, sid)
	if err != nil {
		return false, err
	}

	return !found, nil
}

func AddToSectorSet2(ctx context.Context, blks amt2.Blocks, ss cid.Cid, sectorID uint64, commR, commD []byte) (cid.Cid, ActorError) {
	if sectorID >= build.MinerMaxSectors {
		return cid.Undef, aerrors.Newf(25, "sector ID out of range: %d", sectorID)
	}
	ssr, err := amt2.LoadAMT(blks, ss)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "could not load sector set node")
	}

	// TODO: Spec says to use SealCommitment, and construct commD from deals each time,
	//  but that would make SubmitPoSt way, way more expensive
	if err := ssr.Set(sectorID, [][]byte{commR, commD}); err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to set commitment in sector set")
	}

	ncid, err := ssr.Flush()
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to flush sector set")
	}

	return ncid, nil
}

func GetFromSectorSet2(ctx context.Context, s types.Storage, ss cid.Cid, sectorID uint64) (bool, []byte, []byte, ActorError) {
	if sectorID >= build.MinerMaxSectors {
		return false, nil, nil, aerrors.Newf(25, "sector ID out of range: %d", sectorID)
	}

	ssr, err := amt2.LoadAMT(types.WrapStorage(s), ss)
	if err != nil {
		return false, nil, nil, aerrors.HandleExternalError(err, "could not load sector set node")
	}

	var comms [][]byte
	err = ssr.Get(sectorID, &comms)
	if err != nil {
		if _, ok := err.(*amt2.ErrNotFound); ok {
			return false, nil, nil, nil
		}
		return false, nil, nil, aerrors.HandleExternalError(err, "failed to find sector in sector set")
	}

	if len(comms) != 2 {
		return false, nil, nil, aerrors.Newf(20, "sector set entry should only have 2 elements")
	}

	return true, comms[0], comms[1], nil
}

func RemoveFromSectorSet2(ctx context.Context, s types.Storage, ss cid.Cid, ids []uint64) (cid.Cid, aerrors.ActorError) {

	ssr, err := amt2.LoadAMT(types.WrapStorage(s), ss)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "could not load sector set node")
	}

	for _, id := range ids {
		if err := ssr.Delete(id); err != nil {
			log.Warnf("failed to delete sector %d from set: %s", id, err)
		}
	}

	ncid, err := ssr.Flush()
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to flush sector set")
	}

	return ncid, nil
}

func (sma StorageMinerActor2) GetWorkerAddr(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
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

func (sma StorageMinerActor2) GetOwner(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
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

func (sma StorageMinerActor2) GetPeerID(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
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

func (sma StorageMinerActor2) UpdatePeerID(act *types.Actor, vmctx types.VMContext, params *UpdatePeerIDParams) ([]byte, ActorError) {
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

func (sma StorageMinerActor2) GetSectorSize(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	mi, err := loadMinerInfo(vmctx, self)
	if err != nil {
		return nil, err
	}

	return types.NewInt(mi.SectorSize).Bytes(), nil
}

func (sma StorageMinerActor2) IsSlashed(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	_, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	return cbg.EncodeBool(self.SlashedAt != 0), nil
}

// TODO: better name
func (sma StorageMinerActor2) CheckMiner(act *types.Actor, vmctx types.VMContext, params *CheckMinerParams) ([]byte, ActorError) {
	if vmctx.Message().From != StoragePowerAddress {
		return nil, aerrors.New(2, "only the storage power actor can check miner")
	}

	oldstate, self, err := loadState(vmctx)
	if err != nil {
		return nil, err
	}

	if !isLate(vmctx.BlockHeight(), self) {
		// Everything's fine
		return nil, nil
	}

	if self.SlashedAt != 0 {
		// Don't slash more than necessary
		return nil, nil
	}

	if params.NetworkPower.Equals(self.Power) {
		// Don't break the network when there's only one miner left

		log.Warnf("can't slash miner %s for missed PoSt, no power would be left in the network", vmctx.Message().To)
		return nil, nil
	}

	// Slash for being late

	self.SlashedAt = vmctx.BlockHeight()
	oldPower := self.Power

	if vmctx.BlockHeight() > build.ForkMissingSnowballs {
		self.Power = types.NewInt(0)
	}

	nstate, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}
	if err := vmctx.Storage().Commit(oldstate, nstate); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := oldPower.MarshalCBOR(&out); err != nil {
		return nil, aerrors.HandleExternalError(err, "marshaling return value")
	}
	return out.Bytes(), nil
}

func (sma StorageMinerActor2) DeclareFaults(act *types.Actor, vmctx types.VMContext, params *DeclareFaultsParams) ([]byte, ActorError) {
	oldstate, self, aerr := loadState(vmctx)
	if aerr != nil {
		return nil, aerr
	}

	mi, aerr := loadMinerInfo(vmctx, self)
	if aerr != nil {
		return nil, aerr
	}

	if vmctx.Message().From != mi.Worker {
		return nil, aerrors.New(1, "not authorized to declare faults for miner")
	}

	nfaults, err := types.MergeBitFields(params.Faults, self.FaultSet)
	if err != nil {
		return nil, aerrors.Absorb(err, 1, "failed to merge bitfields")
	}

	ss, nerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Sectors)
	if nerr != nil {
		return nil, aerrors.HandleExternalError(nerr, "failed to load sector set")
	}

	cf, nerr := nfaults.Count()
	if nerr != nil {
		return nil, aerrors.Absorb(nerr, 2, "could not decode RLE+")
	}

	if cf > 2*ss.Count {
		return nil, aerrors.Newf(3, "too many declared faults: %d > %d", cf, 2*ss.Count)
	}

	self.FaultSet = nfaults

	self.LastFaultSubmission = vmctx.BlockHeight()

	nstate, aerr := vmctx.Storage().Put(self)
	if aerr != nil {
		return nil, aerr
	}
	if err := vmctx.Storage().Commit(oldstate, nstate); err != nil {
		return nil, err
	}

	return nil, nil
}

func (sma StorageMinerActor2) SlashConsensusFault(act *types.Actor, vmctx types.VMContext, params *MinerSlashConsensusFault) ([]byte, ActorError) {
	if vmctx.Message().From != StoragePowerAddress {
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

	if vmctx.BlockHeight() > build.ForkMissingSnowballs {
		oldstate, self, err := loadState(vmctx)
		if err != nil {
			return nil, aerrors.Wrap(err, "failed to load state for slashing")
		}

		self.Power = types.NewInt(0)

		ncid, err := vmctx.Storage().Put(self)
		if err != nil {
			return nil, err
		}
		if err := vmctx.Storage().Commit(oldstate, ncid); err != nil {
			return nil, err
		}
	}

	// TODO: this still allows the miner to commit sectors and submit posts,
	// their users could potentially be unaffected, but the miner will never be
	// able to mine a block again
	// One potential issue: the miner will have to pay back the slashed
	// collateral to continue submitting PoSts, which includes pledge
	// collateral that they no longer really 'need'

	return nil, nil
}

func (sma StorageMinerActor2) SubmitElectionPoSt(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, aerrors.ActorError) {
	if vmctx.Message().From != NetworkAddress {
		return nil, aerrors.Newf(1, "submit election post can only be called by the storage power actor")
	}

	oldstate, self, aerr := loadState(vmctx)
	if aerr != nil {
		return nil, aerr
	}

	if self.SlashedAt != 0 {
		return nil, aerrors.New(1, "slashed miners can't perform election PoSt")
	}

	pss, nerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingSet)
	if nerr != nil {
		return nil, aerrors.HandleExternalError(nerr, "failed to load proving set")
	}

	ss, nerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Sectors)
	if nerr != nil {
		return nil, aerrors.HandleExternalError(nerr, "failed to load proving set")
	}

	faults, nerr := self.FaultSet.AllMap(2 * ss.Count)
	if nerr != nil {
		return nil, aerrors.Absorb(nerr, 1, "invalid bitfield (fatal?)")
	}

	activeFaults := uint64(0)
	for f := range faults {
		if f > amt2.MaxIndex {
			continue
		}

		var comms [][]byte
		err := pss.Get(f, &comms)
		if err != nil {
			var notfound *amt2.ErrNotFound
			if !xerrors.As(err, &notfound) {
				return nil, aerrors.HandleExternalError(err, "failed to find sector in sector set")
			}
			continue
		}

		activeFaults++
	}

	if err := onSuccessfulPoSt2(self, vmctx, activeFaults); err != nil { // TODO
		return nil, err
	}

	ncid, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, err
	}
	if err := vmctx.Storage().Commit(oldstate, ncid); err != nil {
		return nil, err
	}

	return nil, nil
}

func onSuccessfulPoSt2(self *StorageMinerActorState, vmctx types.VMContext, activeFaults uint64) aerrors.ActorError {
	// FORK
	if vmctx.BlockHeight() < build.ForkBootyBayHeight {
		return onSuccessfulPoSt(self, vmctx, activeFaults)
	}

	var mi MinerInfo
	if err := vmctx.Storage().Get(self.Info, &mi); err != nil {
		return err
	}

	pss, nerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingSet)
	if nerr != nil {
		return aerrors.HandleExternalError(nerr, "failed to load proving set")
	}

	ss, nerr := amt2.LoadAMT(types.WrapStorage(vmctx.Storage()), self.Sectors)
	if nerr != nil {
		return aerrors.HandleExternalError(nerr, "failed to load sector set")
	}

	faults, nerr := self.FaultSet.All(2 * ss.Count)
	if nerr != nil {
		return aerrors.Absorb(nerr, 1, "invalid bitfield (fatal?)")
	}

	self.FaultSet = types.NewBitField()

	oldPower := self.Power
	newPower := types.BigMul(types.NewInt(pss.Count-activeFaults), types.NewInt(mi.SectorSize))

	// If below the minimum size requirement, miners have zero power
	if newPower.LessThan(types.NewInt(build.MinimumMinerPower)) {
		newPower = types.NewInt(0)
	}

	self.Power = newPower

	delta := types.BigSub(self.Power, oldPower)
	if self.SlashedAt != 0 {
		self.SlashedAt = 0
		delta = self.Power
	}

	prevSlashingDeadline := self.ElectionPeriodStart + build.SlashablePowerDelay
	if !self.Active && newPower.GreaterThan(types.NewInt(0)) {
		self.Active = true
		prevSlashingDeadline = 0
	}

	if !(oldPower.IsZero() && newPower.IsZero()) {
		enc, err := SerializeParams(&UpdateStorageParams{
			Delta:                 delta,
			NextSlashDeadline:     vmctx.BlockHeight() + build.SlashablePowerDelay,
			PreviousSlashDeadline: prevSlashingDeadline,
		})
		if err != nil {
			return err
		}

		_, err = vmctx.Send(StoragePowerAddress, SPAMethods.UpdateStorage, types.NewInt(0), enc)
		if err != nil {
			return aerrors.Wrap(err, "updating storage failed")
		}

		self.ElectionPeriodStart = vmctx.BlockHeight()
	}

	var ncid cid.Cid
	var err aerrors.ActorError

	ncid, err = RemoveFromSectorSet2(vmctx.Context(), vmctx.Storage(), self.Sectors, faults)
	if err != nil {
		return err
	}

	self.Sectors = ncid
	self.ProvingSet = ncid
	return nil
}
