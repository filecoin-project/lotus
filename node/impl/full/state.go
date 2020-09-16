package full

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-state-types/dline"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	v0miner "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/filecoin-project/specs-actors/actors/util/smoothing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
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
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
)

var errBreakForeach = errors.New("break")

type StateAPI struct {
	fx.In

	// TODO: the wallet here is only needed because we have the MinerCreateBlock
	// API attached to the state API. It probably should live somewhere better
	Wallet *wallet.Wallet

	ProofVerifier ffiwrapper.Verifier
	StateManager  *stmgr.StateManager
	Chain         *store.ChainStore
	Beacon        beacon.Schedule
}

func (a *StateAPI) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	return stmgr.GetNetworkName(ctx, a.StateManager, a.Chain.GetHeaviestTipSet().ParentState())
}

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address, filter *bitfield.BitField, filterOut bool, tsk types.TipSetKey) ([]*miner.ChainSectorInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerSectorSet(ctx, a.StateManager, ts, addr, filter, filterOut)
}

func (a *StateAPI) StateMinerActiveSectors(ctx context.Context, maddr address.Address, tsk types.TipSetKey) ([]*miner.ChainSectorInfo, error) { // TODO: only used in cli
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

	return mas.LoadSectorsFromSet(&activeSectors, false)
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

func (a *StateAPI) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) ([]miner.Deadline, error) {
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

	out := make([]miner.Deadline, deadlines)
	if err := mas.ForEachDeadline(func(i uint64, dl miner.Deadline) error {
		out[i] = dl
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *StateAPI) StateMinerPartitions(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]*miner.Partition, error) {
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

	var out []*miner.Partition
	err = dl.ForEachPartition(func(_ uint64, part miner.Partition) error {
		p := part
		out = append(out, &p)
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

	return mas.DeadlineInfo(ts.Height()).NextNotElapsed(), nil
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

	m, net, err := stmgr.GetPower(ctx, a.StateManager, ts, addr)
	if err != nil {
		return nil, err
	}

	return &api.MinerPower{
		MinerPower: m,
		TotalPower: net,
	}, nil
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

	oif, err := vm.DumpActorState(act.Code, blk.RawData())
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

func (a *StateAPI) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	ts, recpt, found, err := a.StateManager.WaitForMessage(ctx, msg, confidence)
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

	var state market.State
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	if _, err := a.StateManager.LoadActorState(ctx, builtin.StorageMarketActorAddr, &state, ts); err != nil {
		return nil, err
	}
	store := a.StateManager.ChainStore().Store(ctx)
	escrow, err := adt.AsMap(store, state.EscrowTable)
	if err != nil {
		return nil, err
	}
	locked, err := adt.AsMap(store, state.LockedTable)
	if err != nil {
		return nil, err
	}

	var es, lk abi.TokenAmount
	err = escrow.ForEach(&es, func(k string) error {
		a, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return err
		}

		if found, err := locked.Get(abi.AddrKey(a), &lk); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("locked funds not found")
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

	store := a.StateManager.ChainStore().Store(ctx)
	da, err := adt.AsArray(store, state.Proposals)
	if err != nil {
		return nil, err
	}

	sa, err := adt.AsArray(store, state.States)
	if err != nil {
		return nil, err
	}

	var d market.DealProposal
	if err := da.ForEach(&d, func(i int64) error {
		var s market.DealState
		if found, err := sa.Get(uint64(i), &s); err != nil {
			return xerrors.Errorf("failed to get state for deal in proposals array: %w", err)
		} else if !found {
			s.SectorStartEpoch = -1
		}
		out[strconv.FormatInt(i, 10)] = api.MarketDeal{
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
	store := adt.WrapStore(ctx, cbor.NewCborStore(a.Chain.Blockstore()))

	nh, err := adt.AsMap(store, new)
	if err != nil {
		return nil, err
	}

	oh, err := adt.AsMap(store, old)
	if err != nil {
		return nil, err
	}

	out := map[string]types.Actor{}

	var (
		ncval, ocval cbg.Deferred
		buf          = bytes.NewReader(nil)
	)
	err = nh.ForEach(&ncval, func(k string) error {
		var act types.Actor

		addr, err := address.NewFromBytes([]byte(k))
		if err != nil {
			return xerrors.Errorf("address in state tree was not valid: %w", err)
		}

		found, err := oh.Get(abi.AddrKey(addr), &ocval)
		if err != nil {
			return err
		}

		if found && bytes.Equal(ocval.Raw, ncval.Raw) {
			return nil // not changed
		}

		buf.Reset(ncval.Raw)
		err = act.UnmarshalCBOR(buf)
		buf.Reset(nil)

		if err != nil {
			return err
		}

		out[addr.String()] = act

		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, nil
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

func (a *StateAPI) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
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

func (a *StateAPI) MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor: %w", err)
	}
	msas, err := multisig.Load(a.Chain.Store(ctx), act)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor state: %w", err)
	}
	locked, err := msas.LockedBalance(ts.Height())
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to compute locked multisig balance: %w", err)
	}
	return types.BigSub(act.Balance, locked), nil
}

func (a *StateAPI) MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error) {
	startTs, err := a.Chain.GetTipSetFromKey(start)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading start tipset %s: %w", start, err)
	}

	endTs, err := a.Chain.GetTipSetFromKey(end)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading end tipset %s: %w", end, err)
	}

	if startTs.Height() > endTs.Height() {
		return types.EmptyInt, xerrors.Errorf("start tipset %d is after end tipset %d", startTs.Height(), endTs.Height())
	} else if startTs.Height() == endTs.Height() {
		return big.Zero(), nil
	}

	act, err := a.StateManager.LoadActor(ctx, addr, endTs)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor at end epoch: %w", err)
	}

	msas, err := multisig.Load(a.Chain.Store(ctx), act)
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
		return types.EmptyInt, xerrors.Errorf("loading miner actor %s: %w", maddr, err)
	} else s, err := market.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading market actor state %s: %w", maddr, err)
	} else if w, vw, err := s.VerifyDealsForActivation(maddr, pci.DealIDs, ts.Height(), pci.Expiration); err != nil {
			return types.EmptyInt, xerrors.Errorf("verifying deals for activation: %w", err)
	} else {
		// NB: not exactly accurate, but should always lead us to *over* estimate, not under
		duration := pci.Expiration - ts.Height()

		// TODO: handle changes to this function across actor upgrades.
		sectorWeight = v0miner.QAPowerForWeight(ssize, duration, w, vw)
	}

	var powerSmoothed smoothing.FilterEstimate
	if act, err := state.GetActor(power.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor: %w", err)
	} else s, err := power.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading power actor state: %w", err)
	} else p, err := s.TotalPowerSmoothed(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine total power: %w", err)
	} else {
		powerSmoothed = p
	}

	var rewardSmoothed smoothing.FilterEstimate
	if act, err := state.GetActor(reward.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor: %w", err)
	} else s, err := reward.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading reward actor state: %w", err)
	} else r, err := s.RewardSmoothed(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine total reward: %w", err)
	} else {
		rewardSmoothed = r
	}

	// TODO: abstract over network upgrades.
	deposit := v0miner.PreCommitDepositForPower(rewardSmoothed, powerSmoothed, sectorWeight)

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
	} else s, err := market.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading market actor state %s: %w", maddr, err)
	} else if w, vw, err := s.VerifyDealsForActivation(maddr, pci.DealIDs, ts.Height(), pci.Expiration); err != nil {
			return types.EmptyInt, xerrors.Errorf("verifying deals for activation: %w", err)
	} else {
		// NB: not exactly accurate, but should always lead us to *over* estimate, not under
		duration := pci.Expiration - ts.Height()

		// TODO: handle changes to this function across actor upgrades.
		sectorWeight = v0miner.QAPowerForWeight(ssize, duration, w, vw)
	}

	var (
		powerSmoothed smoothing.FilterEstimate
		pledgeCollerateral abi.TokenAmount
	)
	if act, err := state.GetActor(power.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor: %w", err)
	} else s, err := power.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading power actor state: %w", err)
	} else p, err := s.TotalPowerSmoothed(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine total power: %w", err)
	} else c, err := s.TotalLocked(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine pledge collateral: %w", err)
	} else {
		powerSmoothed = p
		pledgeCollateral = c
	}

	var (
		rewardSmoothed smoothing.FilterEstimate
		baselinePower abi.StoragePower
	)
	if act, err := state.GetActor(reward.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading miner actor: %w", err)
	} else s, err := reward.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading reward actor state: %w", err)
	} else r, err := s.RewardSmoothed(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine total reward: %w", err)
	} else p, err := s.BaselinePower(); err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to determine baseline power: %w", err)
	} else {
		rewardSmoothed = r
		baselinePower = p
	}

	// TODO: abstract over network upgrades.

	circSupply, err := a.StateCirculatingSupply(ctx, ts.Key())
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting circulating supply: %w", err)
	}

	initialPledge := miner.InitialPledgeForPower(
		sectorWeight,
		baselinePower,
		pledgeCollateral,
		rewardSmoothed,
		powerSmoothed,
		circSupply.FilCirculating,
	)

	return types.BigDiv(types.BigMul(initialPledge, initialPledgeNum), initialPledgeDen), nil
}

func (a *StateAPI) StateMinerAvailableBalance(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	var act *types.Actor
	var mas miner.State

	if err := a.StateManager.WithParentState(ts, a.StateManager.WithActor(maddr, func(actor *types.Actor) error {
		act = actor
		return a.StateManager.WithActorState(ctx, &mas)(actor)
	})); err != nil {
		return types.BigInt{}, xerrors.Errorf("getting miner state: %w", err)
	}
	as := store.ActorStore(ctx, a.Chain.Blockstore())

	vested, err := mas.CheckVestedFunds(as, ts.Height())
	if err != nil {
		return types.EmptyInt, err
	}

	return types.BigAdd(mas.GetAvailableBalance(act.Balance), vested), nil
}

// StateVerifiedClientStatus returns the data cap for the given address.
// Returns nil if there is no entry in the data cap table for the
// address.
func (a *StateAPI) StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*verifreg.DataCap, error) {
	act, err := a.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, tsk)
	if err != nil {
		return nil, err
	}

	aid, err := a.StateLookupID(ctx, addr, tsk)
	if err != nil {
		log.Warnf("lookup failure %v", err)
		return nil, err
	}

	store := a.StateManager.ChainStore().Store(ctx)

	var st verifreg.State
	if err := store.Get(ctx, act.Head, &st); err != nil {
		return nil, err
	}

	vh, err := adt.AsMap(store, st.VerifiedClients)
	if err != nil {
		return nil, err
	}

	var dcap verifreg.DataCap
	if found, err := vh.Get(abi.AddrKey(aid), &dcap); err != nil {
		return nil, err
	} else if !found {
		return nil, nil
	}

	return &dcap, nil
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

	var powerState power.State
	var rewardState reward.State

	err = a.StateManager.WithParentStateTsk(ts.Key(), func(state *state.StateTree) error {
		if err := a.StateManager.WithActor(builtin.StoragePowerActorAddr, a.StateManager.WithActorState(ctx, &powerState))(state); err != nil {
			return xerrors.Errorf("getting power state: %w", err)
		}

		if err := a.StateManager.WithActor(builtin.RewardActorAddr, a.StateManager.WithActorState(ctx, &rewardState))(state); err != nil {
			return xerrors.Errorf("getting reward state: %w", err)
		}

		return nil
	})

	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("getting power and reward actor states: %w", err)
	}

	circ, err := a.StateCirculatingSupply(ctx, ts.Key())
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("getting total circulating supply: %w", err)
	}

	min, max := market.DealProviderCollateralBounds(size,
		verified,
		powerState.TotalRawBytePower,
		powerState.ThisEpochQualityAdjPower,
		rewardState.ThisEpochBaselinePower,
		circ.FilCirculating,
		a.StateManager.GetNtwkVersion(ctx, ts.Height()))
	return api.DealCollateralBounds{
		Min: types.BigDiv(types.BigMul(min, dealProviderCollateralNum), dealProviderCollateralDen),
		Max: max,
	}, nil
}

func (a *StateAPI) StateCirculatingSupply(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	sTree, err := a.stateForTs(ctx, ts)
	if err != nil {
		return api.CirculatingSupply{}, err
	}
	return a.StateManager.GetCirculatingSupplyDetailed(ctx, ts.Height(), sTree)
}
