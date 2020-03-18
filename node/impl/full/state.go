package full

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/prometheus/common/log"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
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

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.ChainSectorInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerSectorSet(ctx, a.StateManager, ts, addr)
}

func (a *StateAPI) StateMinerProvingSet(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.ChainSectorInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerProvingSet(ctx, a.StateManager, ts, addr)
}

func (a *StateAPI) StateMinerPower(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	mpow, tpow, err := stmgr.GetPower(ctx, a.StateManager, ts, maddr)
	if err != nil {
		return nil, err
	}

	if maddr != address.Undef {
		slashed, err := stmgr.GetMinerSlashed(ctx, a.StateManager, ts, maddr)
		if err != nil {
			return nil, err
		}
		if slashed {
			mpow = types.NewInt(0)
		}
	}

	return &api.MinerPower{
		MinerPower: mpow,
		TotalPower: tpow,
	}, nil
}

func (a *StateAPI) StateMinerWorker(ctx context.Context, m address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerWorker(ctx, a.StateManager, ts, m)
}

func (a *StateAPI) StateMinerPeerID(ctx context.Context, m address.Address, tsk types.TipSetKey) (peer.ID, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return "", xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerPeerID(ctx, a.StateManager, ts, m)
}

func (a *StateAPI) StateMinerPostState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*miner.PoStState, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerPostState(ctx, a.StateManager, ts, actor)
}

func (a *StateAPI) StateMinerSectorSize(ctx context.Context, actor address.Address, tsk types.TipSetKey) (abi.SectorSize, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return 0, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerSectorSize(ctx, a.StateManager, ts, actor)
}

func (a *StateAPI) StateMinerFaults(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]abi.SectorNumber, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerFaults(ctx, a.StateManager, ts, addr)
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
		Msg:                m,
		MsgRct:             &r.MessageReceipt,
		InternalExecutions: r.InternalExecutions,
		Error:              errstr,
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

func (a *StateAPI) StateReadState(ctx context.Context, act *types.Actor, tsk types.TipSetKey) (*api.ActorState, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	state, err := a.stateForTs(ctx, ts)
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
func (a *StateAPI) MinerCreateBlock(ctx context.Context, addr address.Address, parentsTSK types.TipSetKey, ticket *types.Ticket, proof *types.EPostProof, msgs []*types.SignedMessage, height abi.ChainEpoch, ts uint64) (*types.BlockMsg, error) {
	parents, err := a.Chain.GetTipSetFromKey(parentsTSK)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", parentsTSK, err)
	}
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
	locked, err := hamt.LoadNode(ctx, cst, state.EscrowTable, hamt.UseTreeBitWidth(5))
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
			return err
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

	minBalance := types.BigDiv(types.BigInt(st.InitialBalance), types.NewInt(uint64(st.UnlockDuration)))
	minBalance = types.BigMul(minBalance, types.NewInt(uint64(offset)))
	return types.BigSub(act.Balance, minBalance), nil
}

func (a *StateAPI) StateListRewards(ctx context.Context, miner address.Address, skip uint64, tsk types.TipSetKey) ([]reward.Reward, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, err
	}

	var st reward.State
	if _, err := a.StateManager.LoadActorState(ctx, builtin.RewardActorAddr, &st, ts); err != nil {
		return nil, xerrors.Errorf("failed to load reward actor state: %w", err)
	}

	as := store.ActorStore(ctx, a.Chain.Blockstore())
	rmap := adt.AsMultimap(as, st.RewardMap)
	rewards, found, err := rmap.Get(adt.AddrKey(miner))
	if err != nil {
		return nil, xerrors.Errorf("failed to get rewards set for miner: %w", err)
	}

	if !found {
		return nil, xerrors.Errorf("no rewards found for miner")
	}

	l, err := rewards.Length()
	if err != nil {
		return nil, err
	}
	if l <= skip {
		return nil, nil
	}

	var out []reward.Reward

	var r reward.Reward
	err = rewards.ForEach(&r, func(i int64) error {
		if skip > 0 {
			skip--
			return nil
		}
		or := r
		out = append(out, or)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("rewards.ForEach failed: %w", err)
	}

	return out, nil
}

func (a *StateAPI) StateListRewardsPath(ctx context.Context, miner address.Address, start_stk, end_stk types.TipSetKey) ([]reward.Reward, error) {
	if start_stk == end_stk {
		return nil, nil
	}
	var end_ts *types.TipSet
	var err error

	if start_stk == types.EmptyTSK {
		return a.StateListRewards(ctx, miner, 0, end_stk)
	}

	if end_ts, err = a.Chain.GetTipSetFromKey(end_stk); err != nil {
		return nil, err
	}

	for end_ts.Key() != start_stk && len(end_ts.Parents().Cids()) > 0 {
		if end_ts, err = a.Chain.GetTipSetFromKey(end_ts.Parents()); err != nil {
			return nil, err
		}
	}

	if start_stk != end_ts.Key() {
		return nil, xerrors.Errorf("specified tipsets dosen't on the same chain")
	}

	var start_rewards, end_rewards []reward.Reward
	if start_rewards, err = a.StateListRewards(ctx, miner, 0, start_stk); err != nil {
		return nil, err
	}

	if end_rewards, err = a.StateListRewards(ctx, miner, uint64(len(start_rewards)), end_stk); err != nil {
		return nil, err
	}

	return end_rewards, nil
}
