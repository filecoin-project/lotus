package market

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"go.uber.org/fx"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
)

var log = logging.Logger("market_adapter")

// API is the dependencies need to run a fund manager
type API struct {
	fx.In

	full.ChainAPI
	full.StateAPI
	full.MpoolAPI
}

// FundMgr monitors available balances and adds funds when EnsureAvailable is called
type FundMgr struct {
	api fundMgrAPI

	lk        sync.RWMutex
	available map[address.Address]types.BigInt
}

// StartFundManager creates a new fund manager and sets up event hooks to manage state changes
func StartFundManager(lc fx.Lifecycle, api API) *FundMgr {
	fm := newFundMgr(&api)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ev := events.NewEvents(ctx, &api)
			preds := state.NewStatePredicates(&api)
			dealDiffFn := preds.OnStorageMarketActorChanged(preds.OnBalanceChanged(preds.AvailableBalanceChangedForAddresses(fm.getAddresses)))
			match := func(oldTs, newTs *types.TipSet) (bool, events.StateChange, error) {
				return dealDiffFn(ctx, oldTs.Key(), newTs.Key())
			}
			return ev.StateChanged(fm.checkFunc, fm.stateChanged, fm.revert, 0, events.NoTimeout, match)
		},
	})
	return fm
}

type fundMgrAPI interface {
	StateMarketBalance(context.Context, address.Address, types.TipSetKey) (api.MarketBalance, error)
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
}

func newFundMgr(api fundMgrAPI) *FundMgr {
	return &FundMgr{
		api:       api,
		available: map[address.Address]types.BigInt{},
	}
}

// checkFunc tells the events api to simply proceed (we always want to watch)
func (fm *FundMgr) checkFunc(ts *types.TipSet) (done bool, more bool, err error) {
	return false, true, nil
}

// revert handles reverts to balances
func (fm *FundMgr) revert(ctx context.Context, ts *types.TipSet) error {
	// TODO: Is it ok to just ignore this?
	log.Warn("balance change reverted; TODO: actually handle this!")
	return nil
}

// stateChanged handles balance changes monitored on the chain from one tipset to the next
func (fm *FundMgr) stateChanged(ts *types.TipSet, ts2 *types.TipSet, states events.StateChange, h abi.ChainEpoch) (more bool, err error) {
	changedBalances, ok := states.(state.ChangedBalances)
	if !ok {
		panic("Expected state.ChangedBalances")
	}
	// overwrite our in memory cache with new values from chain (chain is canonical)
	fm.lk.Lock()
	for addr, balanceChange := range changedBalances {
		if fm.available[addr].Int != nil {
			log.Infof("State balance change recorded, prev: %s, new: %s", fm.available[addr].String(), balanceChange.To.String())
		}

		fm.available[addr] = balanceChange.To
	}
	fm.lk.Unlock()
	return true, nil
}

func (fm *FundMgr) getAddresses() []address.Address {
	fm.lk.RLock()
	defer fm.lk.RUnlock()
	addrs := make([]address.Address, 0, len(fm.available))
	for addr := range fm.available {
		addrs = append(addrs, addr)
	}
	return addrs
}

// EnsureAvailable looks at the available balance in escrow for a given
// address, and if less than the passed in amount, adds the difference
func (fm *FundMgr) EnsureAvailable(ctx context.Context, addr, wallet address.Address, amt types.BigInt) (cid.Cid, error) {
	idAddr, err := fm.api.StateLookupID(ctx, addr, types.EmptyTSK)
	if err != nil {
		return cid.Undef, err
	}
	fm.lk.Lock()
	defer fm.lk.Unlock()

	bal, err := fm.api.StateMarketBalance(ctx, addr, types.EmptyTSK)
	if err != nil {
		return cid.Undef, err
	}

	stateAvail := types.BigSub(bal.Escrow, bal.Locked)

	avail, ok := fm.available[idAddr]
	if !ok {
		avail = stateAvail
	}

	toAdd := types.BigSub(amt, avail)
	if toAdd.LessThan(types.NewInt(0)) {
		toAdd = types.NewInt(0)
	}
	fm.available[idAddr] = big.Add(avail, toAdd)

	log.Infof("Funds operation w/ Expected Balance: %s, In State: %s, Requested: %s, Adding: %s", avail.String(), stateAvail.String(), amt.String(), toAdd.String())

	if toAdd.LessThanEqual(big.Zero()) {
		return cid.Undef, nil
	}

	params, err := actors.SerializeParams(&addr)
	if err != nil {
		fm.available[idAddr] = avail
		return cid.Undef, err
	}

	smsg, err := fm.api.MpoolPushMessage(ctx, &types.Message{
		To:     market.Address,
		From:   wallet,
		Value:  toAdd,
		Method: builtin.MethodsMarket.AddBalance,
		Params: params,
	}, nil)
	if err != nil {
		fm.available[idAddr] = avail
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}
