package full

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/bufbstore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type StateAPI struct {
	fx.In

	// TODO: the wallet here is only needed because we have the MinerCreateBlock
	// API attached to the state API. It probably should live somewhere better
	Wallet *wallet.Wallet

	ProofVerifier ffiwrapper.Verifier
	StateManager  *stmgr.StateManager
	Chain         *store.ChainStore
	Beacon        beacon.RandomBeacon
}

func (a *StateAPI) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	return stmgr.GetNetworkName(ctx, a.StateManager, a.Chain.GetHeaviestTipSet().ParentState())
}

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address, filter *abi.BitField, filterOut bool, tsk types.TipSetKey) ([]*api.ChainSectorInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerSectorSet(ctx, a.StateManager, ts, addr, filter, filterOut)
}

func (a *StateAPI) StateMinerProvingSet(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.ChainSectorInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	var mas miner.State
	_, err = a.StateManager.LoadActorState(ctx, addr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return stmgr.GetProvingSetRaw(ctx, a.StateManager, mas)
}

func (a *StateAPI) StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (api.MinerInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return api.MinerInfo{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	mi, err := stmgr.StateMinerInfo(ctx, a.StateManager, ts, actor)
	if err != nil {
		return api.MinerInfo{}, err
	}
	return api.NewApiMinerInfo(mi), nil
}

func (a *StateAPI) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) (*miner.Deadlines, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerDeadlines(ctx, a.StateManager, ts, m)
}

func (a *StateAPI) StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*miner.DeadlineInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	var mas miner.State
	_, err = a.StateManager.LoadActorState(ctx, addr, &mas, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return miner.ComputeProvingPeriodDeadline(mas.ProvingPeriodStart, ts.Height()), nil
}

func (a *StateAPI) StateMinerFaults(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.BitField, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerFaults(ctx, a.StateManager, ts, addr)
}

func (a *StateAPI) StateAllMinerFaults(ctx context.Context, lookback abi.ChainEpoch, endTsk types.TipSetKey) ([]*api.Fault, error) {
	endTs, err := a.Chain.GetTipSetFromKey(endTsk)
	if err != nil {
		return nil, xerrors.Errorf("loading end tipset %s: %w", endTsk, err)
	}

	cutoff := endTs.Height() - lookback
	miners, err := stmgr.ListMinerActors(ctx, a.StateManager, endTs)

	if err != nil {
		return nil, xerrors.Errorf("loading miners: %w", err)
	}

	var allFaults []*api.Fault

	for _, m := range miners {
		var mas miner.State
		_, err := a.StateManager.LoadActorState(ctx, m, &mas, endTs)
		if err != nil {
			return nil, xerrors.Errorf("failed to load miner actor state %s: %w", m, err)
		}

		err = mas.ForEachFaultEpoch(a.Chain.Store(ctx), func(faultStart abi.ChainEpoch, faults *abi.BitField) error {
			if faultStart >= cutoff {
				allFaults = append(allFaults, &api.Fault{
					Miner: m,
					Epoch: faultStart,
				})
				return nil
			}
			return nil
		})

		if err != nil {
			return nil, xerrors.Errorf("failure when iterating over miner states: %w", err)
		}
	}

	return allFaults, nil
}

func (a *StateAPI) StateMinerRecoveries(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.BitField, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerRecoveries(ctx, a.StateManager, ts, addr)
}

func (a *StateAPI) StateMinerPower(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	m, net, err := stmgr.GetPower(ctx, a.StateManager, ts, addr)
	if err != nil {
		return nil, err
	}

	return &api.MinerPower{
		MinerPower: m,
		TotalPower: net,
	}, nil
}

func (a *StateAPI) StatePledgeCollateral(ctx context.Context, tsk types.TipSetKey) (types.BigInt, error) {
	/*ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

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

	if ret.MsgRct.ExitCode != 0 {
		return types.NewInt(0), xerrors.Errorf("failed to get miner worker addr (exit code %d)", ret.MsgRct.ExitCode)
	}

	return types.BigFromBytes(ret.Return), nil*/
	log.Error("TODO StatePledgeCollateral")
	return big.Zero(), nil
}

func (a *StateAPI) StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (*api.InvocResult, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return a.StateManager.Call(ctx, msg, ts)
}

func (a *StateAPI) StateReplay(ctx context.Context, tsk types.TipSetKey, mc cid.Cid) (*api.InvocResult, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	m, r, err := a.StateManager.Replay(ctx, ts, mc)
	if err != nil {
		return nil, err
	}

	var errstr string
	if r.ActorErr != nil {
		errstr = r.ActorErr.Error()
	}

	return &api.InvocResult{
		Msg:            m,
		MsgRct:         &r.MessageReceipt,
		ExecutionTrace: r.ExecutionTrace,
		Error:          errstr,
		Duration:       r.Duration,
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
	cst := cbor.NewCborStore(buf)
	return state.LoadStateTree(cst, st)
}

func (a *StateAPI) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	state, err := a.stateForTs(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("computing tipset state failed: %w", err)
	}

	return state.GetActor(actor)
}

func (a *StateAPI) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	state, err := a.stateForTs(ctx, ts)
	if err != nil {
		return address.Undef, err
	}

	return state.LookupID(addr)
}

func (a *StateAPI) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return a.StateManager.ResolveToKeyAddress(ctx, addr, ts)
}

func (a *StateAPI) StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	state, err := a.stateForTs(ctx, ts)
	if err != nil {
		return nil, err
	}

	act, err := state.GetActor(actor)
	if err != nil {
		return nil, err
	}

	blk, err := state.Store.(*cbor.BasicIpldStore).Blocks.Get(act.Head)
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
func (a *StateAPI) MinerGetBaseInfo(ctx context.Context, maddr address.Address, epoch abi.ChainEpoch, tsk types.TipSetKey) (*api.MiningBaseInfo, error) {
	return stmgr.MinerGetBaseInfo(ctx, a.StateManager, a.Beacon, tsk, epoch, maddr, a.ProofVerifier)
}

func (a *StateAPI) MinerCreateBlock(ctx context.Context, bt *api.BlockTemplate) (*types.BlockMsg, error) {
	fblk, err := gen.MinerCreateBlock(ctx, a.StateManager, a.Wallet, bt)
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

func (a *StateAPI) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	ts, recpt, err := a.StateManager.WaitForMessage(ctx, msg, confidence)
	if err != nil {
		return nil, err
	}

	var returndec interface{}
	if recpt.ExitCode == 0 && len(recpt.Return) > 0 {
		cmsg, err := a.Chain.GetCMessage(msg)
		if err != nil {
			return nil, xerrors.Errorf("failed to load message after successful receipt search: %w", err)
		}

		vmsg := cmsg.VMMessage()

		t, err := stmgr.GetReturnType(ctx, a.StateManager, vmsg.To, vmsg.Method, ts)
		if err != nil {
			return nil, xerrors.Errorf("failed to get return type: %w", err)
		}

		if err := t.UnmarshalCBOR(bytes.NewReader(recpt.Return)); err != nil {
			return nil, err
		}

		returndec = t
	}

	return &api.MsgLookup{
		Receipt:   *recpt,
		ReturnDec: returndec,
		TipSet:    ts.Key(),
		Height:    ts.Height(),
	}, nil
}

func (a *StateAPI) StateSearchMsg(ctx context.Context, msg cid.Cid) (*api.MsgLookup, error) {
	ts, recpt, err := a.StateManager.SearchForMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	if ts != nil {
		return &api.MsgLookup{
			Receipt: *recpt,
			TipSet:  ts.Key(),
			Height:  ts.Height(),
		}, nil
	} else {
		return nil, nil
	}
}

func (a *StateAPI) StateGetReceipt(ctx context.Context, msg cid.Cid, tsk types.TipSetKey) (*types.MessageReceipt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return a.StateManager.GetReceipt(ctx, msg, ts)
}

func (a *StateAPI) StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.ListMinerActors(ctx, a.StateManager, ts)
}

func (a *StateAPI) StateListActors(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return a.StateManager.ListAllActors(ctx, ts)
}

func (a *StateAPI) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return api.MarketBalance{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return a.StateManager.MarketBalance(ctx, addr, ts)
}

func (a *StateAPI) StateMarketParticipants(ctx context.Context, tsk types.TipSetKey) (map[string]api.MarketBalance, error) {
	out := map[string]api.MarketBalance{}

	var state market.State
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	if _, err := a.StateManager.LoadActorState(ctx, builtin.StorageMarketActorAddr, &state, ts); err != nil {
		return nil, err
	}
	cst := cbor.NewCborStore(a.StateManager.ChainStore().Blockstore())
	escrow, err := hamt.LoadNode(ctx, cst, state.EscrowTable, hamt.UseTreeBitWidth(5)) // todo: adt map
	if err != nil {
		return nil, err
	}
	locked, err := hamt.LoadNode(ctx, cst, state.LockedTable, hamt.UseTreeBitWidth(5))
	if err != nil {
		return nil, err
	}

	err = escrow.ForEach(ctx, func(k string, val interface{}) error {
		cv := val.(*cbg.Deferred)
		a, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}

		var es abi.TokenAmount
		if err := es.UnmarshalCBOR(bytes.NewReader(cv.Raw)); err != nil {
			return err
		}
		var lk abi.TokenAmount
		if err := locked.Find(ctx, k, &es); err != nil {
			return err
		}

		out[a.String()] = api.MarketBalance{
			Escrow: es,
			Locked: lk,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (a *StateAPI) StateMarketDeals(ctx context.Context, tsk types.TipSetKey) (map[string]api.MarketDeal, error) {
	out := map[string]api.MarketDeal{}

	var state market.State
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	if _, err := a.StateManager.LoadActorState(ctx, builtin.StorageMarketActorAddr, &state, ts); err != nil {
		return nil, err
	}

	blks := cbor.NewCborStore(a.StateManager.ChainStore().Blockstore())
	da, err := amt.LoadAMT(ctx, blks, state.Proposals)
	if err != nil {
		return nil, err
	}

	sa, err := amt.LoadAMT(ctx, blks, state.States)
	if err != nil {
		return nil, err
	}

	if err := da.ForEach(ctx, func(i uint64, v *cbg.Deferred) error {
		var d market.DealProposal
		if err := d.UnmarshalCBOR(bytes.NewReader(v.Raw)); err != nil {
			return err
		}

		var s market.DealState
		if err := sa.Get(ctx, i, &s); err != nil {
			if _, ok := err.(*amt.ErrNotFound); !ok {
				return xerrors.Errorf("failed to get state for deal in proposals array: %w", err)
			}

			s.SectorStartEpoch = -1
		}
		out[strconv.FormatInt(int64(i), 10)] = api.MarketDeal{
			Proposal: d,
			State:    s,
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *StateAPI) StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetStorageDeal(ctx, a.StateManager, dealId, ts)
}

func (a *StateAPI) StateChangedActors(ctx context.Context, old cid.Cid, new cid.Cid) (map[string]types.Actor, error) {
	cst := cbor.NewCborStore(a.Chain.Blockstore())

	nh, err := hamt.LoadNode(ctx, cst, new, hamt.UseTreeBitWidth(5))
	if err != nil {
		return nil, err
	}

	oh, err := hamt.LoadNode(ctx, cst, old, hamt.UseTreeBitWidth(5))
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

func (a *StateAPI) StateMinerSectorCount(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MinerSectors, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return api.MinerSectors{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.SectorSetSizes(ctx, a.StateManager, addr, ts)
}

func (a *StateAPI) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return miner.SectorPreCommitOnChainInfo{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.PreCommitInfo(ctx, a.StateManager, maddr, n, ts)
}

func (a *StateAPI) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.MinerSectorInfo(ctx, a.StateManager, maddr, n, ts)
}

func (a *StateAPI) StateListMessages(ctx context.Context, match *types.Message, tsk types.TipSetKey, toheight abi.ChainEpoch) ([]cid.Cid, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
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

func (a *StateAPI) StateCompute(ctx context.Context, height abi.ChainEpoch, msgs []*types.Message, tsk types.TipSetKey) (*api.ComputeStateOutput, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	st, t, err := stmgr.ComputeState(ctx, a.StateManager, height, msgs, ts)
	if err != nil {
		return nil, err
	}

	return &api.ComputeStateOutput{
		Root:  st,
		Trace: t,
	}, nil
}

func (a *StateAPI) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	var st samsig.State
	act, err := a.StateManager.LoadActorState(ctx, addr, &st, ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor state: %w", err)
	}

	if act.Code != builtin.MultisigActorCodeID {
		return types.EmptyInt, fmt.Errorf("given actor was not a multisig")
	}

	if st.UnlockDuration == 0 {
		return act.Balance, nil
	}

	offset := ts.Height() - st.StartEpoch
	if offset > st.UnlockDuration {
		return act.Balance, nil
	}

	minBalance := types.BigDiv(st.InitialBalance, types.NewInt(uint64(st.UnlockDuration)))
	minBalance = types.BigMul(minBalance, types.NewInt(uint64(offset)))
	return types.BigSub(act.Balance, minBalance), nil
}

var initialPledgeNum = types.NewInt(103)
var initialPledgeDen = types.NewInt(100)

func (a *StateAPI) StateMinerInitialPledgeCollateral(ctx context.Context, maddr address.Address, snum abi.SectorNumber, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.GetActor(maddr, ts)
	if err != nil {
		return types.EmptyInt, err
	}

	as := store.ActorStore(ctx, a.Chain.Blockstore())

	var st miner.State
	if err := as.Get(ctx, act.Head, &st); err != nil {
		return types.EmptyInt, err
	}

	precommit, found, err := st.GetPrecommittedSector(as, snum)
	if err != nil {
		return types.EmptyInt, err
	}

	if !found {
		return types.EmptyInt, xerrors.Errorf("no precommit found for sector %d", snum)
	}

	var dealWeights market.VerifyDealsOnSectorProveCommitReturn
	{
		var err error
		params, err := actors.SerializeParams(&market.VerifyDealsOnSectorProveCommitParams{
			DealIDs:      precommit.Info.DealIDs,
			SectorExpiry: precommit.Info.Expiration,
		})
		if err != nil {
			return types.EmptyInt, err
		}

		ret, err := a.StateManager.Call(ctx, &types.Message{
			From:     maddr,
			To:       builtin.StorageMarketActorAddr,
			Method:   builtin.MethodsMarket.VerifyDealsOnSectorProveCommit,
			GasLimit: 100000000000,
			GasPrice: types.NewInt(0),
			Params:   params,
		}, ts)
		if err != nil {
			return types.EmptyInt, err
		}

		if err := dealWeights.UnmarshalCBOR(bytes.NewReader(ret.MsgRct.Return)); err != nil {
			return types.BigInt{}, err
		}
	}

	initialPledge := big.Zero()
	{
		ssize, err := precommit.Info.SealProof.SectorSize()
		if err != nil {
			return types.EmptyInt, err
		}

		params, err := actors.SerializeParams(&power.OnSectorProveCommitParams{
			Weight: power.SectorStorageWeightDesc{
				SectorSize:         ssize,
				Duration:           precommit.Info.Expiration - ts.Height(), // NB: not exactly accurate, but should always lead us to *over* estimate, not under
				DealWeight:         dealWeights.DealWeight,
				VerifiedDealWeight: dealWeights.VerifiedDealWeight,
			},
		})
		if err != nil {
			return types.EmptyInt, err
		}

		ret, err := a.StateManager.Call(ctx, &types.Message{
			From:     maddr,
			To:       builtin.StoragePowerActorAddr,
			Method:   builtin.MethodsPower.OnSectorProveCommit,
			GasLimit: 10000000000,
			GasPrice: types.NewInt(0),
			Params:   params,
		}, ts)
		if err != nil {
			return types.EmptyInt, err
		}

		if err := initialPledge.UnmarshalCBOR(bytes.NewReader(ret.MsgRct.Return)); err != nil {
			return types.BigInt{}, err
		}
	}

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen), nil
}

func (a *StateAPI) StateMinerAvailableBalance(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.GetActor(maddr, ts)
	if err != nil {
		return types.EmptyInt, err
	}

	as := store.ActorStore(ctx, a.Chain.Blockstore())

	var st miner.State
	if err := as.Get(ctx, act.Head, &st); err != nil {
		return types.EmptyInt, err
	}

	vested, err := st.CheckVestedFunds(as, ts.Height())
	if err != nil {
		return types.EmptyInt, err
	}

	return types.BigAdd(st.GetAvailableBalance(act.Balance), vested), nil
}
