package market

import (
	"context"
	"sync"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
)

var log = logging.Logger("market_adapter")

type FundMgr struct {
	sm    *stmgr.StateManager
	mpool full.MpoolAPI

	lk        sync.Mutex
	available map[address.Address]types.BigInt
}

func NewFundMgr(sm *stmgr.StateManager, mpool full.MpoolAPI) *FundMgr {
	return &FundMgr{
		sm:    sm,
		mpool: mpool,

		available: map[address.Address]types.BigInt{},
	}
}

func (fm *FundMgr) EnsureAvailable(ctx context.Context, addr, wallet address.Address, amt types.BigInt) (cid.Cid, error) {
	fm.lk.Lock()
	avail, ok := fm.available[addr]
	if !ok {
		bal, err := fm.sm.MarketBalance(ctx, addr, nil)
		if err != nil {
			fm.lk.Unlock()
			return cid.Undef, err
		}

		avail = types.BigSub(bal.Escrow, bal.Locked)
	}

	toAdd := types.NewInt(0)
	avail = types.BigSub(avail, amt)
	if avail.LessThan(types.NewInt(0)) {
		// TODO: some rules around adding more to avoid doing stuff on-chain
		//  all the time
		toAdd = types.BigSub(toAdd, avail)
		avail = types.NewInt(0)
	}
	fm.available[addr] = avail

	fm.lk.Unlock()

	var err error
	params, err := actors.SerializeParams(&addr)
	if err != nil {
		return cid.Undef, err
	}

	smsg, err := fm.mpool.MpoolPushMessage(ctx, &types.Message{
		To:       builtin.StorageMarketActorAddr,
		From:     wallet,
		Value:    toAdd,
		GasPrice: types.NewInt(0),
		GasLimit: 1000000,
		Method:   builtin.MethodsMarket.AddBalance,
		Params:   params,
	})
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}
