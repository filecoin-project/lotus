package actors

import (
	"context"
	"fmt"
	"math"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	xerrors "golang.org/x/xerrors"

	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type StorageMarketActor struct{}

type smaMethods struct {
	Constructor             uint64
	CreateStorageMiner      uint64
	SlashConsensusFault     uint64
	UpdateStorage           uint64
	GetTotalStorage         uint64
	PowerLookup             uint64
	IsMiner                 uint64
	PledgeCollateralForSize uint64
}

var SMAMethods = smaMethods{1, 2, 3, 4, 5, 6, 7, 8}

func (sma StorageMarketActor) Exports() []interface{} {
	return []interface{}{
		//1: sma.StorageMarketConstructor,
		2: sma.CreateStorageMiner,
		3: sma.SlashConsensusFault,
		4: sma.UpdateStorage,
		5: sma.GetTotalStorage,
		6: sma.PowerLookup,
		7: sma.IsMiner,
		8: sma.PledgeCollateralForSize,
	}
}

type StorageMarketState struct {
	Miners     cid.Cid
	MinerCount uint64

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

func SupportedSectorSize(ssize types.BigInt) bool {
	if ssize.Uint64() == build.SectorSize {
		return true
	}
	return false
}

type SlashConsensusFaultParams struct {
	Block1 *types.BlockHeader
	Block2 *types.BlockHeader
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

func (sma StorageMarketActor) SlashConsensusFault(act *types.Actor, vmctx types.VMContext, params *SlashConsensusFaultParams) ([]byte, ActorError) {
	if params.Block1.Miner != params.Block2.Miner {
		return nil, aerrors.New(2, "blocks must be from the same miner")
	}

	rval, err := vmctx.Send(params.Block1.Miner, MAMethods.GetWorkerAddr, types.NewInt(0), nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to get miner worker")
	}

	worker, oerr := address.NewFromBytes(rval)
	if oerr != nil {
		// REVIEW: should this be fatal? i can't think of a real situation that would get us here
		return nil, aerrors.Escalate(oerr, "response from 'GetWorkerAddr' was not a valid address")
	}

	if err := params.Block1.CheckBlockSignature(worker); err != nil {
		return nil, aerrors.Absorb(err, 3, "block1 did not have valid signature")
	}

	if err := params.Block2.CheckBlockSignature(worker); err != nil {
		return nil, aerrors.Absorb(err, 4, "block2 did not have valid signature")
	}

	// see the "Consensus Faults" section of the faults spec (faults.md)
	// for details on these slashing conditions.
	if !shouldSlash(params.Block1, params.Block2) {
		return nil, aerrors.New(5, "blocks do not prove a slashable offense")
	}

	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	if types.BigCmp(self.TotalStorage, types.NewInt(0)) == 0 {
		return nil, aerrors.Fatal("invalid state, storage market actor has zero total storage")
	}

	miner := params.Block1.Miner
	if has, err := MinerSetHas(context.TODO(), vmctx, self.Miners, miner); err != nil {
		return nil, aerrors.Wrapf(err, "failed to check miner in set")
	} else if !has {
		return nil, aerrors.New(6, "either already slashed or not a miner")
	}

	minerBalance, err := vmctx.GetBalance(miner)
	if err != nil {
		return nil, err
	}

	minerPower, err := powerLookup(context.TODO(), vmctx, &self, miner)
	if err != nil {
		return nil, err
	}

	slashedCollateral := pledgeCollateralForSize(minerPower, self.TotalStorage, self.MinerCount)
	if types.BigCmp(slashedCollateral, minerBalance) < 0 {
		slashedCollateral = minerBalance
	}

	// Some of the slashed collateral should be paid to the slasher
	// GROWTH_RATE determines how fast the slasher share of slashed collateral will increase as block elapses
	// current GROWTH_RATE results in SLASHER_SHARE reaches 1 after 30 blocks
	// TODO: define arithmetic precision and rounding for this operation
	blockElapsed := vmctx.BlockHeight() - params.Block1.Height
	growthRate := 1.26
	initialShare := 0.001

	// REVIEW: floating point precision loss anyone?
	slasherPortion := initialShare * math.Pow(growthRate, float64(blockElapsed))
	if slasherPortion > 1 {
		slasherPortion = 1
	}

	slasherShare := types.BigDiv(types.BigMul(types.NewInt(uint64(1000000*slasherPortion)), slashedCollateral), types.NewInt(1000000))
	burnPortion := types.BigSub(slashedCollateral, slasherShare)

	_, err = vmctx.Send(vmctx.Message().From, 0, slasherShare, nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to pay slasher")
	}

	_, err = vmctx.Send(BurntFundsAddress, 0, burnPortion, nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "failed to burn funds")
	}

	// Remove the miner from the list of network miners
	ncid, err := MinerSetRemove(context.TODO(), vmctx, self.Miners, miner)
	if err != nil {
		return nil, err
	}
	self.Miners = ncid
	self.MinerCount--

	self.TotalStorage = types.BigSub(self.TotalStorage, minerPower)

	return nil, nil
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

	pow, err := powerLookup(context.TODO(), vmctx, &self, params.Miner)
	if err != nil {
		return nil, err
	}

	return pow.Bytes(), nil
}

func powerLookup(ctx context.Context, vmctx types.VMContext, self *StorageMarketState, miner address.Address) (types.BigInt, ActorError) {
	has, err := MinerSetHas(context.TODO(), vmctx, self.Miners, miner)
	if err != nil {
		return types.EmptyInt, err
	}

	if !has {
		return types.EmptyInt, aerrors.New(1, "miner not registered with storage market")
	}

	ret, err := vmctx.Send(miner, MAMethods.GetPower, types.NewInt(0), nil)
	if err != nil {
		return types.EmptyInt, aerrors.Wrap(err, "invoke Miner.GetPower")
	}

	return types.BigFromBytes(ret), nil
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

type PledgeCollateralParams struct {
	Size types.BigInt
}

func (sma StorageMarketActor) PledgeCollateralForSize(act *types.Actor, vmctx types.VMContext, param *PledgeCollateralParams) ([]byte, ActorError) {
	var self StorageMarketState
	if err := vmctx.Storage().Get(vmctx.Storage().GetHead(), &self); err != nil {
		return nil, err
	}

	totalCollateral := pledgeCollateralForSize(param.Size, self.TotalStorage, self.MinerCount)

	return totalCollateral.Bytes(), nil
}

func pledgeCollateralForSize(size, totalStorage types.BigInt, minerCount uint64) types.BigInt {

	availableFilecoin := types.NewInt(5000000) // TODO: get actual available filecoin amount

	totalPowerCollateral := types.BigDiv(types.BigMul(availableFilecoin, types.NewInt(uint64(float64(100*build.PowerCollateralProportion)))), types.NewInt(100))
	totalPerCapitaCollateral := types.BigDiv(types.BigMul(availableFilecoin, types.NewInt(uint64(float64(100*build.PowerCollateralProportion)))), types.NewInt(100))

	powerCollateral := types.BigDiv(types.BigMul(totalPowerCollateral, size), totalStorage)
	perCapCollateral := types.BigDiv(totalPerCapitaCollateral, types.NewInt(minerCount))

	return types.BigAdd(powerCollateral, perCapCollateral)
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

	mkey := string(maddr.Bytes())
	err = nd.Find(ctx, mkey, nil)
	if err == nil {
		return cid.Undef, aerrors.Escalate(fmt.Errorf("miner already found"), "miner set add failed")
	}

	if !xerrors.Is(err, hamt.ErrNotFound) {
		return cid.Undef, aerrors.Escalate(err, "failed to do miner set check")
	}

	if err := nd.Set(ctx, mkey, uint64(1)); err != nil {
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

func MinerSetRemove(ctx context.Context, vmctx types.VMContext, rcid cid.Cid, maddr address.Address) (cid.Cid, aerrors.ActorError) {
	nd, err := hamt.LoadNode(ctx, vmctx.Ipld(), rcid)
	if err != nil {
		return cid.Undef, aerrors.Escalate(err, "failed to load miner set")
	}

	mkey := string(maddr.Bytes())
	switch nd.Delete(ctx, mkey) {
	case hamt.ErrNotFound:
		return cid.Undef, aerrors.New(1, "miner not found in set on delete")
	default:
		return cid.Undef, aerrors.Escalate(err, "failed to delete miner from set")
	case nil:
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
