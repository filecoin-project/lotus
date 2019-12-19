package market

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
)

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

func (fm *FundMgr) EnsureAvailable(ctx context.Context, addr address.Address, amt types.BigInt) error {
	fm.lk.Lock()
	avail, ok := fm.available[addr]
	if !ok {
		bal, err := fm.sm.MarketBalance(ctx, addr, nil)
		if err != nil {
			fm.lk.Unlock()
			return err
		}

		avail = bal.Available
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

	smsg, err := fm.mpool.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     addr,
		Value:    toAdd,
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   actors.SMAMethods.AddBalance,
	})
	if err != nil {
		return err
	}

	_, r, err := fm.sm.WaitForMessage(ctx, smsg.Cid())
	if err != nil {
		return err
	}

	if r.ExitCode != 0 {
		return xerrors.Errorf("adding funds to storage miner market actor failed: exit %d", r.ExitCode)
	}
	return nil
}
