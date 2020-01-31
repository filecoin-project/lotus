package actors

import (
	"bytes"
	"context"
	"io"

	"github.com/filecoin-project/go-amt-ipld"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type StoragePowerActor struct{}

type spaMethods struct {
	Constructor             uint64
	CreateStorageMiner      uint64
	ArbitrateConsensusFault uint64
	UpdateStorage           uint64
	GetTotalStorage         uint64
	PowerLookup             uint64
	IsValidMiner            uint64
	PledgeCollateralForSize uint64
	CheckProofSubmissions   uint64
}

var SPAMethods = spaMethods{1, 2, 3, 4, 5, 6, 7, 8, 9}

func (spa StoragePowerActor) Exports() []interface{} {
	return []interface{}{
		//1: spa.StoragePowerConstructor,
		2: spa.CreateStorageMiner,
		3: spa.ArbitrateConsensusFault,
		4: spa.UpdateStorage,
		5: spa.GetTotalStorage,
		6: spa.PowerLookup,
		7: spa.IsValidMiner,
		8: spa.PledgeCollateralForSize,
		9: spa.CheckProofSubmissions,
	}
}

type StoragePowerState struct {
	Miners         cid.Cid
	ProvingBuckets cid.Cid // amt[ProvingPeriodBucket]hamt[minerAddress]struct{}
	MinerCount     uint64
	LastMinerCheck uint64

	TotalStorage types.BigInt
}

type CreateStorageMinerParams struct {
	Owner      address.Address
	Worker     address.Address
	SectorSize uint64
	PeerID     peer.ID
}

func (spa StoragePowerActor) CreateStorageMiner(act *types.Actor, vmctx types.VMContext, params *CreateStorageMinerParams) ([]byte, ActorError) {
	if !build.SupportedSectorSize(params.SectorSize) {
		return nil, aerrors.Newf(1, "Unsupported sector size: %d", params.SectorSize)
	}

	var self StoragePowerState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	reqColl, err := pledgeCollateralForSize(vmctx, types.NewInt(0), self.TotalStorage, self.MinerCount+1)
	if err != nil {
		return nil, err
	}

	if vmctx.Message().Value.LessThan(reqColl) {
		return nil, aerrors.Newf(1, "not enough funds passed to cover required miner collateral (needed %s, got %s)", reqColl, vmctx.Message().Value)
	}

	// FORK
	minerCid := StorageMinerCodeCid
	if vmctx.BlockHeight() > build.ForkFrigidHeight {
		minerCid = StorageMiner2CodeCid
	}

	encoded, err := CreateExecParams(minerCid, &StorageMinerConstructorParams{
		Owner:      params.Owner,
		Worker:     params.Worker,
		SectorSize: params.SectorSize,
		PeerID:     params.PeerID,
	})
	if err != nil {
		return nil, err
	}

	ret, err := vmctx.Send(InitAddress, IAMethods.Exec, vmctx.Message().Value, encoded)
	if err != nil {
		return nil, err
	}

	naddr, nerr := address.NewFromBytes(ret)
	if nerr != nil {
		return nil, aerrors.Absorb(nerr, 2, "could not read address of new actor")
	}

	ncid, err := MinerSetAdd(context.TODO(), vmctx, self.Miners, naddr)
	if err != nil {
		return nil, err
	}
	self.Miners = ncid
	self.MinerCount++

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return nil, err
	}

	return naddr.Bytes(), nil
}

type ArbitrateConsensusFaultParams struct {
	Block1 *types.BlockHeader
	Block2 *types.BlockHeader
}

func (spa StoragePowerActor) ArbitrateConsensusFault(act *types.Actor, vmctx types.VMContext, params *ArbitrateConsensusFaultParams) ([]byte, ActorError) {
	if params == nil || params.Block1 == nil || params.Block2 == nil {
		return nil, aerrors.New(1, "failed to parse params")
	}

	if params.Block1.Miner != params.Block2.Miner {
		return nil, aerrors.New(2, "blocks must be from the same miner")
	}

	// FORK
	if vmctx.BlockHeight() > build.ForkBlizzardHeight {
		if params.Block1.Height <= build.ForkBlizzardHeight {
			return nil, aerrors.New(10, "cannot slash miners with blocks from before blizzard")
		}

		if params.Block2.Height <= build.ForkBlizzardHeight {
			return nil, aerrors.New(11, "cannot slash miners with blocks from before blizzard")
		}

		if params.Block1.Cid() == params.Block2.Cid() {
			return nil, aerrors.New(3, "blocks must be different")
		}
	}

	rval, err := vmctx.Send(params.Block1.Miner, MAMethods.GetWorkerAddr, types.NewInt(0), nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to get miner worker")
	}

	worker, oerr := address.NewFromBytes(rval)
	if oerr != nil {
		// REVIEW: should this be fatal? i can't think of a real situation that would get us here
		return nil, aerrors.Absorb(oerr, 3, "response from 'GetWorkerAddr' was not a valid address")
	}

	if err := params.Block1.CheckBlockSignature(vmctx.Context(), worker); err != nil {
		return nil, aerrors.Absorb(err, 4, "block1 did not have valid signature")
	}

	if err := params.Block2.CheckBlockSignature(vmctx.Context(), worker); err != nil {
		return nil, aerrors.Absorb(err, 5, "block2 did not have valid signature")
	}

	// see the "Consensus Faults" section of the faults spec (faults.md)
	// for details on these slashing conditions.
	if !shouldSlash(params.Block1, params.Block2) {
		return nil, aerrors.New(6, "blocks do not prove a slashable offense")
	}

	var self StoragePowerState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	if types.BigCmp(self.TotalStorage, types.NewInt(0)) == 0 {
		return nil, aerrors.Fatal("invalid state, storage power actor has zero total storage")
	}

	miner := params.Block1.Miner
	if has, err := MinerSetHas(vmctx, self.Miners, miner); err != nil {
		return nil, aerrors.Wrapf(err, "failed to check miner in set")
	} else if !has {
		return nil, aerrors.New(7, "either already slashed or not a miner")
	}

	minerPower, err := powerLookup(context.TODO(), vmctx, &self, miner)
	if err != nil {
		return nil, err
	}

	slashedCollateral, err := pledgeCollateralForSize(vmctx, minerPower, self.TotalStorage, self.MinerCount)
	if err != nil {
		return nil, err
	}

	enc, err := SerializeParams(&MinerSlashConsensusFault{
		Slasher:           vmctx.Message().From,
		AtHeight:          params.Block1.Height,
		SlashedCollateral: slashedCollateral,
	})
	if err != nil {
		return nil, err
	}

	_, err = vmctx.Send(miner, MAMethods.SlashConsensusFault, types.NewInt(0), enc)
	if err != nil {
		return nil, err
	}

	// Remove the miner from the list of network miners
	ncid, err := MinerSetRemove(context.TODO(), vmctx, self.Miners, miner)
	if err != nil {
		return nil, err
	}
	self.Miners = ncid
	self.MinerCount--

	self.TotalStorage = types.BigSub(self.TotalStorage, minerPower)

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return nil, err
	}

	return nil, nil
}

func cidArrContains(a []cid.Cid, b cid.Cid) bool {
	for _, c := range a {
		if b == c {
			return true
		}
	}

	return false
}

func shouldSlash(block1, block2 *types.BlockHeader) bool {
	// First slashing condition, blocks have the same ticket round
	if block1.Height == block2.Height {
		return true
	}

	/* Second slashing condition requires having access to the parent tipset blocks
	// This might not always be available, needs some thought on the best way to deal with this


	// Second slashing condition, miner ignored own block when mining
	// Case A: block2 could have been in block1's parent set but is not
	b1ParentHeight := block1.Height - len(block1.Tickets)

	block1ParentTipSet := block1.Parents
	if !cidArrContains(block1.Parents, block2.Cid()) &&
		b1ParentHeight == block2.Height &&
		block1ParentTipSet.ParentCids == block2.ParentCids {
		return true
	}

	// Case B: block1 could have been in block2's parent set but is not
	block2ParentTipSet := parentOf(block2)
	if !block2Parent.contains(block1) &&
		block2ParentTipSet.Height == block1.Height &&
		block2ParentTipSet.ParentCids == block1.ParentCids {
		return true
	}

	*/

	return false
}

type UpdateStorageParams struct {
	Delta                 types.BigInt
	NextSlashDeadline     uint64
	PreviousSlashDeadline uint64
}

func (spa StoragePowerActor) UpdateStorage(act *types.Actor, vmctx types.VMContext, params *UpdateStorageParams) ([]byte, ActorError) {
	var self StoragePowerState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	has, err := MinerSetHas(vmctx, self.Miners, vmctx.Message().From)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, aerrors.New(1, "update storage must only be called by a miner actor")
	}

	self.TotalStorage = types.BigAdd(self.TotalStorage, params.Delta)

	previousBucket := params.PreviousSlashDeadline % build.SlashablePowerDelay
	nextBucket := params.NextSlashDeadline % build.SlashablePowerDelay

	if previousBucket == nextBucket && params.PreviousSlashDeadline != 0 {
		nroot, err := vmctx.Storage().Put(&self)
		if err != nil {
			return nil, err
		}

		if err := vmctx.Storage().Commit(old, nroot); err != nil {
			return nil, err
		}

		return nil, nil // Nothing to do
	}

	buckets, eerr := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingBuckets)
	if eerr != nil {
		return nil, aerrors.HandleExternalError(eerr, "loading proving buckets amt")
	}

	if params.PreviousSlashDeadline != 0 { // delete from previous bucket
		err := deleteMinerFromBucket(vmctx, buckets, previousBucket)
		if err != nil {
			return nil, aerrors.Wrapf(err, "delete from bucket %d, next %d", previousBucket, nextBucket)
		}
	}

	err = addMinerToBucket(vmctx, buckets, nextBucket)
	if err != nil {
		return nil, err
	}

	self.ProvingBuckets, eerr = buckets.Flush()
	if eerr != nil {
		return nil, aerrors.HandleExternalError(eerr, "flushing proving buckets")
	}

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return nil, err
	}

	return nil, nil
}

func deleteMinerFromBucket(vmctx types.VMContext, buckets *amt.Root, previousBucket uint64) aerrors.ActorError {
	var bucket cid.Cid
	err := buckets.Get(previousBucket, &bucket)
	switch err.(type) {
	case *amt.ErrNotFound:
		return aerrors.HandleExternalError(err, "proving bucket missing")
	case nil: // noop
	default:
		return aerrors.HandleExternalError(err, "getting proving bucket")
	}

	bhamt, err := hamt.LoadNode(vmctx.Context(), vmctx.Ipld(), bucket)
	if err != nil {
		return aerrors.HandleExternalError(err, "failed to load proving bucket")
	}
	err = bhamt.Delete(vmctx.Context(), string(vmctx.Message().From.Bytes()))
	if err != nil {
		return aerrors.HandleExternalError(err, "deleting miner from proving bucket")
	}

	err = bhamt.Flush(vmctx.Context())
	if err != nil {
		return aerrors.HandleExternalError(err, "flushing previous proving bucket")
	}

	bucket, err = vmctx.Ipld().Put(vmctx.Context(), bhamt)
	if err != nil {
		return aerrors.HandleExternalError(err, "putting previous proving bucket hamt")
	}

	err = buckets.Set(previousBucket, bucket)
	if err != nil {
		return aerrors.HandleExternalError(err, "setting previous proving bucket cid in amt")
	}

	return nil
}

func addMinerToBucket(vmctx types.VMContext, buckets *amt.Root, nextBucket uint64) aerrors.ActorError {
	var bhamt *hamt.Node
	var bucket cid.Cid
	err := buckets.Get(nextBucket, &bucket)
	switch err.(type) {
	case *amt.ErrNotFound:
		bhamt = hamt.NewNode(vmctx.Ipld())
	case nil:
		bhamt, err = hamt.LoadNode(vmctx.Context(), vmctx.Ipld(), bucket)
		if err != nil {
			return aerrors.HandleExternalError(err, "failed to load proving bucket")
		}
	default:
		return aerrors.HandleExternalError(err, "getting proving bucket")
	}

	err = bhamt.Set(vmctx.Context(), string(vmctx.Message().From.Bytes()), CborNull)
	if err != nil {
		return aerrors.HandleExternalError(err, "setting miner in proving bucket")
	}

	err = bhamt.Flush(vmctx.Context())
	if err != nil {
		return aerrors.HandleExternalError(err, "flushing previous proving bucket")
	}

	bucket, err = vmctx.Ipld().Put(vmctx.Context(), bhamt)
	if err != nil {
		return aerrors.HandleExternalError(err, "putting previous proving bucket hamt")
	}

	err = buckets.Set(nextBucket, bucket)
	if err != nil {
		return aerrors.HandleExternalError(err, "setting previous proving bucket cid in amt")
	}
	return nil
}

func (spa StoragePowerActor) GetTotalStorage(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	var self StoragePowerState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, err
	}

	return self.TotalStorage.Bytes(), nil
}

type PowerLookupParams struct {
	Miner address.Address
}

func (spa StoragePowerActor) PowerLookup(act *types.Actor, vmctx types.VMContext, params *PowerLookupParams) ([]byte, ActorError) {
	var self StoragePowerState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, aerrors.Wrap(err, "getting head")
	}

	pow, err := powerLookup(context.TODO(), vmctx, &self, params.Miner)
	if err != nil {
		return nil, err
	}

	return pow.Bytes(), nil
}

func powerLookup(ctx context.Context, vmctx types.VMContext, self *StoragePowerState, miner address.Address) (types.BigInt, ActorError) {
	has, err := MinerSetHas(vmctx, self.Miners, miner)
	if err != nil {
		return types.EmptyInt, err
	}

	if !has {
		return types.EmptyInt, aerrors.New(1, "miner not registered with storage power actor")
	}

	// TODO: Use local amt
	ret, err := vmctx.Send(miner, MAMethods.GetPower, types.NewInt(0), nil)
	if err != nil {
		return types.EmptyInt, aerrors.Wrap(err, "invoke Miner.GetPower")
	}

	return types.BigFromBytes(ret), nil
}

type IsValidMinerParam struct {
	Addr address.Address
}

func (spa StoragePowerActor) IsValidMiner(act *types.Actor, vmctx types.VMContext, param *IsValidMinerParam) ([]byte, ActorError) {
	var self StoragePowerState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, err
	}

	has, err := MinerSetHas(vmctx, self.Miners, param.Addr)
	if err != nil {
		return nil, err
	}

	if !has {
		log.Warnf("Miner INVALID: not in set: %s", param.Addr)

		return cbg.CborBoolFalse, nil
	}

	ret, err := vmctx.Send(param.Addr, MAMethods.IsSlashed, types.NewInt(0), nil)
	if err != nil {
		return nil, err
	}

	slashed := bytes.Equal(ret, cbg.CborBoolTrue)

	if slashed {
		log.Warnf("Miner INVALID: /SLASHED/ : %s", param.Addr)
	}

	return cbg.EncodeBool(!slashed), nil
}

type PledgeCollateralParams struct {
	Size types.BigInt
}

func (spa StoragePowerActor) PledgeCollateralForSize(act *types.Actor, vmctx types.VMContext, param *PledgeCollateralParams) ([]byte, ActorError) {
	var self StoragePowerState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, err
	}

	totalCollateral, err := pledgeCollateralForSize(vmctx, param.Size, self.TotalStorage, self.MinerCount)
	if err != nil {
		return nil, err
	}

	return totalCollateral.Bytes(), nil
}

func pledgeCollateralForSize(vmctx types.VMContext, size, totalStorage types.BigInt, minerCount uint64) (types.BigInt, aerrors.ActorError) {
	netBalance, err := vmctx.GetBalance(NetworkAddress)
	if err != nil {
		return types.EmptyInt, err
	}

	// TODO: the spec says to also grab 'total vested filecoin' and include it as available
	// If we don't factor that in, we effectively assume all of the locked up filecoin is 'available'
	// the blocker on that right now is that its hard to tell how much filecoin is unlocked

	availableFilecoin := types.BigSub(
		types.BigMul(types.NewInt(build.TotalFilecoin), types.NewInt(build.FilecoinPrecision)),
		netBalance,
	)

	totalPowerCollateral := types.BigDiv(
		types.BigMul(
			availableFilecoin,
			types.NewInt(build.PowerCollateralProportion),
		),
		types.NewInt(build.CollateralPrecision),
	)

	totalPerCapitaCollateral := types.BigDiv(
		types.BigMul(
			availableFilecoin,
			types.NewInt(build.PerCapitaCollateralProportion),
		),
		types.NewInt(build.CollateralPrecision),
	)

	// REVIEW: for bootstrapping purposes, we skip the power portion of the
	// collateral if there is no collateral in the network yet
	powerCollateral := types.NewInt(0)
	if types.BigCmp(totalStorage, types.NewInt(0)) != 0 {
		powerCollateral = types.BigDiv(
			types.BigMul(
				totalPowerCollateral,
				size,
			),
			totalStorage,
		)
	}

	perCapCollateral := types.BigDiv(
		totalPerCapitaCollateral,
		types.NewInt(minerCount),
	)

	return types.BigAdd(powerCollateral, perCapCollateral), nil
}

func (spa StoragePowerActor) CheckProofSubmissions(act *types.Actor, vmctx types.VMContext, param *struct{}) ([]byte, ActorError) {
	if vmctx.Message().From != CronAddress {
		return nil, aerrors.New(1, "CheckProofSubmissions is only callable from the cron actor")
	}

	var self StoragePowerState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	for i := self.LastMinerCheck; i < vmctx.BlockHeight(); i++ {
		height := i + 1

		err := checkProofSubmissionsAtH(vmctx, &self, height)
		if err != nil {
			return nil, err
		}
	}

	self.LastMinerCheck = vmctx.BlockHeight()

	nroot, aerr := vmctx.Storage().Put(&self)
	if aerr != nil {
		return nil, aerr
	}

	if err := vmctx.Storage().Commit(old, nroot); err != nil {
		return nil, err
	}

	return nil, nil
}

func checkProofSubmissionsAtH(vmctx types.VMContext, self *StoragePowerState, height uint64) aerrors.ActorError {
	bucketID := height % build.SlashablePowerDelay

	buckets, eerr := amt.LoadAMT(types.WrapStorage(vmctx.Storage()), self.ProvingBuckets)
	if eerr != nil {
		return aerrors.HandleExternalError(eerr, "loading proving buckets amt")
	}

	var bucket cid.Cid
	err := buckets.Get(bucketID, &bucket)
	switch err.(type) {
	case *amt.ErrNotFound:
		return nil // nothing to do
	case nil:
	default:
		return aerrors.HandleExternalError(err, "getting proving bucket")
	}

	bhamt, err := hamt.LoadNode(vmctx.Context(), vmctx.Ipld(), bucket)
	if err != nil {
		return aerrors.HandleExternalError(err, "failed to load proving bucket")
	}

	forRemoval := make([]address.Address, 0)

	err = bhamt.ForEach(vmctx.Context(), func(k string, val interface{}) error {
		_, span := trace.StartSpan(vmctx.Context(), "StoragePowerActor.CheckProofSubmissions.loop")
		defer span.End()

		maddr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return aerrors.Escalate(err, "parsing miner address")
		}

		if vmctx.BlockHeight() > build.ForkMissingSnowballs {
			has, aerr := MinerSetHas(vmctx, self.Miners, maddr)
			if aerr != nil {
				return aerr
			}

			if !has {
				forRemoval = append(forRemoval, maddr)
			}

		}

		span.AddAttributes(trace.StringAttribute("miner", maddr.String()))

		params, err := SerializeParams(&CheckMinerParams{NetworkPower: self.TotalStorage})
		if err != nil {
			return err
		}

		ret, err := vmctx.Send(maddr, MAMethods.CheckMiner, types.NewInt(0), params)
		if err != nil {
			return err
		}

		if len(ret) == 0 {
			return nil // miner is fine
		}

		var power types.BigInt
		if err := power.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
			return xerrors.Errorf("unmarshaling CheckMiner response (%x): %w", ret, err)
		}

		if power.GreaterThan(types.NewInt(0)) {
			log.Warnf("slashing miner %s for missed PoSt (%s B, H: %d, Bucket: %d)", maddr, power, height, bucketID)

			self.TotalStorage = types.BigSub(self.TotalStorage, power)
		}
		return nil
	})

	if err != nil {
		return aerrors.HandleExternalError(err, "iterating miners in proving bucket")
	}

	if vmctx.BlockHeight() > build.ForkMissingSnowballs && len(forRemoval) > 0 {
		nBucket, err := MinerSetRemove(vmctx.Context(), vmctx, bucket, forRemoval...)

		if err != nil {
			return aerrors.Wrap(err, "could not remove miners from set")
		}

		eerr := buckets.Set(bucketID, nBucket)
		if err != nil {
			return aerrors.HandleExternalError(eerr, "could not set the bucket")
		}
		ncid, eerr := buckets.Flush()
		if err != nil {
			return aerrors.HandleExternalError(eerr, "could not flush buckets")
		}
		self.ProvingBuckets = ncid
	}

	return nil
}

func MinerSetHas(vmctx types.VMContext, rcid cid.Cid, maddr address.Address) (bool, aerrors.ActorError) {
	nd, err := hamt.LoadNode(vmctx.Context(), vmctx.Ipld(), rcid)
	if err != nil {
		return false, aerrors.HandleExternalError(err, "failed to load miner set")
	}

	err = nd.Find(vmctx.Context(), string(maddr.Bytes()), nil)
	switch err {
	case hamt.ErrNotFound:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, aerrors.HandleExternalError(err, "failed to do set lookup")
	}
}

func MinerSetList(ctx context.Context, cst *hamt.CborIpldStore, rcid cid.Cid) ([]address.Address, error) {
	nd, err := hamt.LoadNode(ctx, cst, rcid)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner set: %w", err)
	}

	var out []address.Address
	err = nd.ForEach(ctx, func(k string, val interface{}) error {
		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		out = append(out, addr)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func MinerSetAdd(ctx context.Context, vmctx types.VMContext, rcid cid.Cid, maddr address.Address) (cid.Cid, aerrors.ActorError) {
	nd, err := hamt.LoadNode(ctx, vmctx.Ipld(), rcid)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to load miner set")
	}

	mkey := string(maddr.Bytes())
	err = nd.Find(ctx, mkey, nil)
	if err == nil {
		return cid.Undef, aerrors.New(20, "miner already in set")
	}

	if !xerrors.Is(err, hamt.ErrNotFound) {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to do miner set check")
	}

	if err := nd.Set(ctx, mkey, uint64(1)); err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "adding miner address to set failed")
	}

	if err := nd.Flush(ctx); err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to flush miner set")
	}

	c, err := vmctx.Ipld().Put(ctx, nd)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to persist miner set to storage")
	}

	return c, nil
}

func MinerSetRemove(ctx context.Context, vmctx types.VMContext, rcid cid.Cid, maddrs ...address.Address) (cid.Cid, aerrors.ActorError) {
	nd, err := hamt.LoadNode(ctx, vmctx.Ipld(), rcid)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to load miner set")
	}

	for _, maddr := range maddrs {
		mkey := string(maddr.Bytes())
		switch nd.Delete(ctx, mkey) {
		default:
			return cid.Undef, aerrors.HandleExternalError(err, "failed to delete miner from set")
		case hamt.ErrNotFound:
			return cid.Undef, aerrors.New(1, "miner not found in set on delete")
		case nil:
		}
	}

	if err := nd.Flush(ctx); err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to flush miner set")
	}

	c, err := vmctx.Ipld().Put(ctx, nd)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to persist miner set to storage")
	}

	return c, nil
}

type cbgNull struct{}

var CborNull = &cbgNull{}

func (cbgNull) MarshalCBOR(w io.Writer) error {
	n, err := w.Write(cbg.CborNull)
	if err != nil {
		return err
	}
	if n != 1 {
		return xerrors.New("expected to write 1 byte")
	}
	return nil
}

func (cbgNull) UnmarshalCBOR(r io.Reader) error {
	b := [1]byte{}
	n, err := r.Read(b[:])
	if err != nil {
		return err
	}
	if n != 1 {
		return xerrors.New("expected 1 byte")
	}
	if !bytes.Equal(b[:], cbg.CborNull) {
		return xerrors.New("expected cbor null")
	}
	return nil
}
