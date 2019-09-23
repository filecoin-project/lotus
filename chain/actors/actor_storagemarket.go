package actors

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"
)

type StorageMarketActor struct{}

type smaMethods struct {
	Constructor             uint64
	CreateStorageMiner      uint64
	ArbitrateConsensusFault uint64
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
		3: sma.ArbitrateConsensusFault,
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

	var self StorageMarketState
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

func SupportedSectorSize(ssize types.BigInt) bool {
	if ssize.Uint64() == build.SectorSize {
		return true
	}
	return false
}

type ArbitrateConsensusFaultParams struct {
	Block1 *types.BlockHeader
	Block2 *types.BlockHeader
}

func (sma StorageMarketActor) ArbitrateConsensusFault(act *types.Actor, vmctx types.VMContext, params *ArbitrateConsensusFaultParams) ([]byte, ActorError) {
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
		return nil, aerrors.Absorb(oerr, 3, "response from 'GetWorkerAddr' was not a valid address")
	}

	if err := params.Block1.CheckBlockSignature(worker); err != nil {
		return nil, aerrors.Absorb(err, 4, "block1 did not have valid signature")
	}

	if err := params.Block2.CheckBlockSignature(worker); err != nil {
		return nil, aerrors.Absorb(err, 5, "block2 did not have valid signature")
	}

	// see the "Consensus Faults" section of the faults spec (faults.md)
	// for details on these slashing conditions.
	if !shouldSlash(params.Block1, params.Block2) {
		return nil, aerrors.New(6, "blocks do not prove a slashable offense")
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
