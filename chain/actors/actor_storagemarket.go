package actors

import (
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"

	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

type StorageMarketActor struct{}

type smaMethods struct {
	Constructor                  uint64
	WithdrawBalance              uint64
	AddBalance                   uint64
	CheckLockedBalance           uint64
	PublishStorageDeals          uint64
	HandleCronAction             uint64
	SettleExpiredDeals           uint64
	ProcessStorageDealsPayment   uint64
	SlashStorageDealCollateral   uint64
	GetLastExpirationFromDealIDs uint64
}

var SMAMethods = smaMethods{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

func (sma StorageMarketActor) Exports() []interface{} {
	return []interface{}{
		// 2: sma.WithdrawBalance,
		// 3: sma.AddBalance,
		// 4: sma.CheckLockedBalance,
		// 5: sma.PublishStorageDeals,
		// 6: sma.HandleCronAction,
		// 7: sma.SettleExpiredDeals,
		// 8: sma.ProcessStorageDealsPayment,
		// 9: sma.SlashStorageDealCollateral,
		// 10: sma.GetLastExpirationFromDealIDs,
	}
}

type StorageParticipantBalance struct {
	Locked    types.BigInt
	Available types.BigInt
}

type StorageMarketState struct {
	Balances cid.Cid // hamt
	Deals    cid.Cid // amt
}

type WithdrawBalanceParams struct {
	Balance types.BigInt
}

func (sma StorageMarketActor) WithdrawBalance(act *types.Actor, vmctx types.VMContext, params *WithdrawBalanceParams) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	b, bnd, err := sma.getBalances(vmctx, self.Balances, []address.Address{vmctx.Message().From})
	if err != nil {
		return nil, aerrors.Wrap(err, "could not get balance")
	}

	balance := b[0]

	if balance.Available.LessThan(params.Balance) {
		return nil, aerrors.Newf(1, "can not withdraw more funds than available: %s > %s", params.Balance, b[0].Available)
	}

	balance.Available = types.BigSub(balance.Available, params.Balance)

	_, err = vmctx.Send(vmctx.Message().From, 0, params.Balance, nil)
	if err != nil {
		return nil, aerrors.Wrap(err, "sending funds failed")
	}

	bcid, err := sma.setBalances(vmctx, bnd, map[address.Address]StorageParticipantBalance{
		vmctx.Message().From: balance,
	})
	if err != nil {
		return nil, err
	}

	self.Balances = bcid

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	return nil, vmctx.Storage().Commit(old, nroot)
}

func (sma StorageMarketActor) AddBalance(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {
	var self StorageMarketState
	old := vmctx.Storage().GetHead()
	if err := vmctx.Storage().Get(old, &self); err != nil {
		return nil, err
	}

	b, bnd, err := sma.getBalances(vmctx, self.Balances, []address.Address{vmctx.Message().From})
	if err != nil {
		return nil, aerrors.Wrap(err, "could not get balance")
	}

	balance := b[0]

	balance.Available = types.BigAdd(balance.Available, vmctx.Message().Value)

	bcid, err := sma.setBalances(vmctx, bnd, map[address.Address]StorageParticipantBalance{
		vmctx.Message().From: balance,
	})
	if err != nil {
		return nil, err
	}

	self.Balances = bcid

	nroot, err := vmctx.Storage().Put(&self)
	if err != nil {
		return nil, err
	}

	return nil, vmctx.Storage().Commit(old, nroot)
}

func (sma StorageMarketActor) setBalances(vmctx types.VMContext, nd *hamt.Node, set map[address.Address]StorageParticipantBalance) (cid.Cid, ActorError) {
	for addr, b := range set {
		if err := nd.Set(vmctx.Context(), string(addr.Bytes()), b); err != nil {
			return cid.Undef, aerrors.HandleExternalError(err, "setting new balance")
		}
	}
	if err := nd.Flush(vmctx.Context()); err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "flushing balance hamt")
	}

	c, err := vmctx.Ipld().Put(vmctx.Context(), nd)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, "failed to balances storage")
	}
	return c, nil
}

func (sma StorageMarketActor) getBalances(vmctx types.VMContext, rcid cid.Cid, addrs []address.Address) ([]StorageParticipantBalance, *hamt.Node, ActorError) {
	nd, err := hamt.LoadNode(vmctx.Context(), vmctx.Ipld(), rcid)
	if err != nil {
		return nil, nil, aerrors.HandleExternalError(err, "failed to load miner set")
	}

	out := make([]StorageParticipantBalance, len(addrs))

	for i, a := range addrs {
		var balance StorageParticipantBalance
		err = nd.Find(vmctx.Context(), string(a.Bytes()), &balance)
		switch err {
		case hamt.ErrNotFound:
			out[i] = StorageParticipantBalance{
				Locked:    types.NewInt(0),
				Available: types.NewInt(0),
			}
		case nil:
			out[i] = balance
		default:
			return nil, nil, aerrors.HandleExternalError(err, "failed to do set lookup")
		}

	}

	return out, nd, nil
}

/*
func (sma StorageMarketActor) CheckLockedBalance(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) PublishStorageDeals(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) HandleCronAction(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) SettleExpiredDeals(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) ProcessStorageDealsPayment(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) SlashStorageDealCollateral(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}

func (sma StorageMarketActor) GetLastExpirationFromDealIDs(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, ActorError) {

}
*/
