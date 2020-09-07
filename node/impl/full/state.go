package full

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"

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
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

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

var errBreakForeach = errors.New("break")

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

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address, filter *bitfield.BitField, filterOut bool, tsk types.TipSetKey) ([]*api.ChainSectorInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetMinerSectorSet(ctx, a.StateManager, ts, addr, filter, filterOut)
}

func (a *StateAPI) StateMinerActiveSectors(ctx context.Context, maddr address.Address, tsk types.TipSetKey) ([]*api.ChainSectorInfo, error) {
	var out []*api.ChainSectorInfo

	err := a.StateManager.WithParentStateTsk(tsk,
		a.StateManager.WithActor(maddr,
			a.StateManager.WithActorState(ctx, func(store adt.Store, mas *miner.State) error {
				var allActive []bitfield.BitField

				err := a.StateManager.WithDeadlines(
					a.StateManager.WithEachDeadline(
						a.StateManager.WithEachPartition(func(store adt.Store, partIdx uint64, partition *miner.Partition) error {
							active, err := partition.ActiveSectors()
							if err != nil {
								return xerrors.Errorf("partition.ActiveSectors: %w", err)
							}

							allActive = append(allActive, active)
							return nil
						})))(store, mas)
				if err != nil {
					return xerrors.Errorf("with deadlines: %w", err)
				}

				active, err := bitfield.MultiMerge(allActive...)
				if err != nil {
					return xerrors.Errorf("merging active sector bitfields: %w", err)
				}

				out, err = stmgr.LoadSectorsFromSet(ctx, a.Chain.Blockstore(), mas.Sectors, &active, false)
				return err
			})))
	if err != nil {
		return nil, xerrors.Errorf("getting active sectors from partitions: %w", err)
	}

	return out, nil
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

func (a *StateAPI) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) ([]*miner.Deadline, error) {
	var out []*miner.Deadline
	return out, a.StateManager.WithParentStateTsk(tsk,
		a.StateManager.WithActor(m,
			a.StateManager.WithActorState(ctx,
				a.StateManager.WithDeadlines(
					a.StateManager.WithEachDeadline(
						func(store adt.Store, idx uint64, deadline *miner.Deadline) error {
							out = append(out, deadline)
							return nil
						})))))
}

func (a *StateAPI) StateMinerPartitions(ctx context.Context, m address.Address, dlIdx uint64, tsk types.TipSetKey) ([]*miner.Partition, error) {
	var out []*miner.Partition
	return out, a.StateManager.WithParentStateTsk(tsk,
		a.StateManager.WithActor(m,
			a.StateManager.WithActorState(ctx,
				a.StateManager.WithDeadlines(
					a.StateManager.WithDeadline(dlIdx,
						a.StateManager.WithEachPartition(func(store adt.Store, partIdx uint64, partition *miner.Partition) error {
							out = append(out, partition)
							return nil
						}))))))
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

	return mas.DeadlineInfo(ts.Height()).NextNotElapsed(), nil
}

func (a *StateAPI) StateMinerFaults(ctx context.Context, addr address.Address, tsk types.TipSetKey) (bitfield.BitField, error) {
	out := bitfield.New()

	err := a.StateManager.WithParentStateTsk(tsk,
		a.StateManager.WithActor(addr,
			a.StateManager.WithActorState(ctx,
				a.StateManager.WithDeadlines(
					a.StateManager.WithEachDeadline(
						a.StateManager.WithEachPartition(func(store adt.Store, idx uint64, partition *miner.Partition) (err error) {
							out, err = bitfield.MergeBitFields(out, partition.Faults)
							return err
						}))))))
	if err != nil {
		return bitfield.BitField{}, err
	}

	return out, err
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
	out := bitfield.New()

	err := a.StateManager.WithParentStateTsk(tsk,
		a.StateManager.WithActor(addr,
			a.StateManager.WithActorState(ctx,
				a.StateManager.WithDeadlines(
					a.StateManager.WithEachDeadline(
						a.StateManager.WithEachPartition(func(store adt.Store, idx uint64, partition *miner.Partition) (err error) {
							out, err = bitfield.MergeBitFields(out, partition.Recoveries)
							return err
						}))))))
	if err != nil {
		return bitfield.BitField{}, err
	}

	return out, err
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

		if found, err := locked.Get(adt.AddrKey(a), &lk); err != nil {
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

		found, err := oh.Get(adt.AddrKey(addr), &ocval)
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
	var out api.MinerSectors

	err := a.StateManager.WithParentStateTsk(tsk,
		a.StateManager.WithActor(addr,
			a.StateManager.WithActorState(ctx, func(store adt.Store, mas *miner.State) error {
				var allActive []bitfield.BitField

				err := a.StateManager.WithDeadlines(
					a.StateManager.WithEachDeadline(
						a.StateManager.WithEachPartition(func(store adt.Store, partIdx uint64, partition *miner.Partition) error {
							active, err := partition.ActiveSectors()
							if err != nil {
								return xerrors.Errorf("partition.ActiveSectors: %w", err)
							}

							allActive = append(allActive, active)
							return nil
						})))(store, mas)
				if err != nil {
					return xerrors.Errorf("with deadlines: %w", err)
				}

				active, err := bitfield.MultiMerge(allActive...)
				if err != nil {
					return xerrors.Errorf("merging active sector bitfields: %w", err)
				}

				out.Active, err = active.Count()
				if err != nil {
					return xerrors.Errorf("counting active sectors: %w", err)
				}

				sarr, err := adt.AsArray(store, mas.Sectors)
				if err != nil {
					return err
				}

				out.Sectors = sarr.Length()
				return nil
			})))
	if err != nil {
		return api.MinerSectors{}, err
	}

	return out, nil
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

type sectorPartitionCb func(store adt.Store, mas *miner.State, di uint64, pi uint64, part *miner.Partition) error

func (a *StateAPI) sectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey, cb sectorPartitionCb) error {
	return a.StateManager.WithParentStateTsk(tsk,
		a.StateManager.WithActor(maddr,
			a.StateManager.WithActorState(ctx, func(store adt.Store, mas *miner.State) error {
				return a.StateManager.WithDeadlines(func(store adt.Store, deadlines *miner.Deadlines) error {
					err := a.StateManager.WithEachDeadline(func(store adt.Store, di uint64, deadline *miner.Deadline) error {
						return a.StateManager.WithEachPartition(func(store adt.Store, pi uint64, partition *miner.Partition) error {
							set, err := partition.Sectors.IsSet(uint64(sectorNumber))
							if err != nil {
								return xerrors.Errorf("is set: %w", err)
							}
							if set {
								if err := cb(store, mas, di, pi, partition); err != nil {
									return err
								}

								return errBreakForeach
							}
							return nil
						})(store, di, deadline)
					})(store, deadlines)
					if err == errBreakForeach {
						err = nil
					}
					return err
				})(store, mas)
			})))
}

func (a *StateAPI) StateSectorExpiration(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*api.SectorExpiration, error) {
	var onTimeEpoch, earlyEpoch abi.ChainEpoch

	err := a.sectorPartition(ctx, maddr, sectorNumber, tsk, func(store adt.Store, mas *miner.State, di uint64, pi uint64, part *miner.Partition) error {
		quant := mas.QuantSpecForDeadline(di)
		expirations, err := miner.LoadExpirationQueue(store, part.ExpirationsEpochs, quant)
		if err != nil {
			return xerrors.Errorf("loading expiration queue: %w", err)
		}

		var eset miner.ExpirationSet
		return expirations.Array.ForEach(&eset, func(epoch int64) error {
			set, err := eset.OnTimeSectors.IsSet(uint64(sectorNumber))
			if err != nil {
				return xerrors.Errorf("checking if sector is in onTime set: %w", err)
			}
			if set {
				onTimeEpoch = abi.ChainEpoch(epoch)
			}

			set, err = eset.EarlySectors.IsSet(uint64(sectorNumber))
			if err != nil {
				return xerrors.Errorf("checking if sector is in early set: %w", err)
			}
			if set {
				earlyEpoch = abi.ChainEpoch(epoch)
			}

			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	if onTimeEpoch == 0 {
		return nil, xerrors.Errorf("expiration for sector %d not found", sectorNumber)
	}

	return &api.SectorExpiration{
		OnTime: onTimeEpoch,
		Early:  earlyEpoch,
	}, nil
}

func (a *StateAPI) StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*api.SectorLocation, error) {
	var found *api.SectorLocation

	err := a.sectorPartition(ctx, maddr, sectorNumber, tsk, func(store adt.Store, mas *miner.State, di, pi uint64, partition *miner.Partition) error {
		found = &api.SectorLocation{
			Deadline:  di,
			Partition: pi,
		}
		return errBreakForeach
	})
	if err != nil {
		return nil, err
	}

	return found, nil
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

var initialPledgeNum = types.NewInt(110)
var initialPledgeDen = types.NewInt(100)

func (a *StateAPI) StateMinerPreCommitDepositForPower(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	var minerState miner.State
	var powerState power.State
	var rewardState reward.State

	err = a.StateManager.WithParentStateTsk(tsk, func(state *state.StateTree) error {
		if err := a.StateManager.WithActor(maddr, a.StateManager.WithActorState(ctx, &minerState))(state); err != nil {
			return xerrors.Errorf("getting miner state: %w", err)
		}

		if err := a.StateManager.WithActor(builtin.StoragePowerActorAddr, a.StateManager.WithActorState(ctx, &powerState))(state); err != nil {
			return xerrors.Errorf("getting power state: %w", err)
		}

		if err := a.StateManager.WithActor(builtin.RewardActorAddr, a.StateManager.WithActorState(ctx, &rewardState))(state); err != nil {
			return xerrors.Errorf("getting reward state: %w", err)
		}

		return nil
	})
	if err != nil {
		return types.EmptyInt, err
	}

	dealWeights := market.VerifyDealsForActivationReturn{
		DealWeight:         big.Zero(),
		VerifiedDealWeight: big.Zero(),
	}

	if len(pci.DealIDs) != 0 {
		var err error
		params, err := actors.SerializeParams(&market.VerifyDealsForActivationParams{
			DealIDs:      pci.DealIDs,
			SectorExpiry: pci.Expiration,
		})
		if err != nil {
			return types.EmptyInt, err
		}

		ret, err := a.StateManager.Call(ctx, &types.Message{
			From:   maddr,
			To:     builtin.StorageMarketActorAddr,
			Method: builtin.MethodsMarket.VerifyDealsForActivation,
			Params: params,
		}, ts)
		if err != nil {
			return types.EmptyInt, err
		}

		if err := dealWeights.UnmarshalCBOR(bytes.NewReader(ret.MsgRct.Return)); err != nil {
			return types.BigInt{}, err
		}
	}

	mi, err := a.StateMinerInfo(ctx, maddr, tsk)
	if err != nil {
		return types.EmptyInt, err
	}

	ssize := mi.SectorSize

	duration := pci.Expiration - ts.Height() // NB: not exactly accurate, but should always lead us to *over* estimate, not under

	sectorWeight := miner.QAPowerForWeight(ssize, duration, dealWeights.DealWeight, dealWeights.VerifiedDealWeight)
	deposit := miner.PreCommitDepositForPower(
		rewardState.ThisEpochRewardSmoothed,
		powerState.ThisEpochQAPowerSmoothed,
		sectorWeight,
	)

	return types.BigDiv(types.BigMul(deposit, initialPledgeNum), initialPledgeDen), nil
}

func (a *StateAPI) StateMinerInitialPledgeCollateral(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	var minerState miner.State
	var powerState power.State
	var rewardState reward.State

	err = a.StateManager.WithParentStateTsk(tsk, func(state *state.StateTree) error {
		if err := a.StateManager.WithActor(maddr, a.StateManager.WithActorState(ctx, &minerState))(state); err != nil {
			return xerrors.Errorf("getting miner state: %w", err)
		}

		if err := a.StateManager.WithActor(builtin.StoragePowerActorAddr, a.StateManager.WithActorState(ctx, &powerState))(state); err != nil {
			return xerrors.Errorf("getting power state: %w", err)
		}

		if err := a.StateManager.WithActor(builtin.RewardActorAddr, a.StateManager.WithActorState(ctx, &rewardState))(state); err != nil {
			return xerrors.Errorf("getting reward state: %w", err)
		}

		return nil
	})
	if err != nil {
		return types.EmptyInt, err
	}

	dealWeights := market.VerifyDealsForActivationReturn{
		DealWeight:         big.Zero(),
		VerifiedDealWeight: big.Zero(),
	}

	if len(pci.DealIDs) != 0 {
		var err error
		params, err := actors.SerializeParams(&market.VerifyDealsForActivationParams{
			DealIDs:      pci.DealIDs,
			SectorExpiry: pci.Expiration,
		})
		if err != nil {
			return types.EmptyInt, err
		}

		ret, err := a.StateManager.Call(ctx, &types.Message{
			From:   maddr,
			To:     builtin.StorageMarketActorAddr,
			Method: builtin.MethodsMarket.VerifyDealsForActivation,
			Params: params,
		}, ts)
		if err != nil {
			return types.EmptyInt, err
		}

		if err := dealWeights.UnmarshalCBOR(bytes.NewReader(ret.MsgRct.Return)); err != nil {
			return types.BigInt{}, err
		}
	}

	mi, err := a.StateMinerInfo(ctx, maddr, tsk)
	if err != nil {
		return types.EmptyInt, err
	}

	ssize := mi.SectorSize

	duration := pci.Expiration - ts.Height() // NB: not exactly accurate, but should always lead us to *over* estimate, not under

	circSupply, err := a.StateCirculatingSupply(ctx, ts.Key())
	if err != nil {
		return big.Zero(), xerrors.Errorf("getting circulating supply: %w", err)
	}

	sectorWeight := miner.QAPowerForWeight(ssize, duration, dealWeights.DealWeight, dealWeights.VerifiedDealWeight)
	initialPledge := miner.InitialPledgeForPower(
		sectorWeight,
		rewardState.ThisEpochBaselinePower,
		powerState.ThisEpochPledgeCollateral,
		rewardState.ThisEpochRewardSmoothed,
		powerState.ThisEpochQAPowerSmoothed,
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
	if found, err := vh.Get(adt.AddrKey(aid), &dcap); err != nil {
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

	st, _, err := a.StateManager.TipSetState(ctx, ts)
	if err != nil {
		return api.CirculatingSupply{}, err
	}

	cst := cbor.NewCborStore(a.Chain.Blockstore())
	sTree, err := state.LoadStateTree(cst, st)
	if err != nil {
		return api.CirculatingSupply{}, err
	}

	return a.StateManager.GetCirculatingSupplyDetailed(ctx, ts.Height(), sTree)
}
