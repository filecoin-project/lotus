package full

import (
	"bytes"
	"context"
	"strconv"

	"github.com/filecoin-project/go-amt-ipld"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/bufbstore"
)

type StateAPI struct {
	fx.In

	// TODO: the wallet here is only needed because we have the MinerCreateBlock
	// API attached to the state API. It probably should live somewhere better
	Wallet *wallet.Wallet

	StateManager *stmgr.StateManager
	Chain        *store.ChainStore
}

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address, ts *types.TipSet) ([]*api.ChainSectorInfo, error) {
	return stmgr.GetMinerSectorSet(ctx, a.StateManager, ts, addr)
}

func (a *StateAPI) StateMinerProvingSet(ctx context.Context, addr address.Address, ts *types.TipSet) ([]*api.ChainSectorInfo, error) {
	return stmgr.GetMinerProvingSet(ctx, a.StateManager, ts, addr)
}

func (a *StateAPI) StateMinerPower(ctx context.Context, maddr address.Address, ts *types.TipSet) (api.MinerPower, error) {
	mpow, tpow, err := stmgr.GetPower(ctx, a.StateManager, ts, maddr)
	if err != nil {
		return api.MinerPower{}, err
	}

	if maddr != address.Undef {
		slashed, err := stmgr.GetMinerSlashed(ctx, a.StateManager, ts, maddr)
		if err != nil {
			return api.MinerPower{}, err
		}
		if slashed != 0 {
			mpow = types.NewInt(0)
		}
	}

	return api.MinerPower{
		MinerPower: mpow,
		TotalPower: tpow,
	}, nil
}

func (a *StateAPI) StateMinerWorker(ctx context.Context, m address.Address, ts *types.TipSet) (address.Address, error) {
	return stmgr.GetMinerWorker(ctx, a.StateManager, ts, m)
}

func (a *StateAPI) StateMinerPeerID(ctx context.Context, m address.Address, ts *types.TipSet) (peer.ID, error) {
	return stmgr.GetMinerPeerID(ctx, a.StateManager, ts, m)
}

func (a *StateAPI) StateMinerElectionPeriodStart(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error) {
	return stmgr.GetMinerElectionPeriodStart(ctx, a.StateManager, ts, actor)
}

func (a *StateAPI) StateMinerSectorSize(ctx context.Context, actor address.Address, ts *types.TipSet) (uint64, error) {
	return stmgr.GetMinerSectorSize(ctx, a.StateManager, ts, actor)
}

func (a *StateAPI) StatePledgeCollateral(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	param, err := actors.SerializeParams(&actors.PledgeCollateralParams{Size: types.NewInt(0)})
	if err != nil {
		return types.NewInt(0), err
	}

	ret, aerr := a.StateManager.Call(ctx, &types.Message{
		From:   actors.StoragePowerAddress,
		To:     actors.StoragePowerAddress,
		Method: actors.SPAMethods.PledgeCollateralForSize,

		Params: param,
	}, ts)
	if aerr != nil {
		return types.NewInt(0), xerrors.Errorf("failed to get miner worker addr: %w", err)
	}

	if ret.ExitCode != 0 {
		return types.NewInt(0), xerrors.Errorf("failed to get miner worker addr (exit code %d)", ret.ExitCode)
	}

	return types.BigFromBytes(ret.Return), nil
}

func (a *StateAPI) StateCall(ctx context.Context, msg *types.Message, ts *types.TipSet) (*types.MessageReceipt, error) {
	return a.StateManager.Call(ctx, msg, ts)
}

func (a *StateAPI) StateReplay(ctx context.Context, ts *types.TipSet, mc cid.Cid) (*api.ReplayResults, error) {
	m, r, err := a.StateManager.Replay(ctx, ts, mc)
	if err != nil {
		return nil, err
	}

	var errstr string
	if r.ActorErr != nil {
		errstr = r.ActorErr.Error()
	}

	return &api.ReplayResults{
		Msg:     m,
		Receipt: &r.MessageReceipt,
		Error:   errstr,
	}, nil
}

func (a *StateAPI) stateForTs(ctx context.Context, ts *types.TipSet) (*state.StateTree, error) {
	if ts == nil {
		ts = a.Chain.GetHeaviestTipSet()
	}

	st, _, err := a.StateManager.TipSetState(ctx, ts)
	if err != nil {
		return nil, err
	}

	buf := bufbstore.NewBufferedBstore(a.Chain.Blockstore())
	cst := hamt.CSTFromBstore(buf)
	return state.LoadStateTree(cst, st)
}

func (a *StateAPI) StateGetActor(ctx context.Context, actor address.Address, ts *types.TipSet) (*types.Actor, error) {
	state, err := a.stateForTs(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("computing tipset state failed: %w", err)
	}

	return state.GetActor(actor)
}

func (a *StateAPI) StateLookupID(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	state, err := a.stateForTs(ctx, ts)
	if err != nil {
		return address.Undef, err
	}

	return state.LookupID(addr)
}

func (a *StateAPI) StateReadState(ctx context.Context, act *types.Actor, ts *types.TipSet) (*api.ActorState, error) {
	state, err := a.stateForTs(ctx, ts)
	if err != nil {
		return nil, err
	}

	blk, err := state.Store.Blocks.GetBlock(ctx, act.Head)
	if err != nil {
		return nil, err
	}

	oif, err := vm.DumpActorState(act.Code, blk.RawData())
	if err != nil {
		return nil, err
	}

	return &api.ActorState{
		Balance: act.Balance,
		State:   oif,
	}, nil
}

// This is on StateAPI because miner.Miner requires this, and MinerAPI requires miner.Miner
func (a *StateAPI) MinerCreateBlock(ctx context.Context, addr address.Address, parents *types.TipSet, ticket *types.Ticket, proof *types.EPostProof, msgs []*types.SignedMessage, height, ts uint64) (*types.BlockMsg, error) {
	fblk, err := gen.MinerCreateBlock(ctx, a.StateManager, a.Wallet, addr, parents, ticket, proof, msgs, height, ts)
	if err != nil {
		return nil, err
	}

	var out types.BlockMsg
	out.Header = fblk.Header
	for _, msg := range fblk.BlsMessages {
		out.BlsMessages = append(out.BlsMessages, msg.Cid())
	}
	for _, msg := range fblk.SecpkMessages {
		out.SecpkMessages = append(out.SecpkMessages, msg.Cid())
	}

	return &out, nil
}

func (a *StateAPI) StateWaitMsg(ctx context.Context, msg cid.Cid) (*api.MsgWait, error) {
	// TODO: consider using event system for this, expose confidence

	ts, recpt, err := a.StateManager.WaitForMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	return &api.MsgWait{
		Receipt: *recpt,
		TipSet:  ts,
	}, nil
}

func (a *StateAPI) StateGetReceipt(ctx context.Context, msg cid.Cid, ts *types.TipSet) (*types.MessageReceipt, error) {
	return a.StateManager.GetReceipt(ctx, msg, ts)
}

func (a *StateAPI) StateListMiners(ctx context.Context, ts *types.TipSet) ([]address.Address, error) {
	return stmgr.ListMinerActors(ctx, a.StateManager, ts)
}

func (a *StateAPI) StateListActors(ctx context.Context, ts *types.TipSet) ([]address.Address, error) {
	return a.StateManager.ListAllActors(ctx, ts)
}

func (a *StateAPI) StateMarketBalance(ctx context.Context, addr address.Address, ts *types.TipSet) (actors.StorageParticipantBalance, error) {
	return a.StateManager.MarketBalance(ctx, addr, ts)
}

func (a *StateAPI) StateMarketParticipants(ctx context.Context, ts *types.TipSet) (map[string]actors.StorageParticipantBalance, error) {
	out := map[string]actors.StorageParticipantBalance{}

	var state actors.StorageMarketState
	if _, err := a.StateManager.LoadActorState(ctx, actors.StorageMarketAddress, &state, ts); err != nil {
		return nil, err
	}
	cst := hamt.CSTFromBstore(a.StateManager.ChainStore().Blockstore())
	nd, err := hamt.LoadNode(ctx, cst, state.Balances)
	if err != nil {
		return nil, err
	}

	err = nd.ForEach(ctx, func(k string, val interface{}) error {
		cv := val.(*cbg.Deferred)
		a, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}
		var b actors.StorageParticipantBalance
		if err := b.UnmarshalCBOR(bytes.NewReader(cv.Raw)); err != nil {
			return err
		}
		out[a.String()] = b
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (a *StateAPI) StateMarketDeals(ctx context.Context, ts *types.TipSet) (map[string]actors.OnChainDeal, error) {
	out := map[string]actors.OnChainDeal{}

	var state actors.StorageMarketState
	if _, err := a.StateManager.LoadActorState(ctx, actors.StorageMarketAddress, &state, ts); err != nil {
		return nil, err
	}

	blks := amt.WrapBlockstore(a.StateManager.ChainStore().Blockstore())
	da, err := amt.LoadAMT(blks, state.Deals)
	if err != nil {
		return nil, err
	}

	if err := da.ForEach(func(i uint64, v *cbg.Deferred) error {
		var d actors.OnChainDeal
		if err := d.UnmarshalCBOR(bytes.NewReader(v.Raw)); err != nil {
			return err
		}
		out[strconv.FormatInt(int64(i), 10)] = d
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *StateAPI) StateMarketStorageDeal(ctx context.Context, dealId uint64, ts *types.TipSet) (*actors.OnChainDeal, error) {
	return stmgr.GetStorageDeal(ctx, a.StateManager, dealId, ts)
}

func (a *StateAPI) StateChangedActors(ctx context.Context, old cid.Cid, new cid.Cid) (map[string]types.Actor, error) {
	cst := hamt.CSTFromBstore(a.Chain.Blockstore())

	nh, err := hamt.LoadNode(ctx, cst, new)
	if err != nil {
		return nil, err
	}

	oh, err := hamt.LoadNode(ctx, cst, old)
	if err != nil {
		return nil, err
	}

	out := map[string]types.Actor{}

	err = nh.ForEach(ctx, func(k string, nval interface{}) error {
		ncval := nval.(*cbg.Deferred)
		var act types.Actor

		var ocval cbg.Deferred

		switch err := oh.Find(ctx, k, &ocval); err {
		case nil:
			if bytes.Equal(ocval.Raw, ncval.Raw) {
				return nil // not changed
			}
			fallthrough
		case hamt.ErrNotFound:
			if err := act.UnmarshalCBOR(bytes.NewReader(ncval.Raw)); err != nil {
				return err
			}

			addr, err := address.NewFromBytes([]byte(k))
			if err != nil {
				return xerrors.Errorf("address in state tree was not valid: %w", err)
			}

			out[addr.String()] = act
		default:
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (a *StateAPI) StateMinerSectorCount(ctx context.Context, addr address.Address, ts *types.TipSet) (api.MinerSectors, error) {
	return stmgr.SectorSetSizes(ctx, a.StateManager, addr, ts)
}

func (a *StateAPI) StateListMessages(ctx context.Context, match *types.Message, ts *types.TipSet, toheight uint64) ([]cid.Cid, error) {
	if ts == nil {
		ts = a.Chain.GetHeaviestTipSet()
	}

	if match.To == address.Undef && match.From == address.Undef {
		return nil, xerrors.Errorf("must specify at least To or From in message filter")
	}

	matchFunc := func(msg *types.Message) bool {
		if match.From != address.Undef && match.From != msg.From {
			return false
		}

		if match.To != address.Undef && match.To != msg.To {
			return false
		}

		return true
	}

	var out []cid.Cid
	for ts.Height() >= toheight {
		msgs, err := a.Chain.MessagesForTipset(ts)
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for tipset (%s): %w", ts.Key(), err)
		}

		for _, msg := range msgs {
			if matchFunc(msg.VMMessage()) {
				out = append(out, msg.Cid())
			}
		}

		if ts.Height() == 0 {
			break
		}

		next, err := a.Chain.LoadTipSet(ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("loading next tipset: %w", err)
		}

		ts = next
	}

	return out, nil
}

func (a *StateAPI) StateCompute(ctx context.Context, height uint64, msgs []*types.Message, ts *types.TipSet) (cid.Cid, error) {
	return stmgr.ComputeState(ctx, a.StateManager, height, msgs, ts)
}
