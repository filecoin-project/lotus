package full

import (
	"bytes"
	"context"
	"strconv"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/actors/policy"
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

type StateModuleAPI interface {
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error)
	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
}

// StateModule provides a default implementation of StateModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type StateModule struct {
	fx.In

	StateManager *stmgr.StateManager
	Chain        *store.ChainStore
}

var _ StateModuleAPI = (*StateModule)(nil)

type StateAPI struct {
	fx.In

	// TODO: the wallet here is only needed because we have the MinerCreateBlock
	// API attached to the state API. It probably should live somewhere better
	Wallet    api.WalletAPI
	DefWallet wallet.Default

	StateModuleAPI

	ProofVerifier ffiwrapper.Verifier
	StateManager  *stmgr.StateManager
	Chain         *store.ChainStore
	Beacon        beacon.Schedule
}

func (a *StateAPI) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	return stmgr.GetNetworkName(ctx, a.StateManager, a.Chain.GetHeaviestTipSet().ParentState())
}

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address, sectorNos *bitfield.BitField, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerSectorSet(ctx, a.StateManager, ts, addr, sectorNos)
}

func (a *StateAPI) StateMinerActiveSectors(ctx context.Context, maddr address.Address, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error) { // TODO: only used in cli
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActorTsk(ctx, maddr, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	activeSectors, err := miner.AllPartSectors(mas, miner.Partition.ActiveSectors)
	if err != nil {
		return nil, xerrors.Errorf("merge partition active sets: %w", err)
	}

	return stmgr.GetMinerSectorSet(ctx, a.StateManager, ts, maddr, &activeSectors)
}

func (a *StateAPI) StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (miner.MinerInfo, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, actor, tsk)
	if err != nil {
		return miner.MinerInfo{}, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return miner.MinerInfo{}, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	return mas.Info()
}

func (a *StateAPI) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) ([]api.Deadline, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, m, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	deadlines, err := mas.NumDeadlines()
	if err != nil {
		return nil, xerrors.Errorf("getting deadline count: %w", err)
	}

	out := make([]api.Deadline, deadlines)
	if err := mas.ForEachDeadline(func(i uint64, dl miner.Deadline) error {
		ps, err := dl.PostSubmissions()
		if err != nil {
			return err
		}

		out[i] = api.Deadline{
			PostSubmissions: ps,
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *StateAPI) StateMinerPartitions(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]api.Partition, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, m, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	dl, err := mas.LoadDeadline(dlIdx)
	if err != nil {
		return nil, xerrors.Errorf("failed to load the deadline: %w", err)
	}

	var out []api.Partition
	err = dl.ForEachPartition(func(_ uint64, part miner.Partition) error {
		allSectors, err := part.AllSectors()
		if err != nil {
			return xerrors.Errorf("getting AllSectors: %w", err)
		}

		faultySectors, err := part.FaultySectors()
		if err != nil {
			return xerrors.Errorf("getting FaultySectors: %w", err)
		}

		recoveringSectors, err := part.RecoveringSectors()
		if err != nil {
			return xerrors.Errorf("getting RecoveringSectors: %w", err)
		}

		liveSectors, err := part.LiveSectors()
		if err != nil {
			return xerrors.Errorf("getting LiveSectors: %w", err)
		}

		activeSectors, err := part.ActiveSectors()
		if err != nil {
			return xerrors.Errorf("getting ActiveSectors: %w", err)
		}

		out = append(out, api.Partition{
			AllSectors:        allSectors,
			FaultySectors:     faultySectors,
			RecoveringSectors: recoveringSectors,
			LiveSectors:       liveSectors,
			ActiveSectors:     activeSectors,
		})
		return nil
	})

	return out, err
}

func (a *StateAPI) StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*dline.Info, error) {
	ts, err := a.StateManager.ChainStore().GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	di, err := mas.DeadlineInfo(ts.Height())
	if err != nil {
		return nil, xerrors.Errorf("failed to get deadline info: %w", err)
	}

	return di.NextNotElapsed(), nil
}

func (a *StateAPI) StateMinerFaults(ctx context.Context, addr address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, addr, tsk)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	return miner.AllPartSectors(mas, miner.Partition.FaultySectors)
}

func (a *StateAPI) StateAllMinerFaults(ctx context.Context, lookback abi.ChainEpoch, endTsk types.TipSetKey) ([]*api.Fault, error) {
	return nil, xerrors.Errorf("fixme")

	/*endTs, err := a.Chain.GetTipSetFromKey(endTsk)
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

		err = mas.ForEachFaultEpoch(a.Chain.Store(ctx), func(faultStart abi.ChainEpoch, faults abi.BitField) error {
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

	return allFaults, nil*/
}

func (a *StateAPI) StateMinerRecoveries(ctx context.Context, addr address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, addr, tsk)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	return miner.AllPartSectors(mas, miner.Partition.RecoveringSectors)
}

func (a *StateAPI) StateMinerPower(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	m, net, hmp, err := stmgr.GetPower(ctx, a.StateManager, ts, addr)
	if err != nil {
		return nil, err
	}

	return &api.MinerPower{
		MinerPower:  m,
		TotalPower:  net,
		HasMinPower: hmp,
	}, nil
}

func (a *StateAPI) StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (res *api.InvocResult, err error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	for {
		res, err = a.StateManager.Call(ctx, msg, ts)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = a.Chain.GetTipSetFromKey(ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}
	return res, err
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
		MsgCid:         mc,
		Msg:            m,
		MsgRct:         &r.MessageReceipt,
		ExecutionTrace: r.ExecutionTrace,
		Error:          errstr,
		Duration:       r.Duration,
	}, nil
}

func stateForTs(ctx context.Context, ts *types.TipSet, cstore *store.ChainStore, smgr *stmgr.StateManager) (*state.StateTree, error) {
	if ts == nil {
		ts = cstore.GetHeaviestTipSet()
	}

	st, _, err := smgr.TipSetState(ctx, ts)
	if err != nil {
		return nil, err
	}

	buf := bufbstore.NewBufferedBstore(cstore.Blockstore())
	cst := cbor.NewCborStore(buf)
	return state.LoadStateTree(cst, st)
}
func (a *StateAPI) stateForTs(ctx context.Context, ts *types.TipSet) (*state.StateTree, error) {
	return stateForTs(ctx, ts, a.Chain, a.StateManager)
}
func (m *StateModule) stateForTs(ctx context.Context, ts *types.TipSet) (*state.StateTree, error) {
	return stateForTs(ctx, ts, m.Chain, m.StateManager)
}

func (m *StateModule) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	ts, err := m.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	state, err := m.stateForTs(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("computing tipset state failed: %w", err)
	}

	return state.GetActor(actor)
}

func (m *StateModule) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := m.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return m.StateManager.LookupID(ctx, addr, ts)
}

func (m *StateModule) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := m.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return m.StateManager.ResolveToKeyAddress(ctx, addr, ts)
}

func (a *StateAPI) StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	state, err := a.stateForTs(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("getting state for tipset: %w", err)
	}

	act, err := state.GetActor(actor)
	if err != nil {
		return nil, xerrors.Errorf("getting actor: %w", err)
	}

	blk, err := state.Store.(*cbor.BasicIpldStore).Blocks.Get(act.Head)
	if err != nil {
		return nil, xerrors.Errorf("getting actor head: %w", err)
	}

	oif, err := vm.DumpActorState(act, blk.RawData())
	if err != nil {
		return nil, xerrors.Errorf("dumping actor state (a:%s): %w", actor, err)
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

func (m *StateModule) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	return stateWaitMsgLimited(ctx, m.StateManager, m.Chain, msg, confidence, stmgr.LookbackNoLimit)
}
func (a *StateAPI) StateWaitMsgLimited(ctx context.Context, msg cid.Cid, confidence uint64, lookbackLimit abi.ChainEpoch) (*api.MsgLookup, error) {
	return stateWaitMsgLimited(ctx, a.StateManager, a.Chain, msg, confidence, lookbackLimit)
}
func stateWaitMsgLimited(ctx context.Context, smgr *stmgr.StateManager, cstore *store.ChainStore, msg cid.Cid, confidence uint64, lookbackLimit abi.ChainEpoch) (*api.MsgLookup, error) {
	ts, recpt, found, err := smgr.WaitForMessage(ctx, msg, confidence, lookbackLimit)
	if err != nil {
		return nil, err
	}

	var returndec interface{}
	if recpt.ExitCode == 0 && len(recpt.Return) > 0 {
		cmsg, err := cstore.GetCMessage(msg)
		if err != nil {
			return nil, xerrors.Errorf("failed to load message after successful receipt search: %w", err)
		}

		vmsg := cmsg.VMMessage()

		t, err := stmgr.GetReturnType(ctx, smgr, vmsg.To, vmsg.Method, ts)
		if err != nil {
			return nil, xerrors.Errorf("failed to get return type: %w", err)
		}

		if err := t.UnmarshalCBOR(bytes.NewReader(recpt.Return)); err != nil {
			return nil, err
		}

		returndec = t
	}

	return &api.MsgLookup{
		Message:   found,
		Receipt:   *recpt,
		ReturnDec: returndec,
		TipSet:    ts.Key(),
		Height:    ts.Height(),
	}, nil
}

func (a *StateAPI) StateSearchMsg(ctx context.Context, msg cid.Cid) (*api.MsgLookup, error) {
	ts, recpt, found, err := a.StateManager.SearchForMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	if ts != nil {
		return &api.MsgLookup{
			Message: found,
			Receipt: *recpt,
			TipSet:  ts.Key(),
			Height:  ts.Height(),
		}, nil
	}
	return nil, nil
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

	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	state, err := a.StateManager.GetMarketState(ctx, ts)
	if err != nil {
		return nil, err
	}
	escrow, err := state.EscrowTable()
	if err != nil {
		return nil, err
	}
	locked, err := state.LockedTable()
	if err != nil {
		return nil, err
	}

	err = escrow.ForEach(func(a address.Address, es abi.TokenAmount) error {

		lk, err := locked.Get(a)
		if err != nil {
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

	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	state, err := a.StateManager.GetMarketState(ctx, ts)
	if err != nil {
		return nil, err
	}

	da, err := state.Proposals()
	if err != nil {
		return nil, err
	}

	sa, err := state.States()
	if err != nil {
		return nil, err
	}

	if err := da.ForEach(func(dealID abi.DealID, d market.DealProposal) error {
		s, found, err := sa.Get(dealID)
		if err != nil {
			return xerrors.Errorf("failed to get state for deal in proposals array: %w", err)
		} else if !found {
			s = market.EmptyDealState()
		}
		out[strconv.FormatInt(int64(dealID), 10)] = api.MarketDeal{
			Proposal: d,
			State:    *s,
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
	store := a.Chain.Store(ctx)

	oldTree, err := state.LoadStateTree(store, old)
	if err != nil {
		return nil, xerrors.Errorf("failed to load old state tree: %w", err)
	}

	newTree, err := state.LoadStateTree(store, new)
	if err != nil {
		return nil, xerrors.Errorf("failed to load new state tree: %w", err)
	}

	return state.Diff(oldTree, newTree)
}

func (a *StateAPI) StateMinerSectorCount(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MinerSectors, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, addr, tsk)
	if err != nil {
		return api.MinerSectors{}, err
	}
	mas, err := miner.Load(a.Chain.Store(ctx), act)
	if err != nil {
		return api.MinerSectors{}, err
	}
	var activeCount, liveCount, faultyCount uint64
	if err := mas.ForEachDeadline(func(_ uint64, dl miner.Deadline) error {
		return dl.ForEachPartition(func(_ uint64, part miner.Partition) error {
			if active, err := part.ActiveSectors(); err != nil {
				return err
			} else if count, err := active.Count(); err != nil {
				return err
			} else {
				activeCount += count
			}
			if live, err := part.LiveSectors(); err != nil {
				return err
			} else if count, err := live.Count(); err != nil {
				return err
			} else {
				liveCount += count
			}
			if faulty, err := part.FaultySectors(); err != nil {
				return err
			} else if count, err := faulty.Count(); err != nil {
				return err
			} else {
				faultyCount += count
			}
			return nil
		})
	}); err != nil {
		return api.MinerSectors{}, err
	}
	return api.MinerSectors{Live: liveCount, Active: activeCount, Faulty: faultyCount}, nil
}

func (a *StateAPI) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (miner.SectorPreCommitOnChainInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return miner.SectorPreCommitOnChainInfo{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	pci, err := stmgr.PreCommitInfo(ctx, a.StateManager, maddr, n, ts)
	if err != nil {
		return miner.SectorPreCommitOnChainInfo{}, err
	}

	return *pci, err
}

func (a *StateAPI) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.MinerSectorInfo(ctx, a.StateManager, maddr, n, ts)
}

func (a *StateAPI) StateSectorExpiration(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorExpiration, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, maddr, tsk)
	if err != nil {
		return nil, err
	}
	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, err
	}
	return mas.GetSectorExpiration(sectorNumber)
}

func (a *StateAPI) StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorLocation, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, maddr, tsk)
	if err != nil {
		return nil, err
	}
	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, err
	}
	return mas.FindSector(sectorNumber)
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

func (m *StateModule) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := m.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := m.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor: %w", err)
	}
	msas, err := multisig.Load(m.Chain.Store(ctx), act)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor state: %w", err)
	}
	locked, err := msas.LockedBalance(ts.Height())
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to compute locked multisig balance: %w", err)
	}
	return types.BigSub(act.Balance, locked), nil
}

func (a *StateAPI) MsigGetVestingSchedule(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MsigVesting, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("failed to load multisig actor: %w", err)
	}

	msas, err := multisig.Load(a.Chain.Store(ctx), act)
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("failed to load multisig actor state: %w", err)
	}

	ib, err := msas.InitialBalance()
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("failed to load multisig initial balance: %w", err)
	}

	se, err := msas.StartEpoch()
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("failed to load multisig start epoch: %w", err)
	}

	ud, err := msas.UnlockDuration()
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("failed to load multisig unlock duration: %w", err)
	}

	return api.MsigVesting{
		InitialBalance: ib,
		StartEpoch:     se,
		UnlockDuration: ud,
	}, nil
}

func (m *StateModule) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	startTs, err := m.Chain.GetTipSetFromKey(start)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading start tipset %s: %w", start, err)
	}

	endTs, err := m.Chain.GetTipSetFromKey(end)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading end tipset %s: %w", end, err)
	}

	if startTs.Height() > endTs.Height() {
		return types.EmptyInt, xerrors.Errorf("start tipset %d is after end tipset %d", startTs.Height(), endTs.Height())
	} else if startTs.Height() == endTs.Height() {
		return big.Zero(), nil
	}

	act, err := m.StateManager.LoadActor(ctx, addr, endTs)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor at end epoch: %w", err)
	}

	msas, err := multisig.Load(m.Chain.Store(ctx), act)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor state: %w", err)
	}

	startLk, err := msas.LockedBalance(startTs.Height())
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to compute locked balance at start height: %w", err)
	}

	endLk, err := msas.LockedBalance(endTs.Height())
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to compute locked balance at end height: %w", err)
	}

	return types.BigSub(startLk, endLk), nil
}

var initialPledgeNum = types.NewInt(110)
var initialPledgeDen = types.NewInt(100)

func (a *StateAPI) StateMinerPreCommitDepositForPower(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	state, err := a.StateManager.ParentState(ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading state %s: %w", tsk, err)
	}

	ssize, err := pci.SealProof.SectorSize()
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to get resolve size: %w", err)
	}

	store := a.Chain.Store(ctx)

	var sectorWeight abi.StoragePower
	if act, err := state.GetActor(market.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading market actor %s: %w", maddr, err)
	} else if s, err := market.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading market actor state %s: %w", maddr, err)
	} else if w, vw, err := s.VerifyDealsForActivation(maddr, pci.DealIDs, ts.Height(), pci.Expiration); err != nil {
		return types.EmptyInt, xerrors.Errorf("verifying deals for activation: %w", err)
	} else {
		// NB: not exactly accurate, but should always lead us to *over* estimate, not under
		duration := pci.Expiration - ts.Height()
		sectorWeight = builtin.QAPowerForWeight(ssize, duration, w, vw)
	}

	var powerSmoothed builtin.FilterEstimate
	if act, err := state.GetActor(power.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading power actor: %w", err)
	} else if s, err := power.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading power actor state: %w", err)
	} else if p, err := s.TotalPowerSmoothed(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine total power: %w", err)
	} else {
		powerSmoothed = p
	}

	rewardActor, err := state.GetActor(reward.Address)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor: %w", err)
	}

	rewardState, err := reward.Load(store, rewardActor)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading reward actor state: %w", err)
	}

	deposit, err := rewardState.PreCommitDepositForPower(powerSmoothed, sectorWeight)
	if err != nil {
		return big.Zero(), xerrors.Errorf("calculating precommit deposit: %w", err)
	}

	return types.BigDiv(types.BigMul(deposit, initialPledgeNum), initialPledgeDen), nil
}

func (a *StateAPI) StateMinerInitialPledgeCollateral(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error) {
	// TODO: this repeats a lot of the previous function. Fix that.
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	state, err := a.StateManager.ParentState(ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading state %s: %w", tsk, err)
	}

	ssize, err := pci.SealProof.SectorSize()
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to get resolve size: %w", err)
	}

	store := a.Chain.Store(ctx)

	var sectorWeight abi.StoragePower
	if act, err := state.GetActor(market.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor %s: %w", maddr, err)
	} else if s, err := market.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading market actor state %s: %w", maddr, err)
	} else if w, vw, err := s.VerifyDealsForActivation(maddr, pci.DealIDs, ts.Height(), pci.Expiration); err != nil {
		return types.EmptyInt, xerrors.Errorf("verifying deals for activation: %w", err)
	} else {
		// NB: not exactly accurate, but should always lead us to *over* estimate, not under
		duration := pci.Expiration - ts.Height()
		sectorWeight = builtin.QAPowerForWeight(ssize, duration, w, vw)
	}

	var (
		powerSmoothed    builtin.FilterEstimate
		pledgeCollateral abi.TokenAmount
	)
	if act, err := state.GetActor(power.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor: %w", err)
	} else if s, err := power.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading power actor state: %w", err)
	} else if p, err := s.TotalPowerSmoothed(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine total power: %w", err)
	} else if c, err := s.TotalLocked(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine pledge collateral: %w", err)
	} else {
		powerSmoothed = p
		pledgeCollateral = c
	}

	rewardActor, err := state.GetActor(reward.Address)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor: %w", err)
	}

	rewardState, err := reward.Load(store, rewardActor)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading reward actor state: %w", err)
	}

	circSupply, err := a.StateVMCirculatingSupplyInternal(ctx, ts.Key())
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting circulating supply: %w", err)
	}

	initialPledge, err := rewardState.InitialPledgeForPower(
		sectorWeight,
		pledgeCollateral,
		&powerSmoothed,
		circSupply.FilCirculating,
	)
	if err != nil {
		return big.Zero(), xerrors.Errorf("calculating initial pledge: %w", err)
	}

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen), nil
}

func (a *StateAPI) StateMinerAvailableBalance(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActor(ctx, maddr, ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	vested, err := mas.VestedFunds(ts.Height())
	if err != nil {
		return types.EmptyInt, err
	}

	abal, err := mas.AvailableBalance(act.Balance)
	if err != nil {
		return types.EmptyInt, err
	}

	return types.BigAdd(abal, vested), nil
}

// StateVerifiedClientStatus returns the data cap for the given address.
// Returns zero if there is no entry in the data cap table for the
// address.
func (a *StateAPI) StateVerifierStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	act, err := a.StateGetActor(ctx, verifreg.Address, tsk)
	if err != nil {
		return nil, err
	}

	aid, err := a.StateLookupID(ctx, addr, tsk)
	if err != nil {
		log.Warnf("lookup failure %v", err)
		return nil, err
	}

	vrs, err := verifreg.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load verified registry state: %w", err)
	}

	verified, dcap, err := vrs.VerifierDataCap(aid)
	if err != nil {
		return nil, xerrors.Errorf("looking up verifier: %w", err)
	}
	if !verified {
		return nil, nil
	}

	return &dcap, nil
}

// StateVerifiedClientStatus returns the data cap for the given address.
// Returns zero if there is no entry in the data cap table for the
// address.
func (a *StateAPI) StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	act, err := a.StateGetActor(ctx, verifreg.Address, tsk)
	if err != nil {
		return nil, err
	}

	aid, err := a.StateLookupID(ctx, addr, tsk)
	if err != nil {
		log.Warnf("lookup failure %v", err)
		return nil, err
	}

	vrs, err := verifreg.Load(a.StateManager.ChainStore().Store(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load verified registry state: %w", err)
	}

	verified, dcap, err := vrs.VerifiedClientDataCap(aid)
	if err != nil {
		return nil, xerrors.Errorf("looking up verified client: %w", err)
	}
	if !verified {
		return nil, nil
	}

	return &dcap, nil
}

func (a *StateAPI) StateVerifiedRegistryRootKey(ctx context.Context, tsk types.TipSetKey) (address.Address, error) {
	vact, err := a.StateGetActor(ctx, verifreg.Address, tsk)
	if err != nil {
		return address.Undef, err
	}

	vst, err := verifreg.Load(a.StateManager.ChainStore().Store(ctx), vact)
	if err != nil {
		return address.Undef, err
	}

	return vst.RootKey()
}

var dealProviderCollateralNum = types.NewInt(110)
var dealProviderCollateralDen = types.NewInt(100)

// StateDealProviderCollateralBounds returns the min and max collateral a storage provider
// can issue. It takes the deal size and verified status as parameters.
func (a *StateAPI) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	pact, err := a.StateGetActor(ctx, power.Address, tsk)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load power actor: %w", err)
	}

	ract, err := a.StateGetActor(ctx, reward.Address, tsk)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load reward actor: %w", err)
	}

	pst, err := power.Load(a.StateManager.ChainStore().Store(ctx), pact)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load power actor state: %w", err)
	}

	rst, err := reward.Load(a.StateManager.ChainStore().Store(ctx), ract)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load reward actor state: %w", err)
	}

	circ, err := a.StateVMCirculatingSupplyInternal(ctx, ts.Key())
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("getting total circulating supply: %w", err)
	}

	powClaim, err := pst.TotalPower()
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("getting total power: %w", err)
	}

	rewPow, err := rst.ThisEpochBaselinePower()
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("getting reward baseline power: %w", err)
	}

	min, max := policy.DealProviderCollateralBounds(size,
		verified,
		powClaim.RawBytePower,
		powClaim.QualityAdjPower,
		rewPow,
		circ.FilCirculating,
		a.StateManager.GetNtwkVersion(ctx, ts.Height()))
	return api.DealCollateralBounds{
		Min: types.BigDiv(types.BigMul(min, dealProviderCollateralNum), dealProviderCollateralDen),
		Max: max,
	}, nil
}

func (a *StateAPI) StateCirculatingSupply(ctx context.Context, tsk types.TipSetKey) (abi.TokenAmount, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	sTree, err := a.stateForTs(ctx, ts)
	if err != nil {
		return types.EmptyInt, err
	}
	return a.StateManager.GetCirculatingSupply(ctx, ts.Height(), sTree)
}

func (a *StateAPI) StateVMCirculatingSupplyInternal(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	sTree, err := a.stateForTs(ctx, ts)
	if err != nil {
		return api.CirculatingSupply{}, err
	}
	return a.StateManager.GetVMCirculatingSupplyDetailed(ctx, ts.Height(), sTree)
}

func (a *StateAPI) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return network.VersionMax, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return a.StateManager.GetNtwkVersion(ctx, ts.Height()), nil
}

func (a *StateAPI) StateMsgGasCost(ctx context.Context, inputMsg cid.Cid, tsk types.TipSetKey) (*api.MsgGasCost, error) {
	var msg cid.Cid
	var ts *types.TipSet
	var err error
	if tsk != types.EmptyTSK {
		msg = inputMsg
		ts, err = a.Chain.LoadTipSet(tsk)
		if err != nil {
			return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
		}
	} else {
		mlkp, err := a.StateSearchMsg(ctx, inputMsg)
		if err != nil {
			return nil, xerrors.Errorf("searching for msg %s: %w", inputMsg, err)
		}
		if mlkp == nil {
			return nil, xerrors.Errorf("didn't find msg %s", inputMsg)
		}

		executionTs, err := a.Chain.GetTipSetFromKey(mlkp.TipSet)
		if err != nil {
			return nil, xerrors.Errorf("loading tipset %s: %w", mlkp.TipSet, err)
		}

		ts, err = a.Chain.LoadTipSet(executionTs.Parents())
		if err != nil {
			return nil, xerrors.Errorf("loading parent tipset %s: %w", mlkp.TipSet, err)
		}

		msg = mlkp.Message
	}

	m, r, err := a.StateManager.Replay(ctx, ts, msg)
	if err != nil {
		return nil, err
	}

	return &api.MsgGasCost{
		Message:            msg,
		GasUsed:            big.NewInt(r.GasUsed),
		BaseFeeBurn:        r.GasCosts.BaseFeeBurn,
		OverEstimationBurn: r.GasCosts.OverEstimationBurn,
		MinerPenalty:       r.GasCosts.MinerPenalty,
		MinerTip:           r.GasCosts.MinerTip,
		Refund:             r.GasCosts.Refund,
		TotalCost:          big.Sub(m.RequiredFunds(), r.GasCosts.Refund),
	}, nil
}
