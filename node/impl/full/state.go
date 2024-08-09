package full

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	market12 "github.com/filecoin-project/go-state-types/builtin/v12/market"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	market5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/market"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type StateModuleAPI interface {
	MsigGetAvailableBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (types.BigInt, error)
	MsigGetVested(ctx context.Context, addr address.Address, start types.TipSetKey, end types.TipSetKey) (types.BigInt, error)
	MsigGetPending(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.MsigTransaction, error)
	StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error)
	StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error)
	StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error)
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
	StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error)
	StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error)
	StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (api.MinerInfo, error)
	StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*dline.Info, error)
	StateMinerPower(context.Context, address.Address, types.TipSetKey) (*api.MinerPower, error)
	StateNetworkVersion(ctx context.Context, key types.TipSetKey) (network.Version, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error)
	StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error)
	StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
}

var _ StateModuleAPI = *new(api.FullNode)

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
	Wallet    api.Wallet
	DefWallet wallet.Default

	StateModuleAPI

	ProofVerifier proofs.Verifier
	StateManager  *stmgr.StateManager
	Chain         *store.ChainStore
	Beacon        beacon.Schedule
	Consensus     consensus.Consensus
	TsExec        stmgr.Executor
}

func (a *StateAPI) StateNetworkName(ctx context.Context) (dtypes.NetworkName, error) {
	return stmgr.GetNetworkName(ctx, a.StateManager, a.Chain.GetHeaviestTipSet().ParentState())
}

func (a *StateAPI) StateMinerSectors(ctx context.Context, addr address.Address, sectorNos *bitfield.BitField, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, addr, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	return mas.LoadSectors(sectorNos)
}

func (a *StateAPI) StateMinerActiveSectors(ctx context.Context, maddr address.Address, tsk types.TipSetKey) ([]*miner.SectorOnChainInfo, error) { // TODO: only used in cli
	act, err := a.StateManager.LoadActorTsk(ctx, maddr, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	activeSectors, err := miner.AllPartSectors(mas, miner.Partition.ActiveSectors)
	if err != nil {
		return nil, xerrors.Errorf("merge partition active sets: %w", err)
	}

	return mas.LoadSectors(&activeSectors)
}

func (m *StateModule) StateMinerInfo(ctx context.Context, actor address.Address, tsk types.TipSetKey) (api.MinerInfo, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return api.MinerInfo{}, xerrors.Errorf("failed to load tipset: %w", err)
	}

	act, err := m.StateManager.LoadActor(ctx, actor, ts)
	if err != nil {
		return api.MinerInfo{}, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(m.StateManager.ChainStore().ActorStore(ctx), act)
	if err != nil {
		return api.MinerInfo{}, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	info, err := mas.Info()
	if err != nil {
		return api.MinerInfo{}, err
	}

	var pid *peer.ID
	if peerID, err := peer.IDFromBytes(info.PeerId); err == nil {
		pid = &peerID
	}

	ret := api.MinerInfo{
		Owner:                      info.Owner,
		Worker:                     info.Worker,
		ControlAddresses:           info.ControlAddresses,
		NewWorker:                  address.Undef,
		WorkerChangeEpoch:          -1,
		PeerId:                     pid,
		Multiaddrs:                 info.Multiaddrs,
		WindowPoStProofType:        info.WindowPoStProofType,
		SectorSize:                 info.SectorSize,
		WindowPoStPartitionSectors: info.WindowPoStPartitionSectors,
		ConsensusFaultElapsed:      info.ConsensusFaultElapsed,
		PendingOwnerAddress:        info.PendingOwnerAddress,
		Beneficiary:                info.Beneficiary,
		BeneficiaryTerm:            &info.BeneficiaryTerm,
		PendingBeneficiaryTerm:     info.PendingBeneficiaryTerm,
	}

	if info.PendingWorkerKey != nil {
		ret.NewWorker = info.PendingWorkerKey.NewWorker
		ret.WorkerChangeEpoch = info.PendingWorkerKey.EffectiveAt
	}

	return ret, nil
}

func (a *StateAPI) StateMinerDeadlines(ctx context.Context, m address.Address, tsk types.TipSetKey) ([]api.Deadline, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, m, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	deadlines, err := mas.NumDeadlines()
	if err != nil {
		return nil, xerrors.Errorf("getting deadline count: %w", err)
	}

	out := make([]api.Deadline, deadlines)
	if err := mas.ForEachDeadline(func(i uint64, dl miner.Deadline) error {
		ps, err := dl.PartitionsPoSted()
		if err != nil {
			return err
		}

		l, err := dl.DisputableProofCount()
		if err != nil {
			return err
		}

		out[i] = api.Deadline{
			PostSubmissions:      ps,
			DisputableProofCount: l,
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

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
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

func (m *StateModule) StateMinerProvingDeadline(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*dline.Info, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := m.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(m.StateManager.ChainStore().ActorStore(ctx), act)
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

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
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

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
	if err != nil {
		return bitfield.BitField{}, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	return miner.AllPartSectors(mas, miner.Partition.RecoveringSectors)
}

func (m *StateModule) StateMinerPower(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*api.MinerPower, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	mp, net, hmp, err := stmgr.GetPower(ctx, m.StateManager, ts, addr)
	if err != nil {
		return nil, err
	}

	return &api.MinerPower{
		MinerPower:  mp,
		TotalPower:  net,
		HasMinPower: hmp,
	}, nil
}

func (a *StateAPI) StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (res *api.InvocResult, err error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	for {
		res, err = a.StateManager.Call(ctx, msg, ts)
		if err != stmgr.ErrExpensiveFork {
			break
		}
		ts, err = a.Chain.GetTipSetFromKey(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("getting parent tipset: %w", err)
		}
	}
	return res, err
}

func (a *StateAPI) StateReplay(ctx context.Context, tsk types.TipSetKey, mc cid.Cid) (*api.InvocResult, error) {
	msgToReplay := mc
	var ts *types.TipSet
	var err error
	if tsk == types.EmptyTSK {
		mlkp, err := a.StateSearchMsg(ctx, types.EmptyTSK, mc, stmgr.LookbackNoLimit, true)
		if err != nil {
			return nil, xerrors.Errorf("searching for msg %s: %w", mc, err)
		}
		if mlkp == nil {
			return nil, xerrors.Errorf("didn't find msg %s", mc)
		}

		msgToReplay = mlkp.Message

		executionTs, err := a.Chain.GetTipSetFromKey(ctx, mlkp.TipSet)
		if err != nil {
			return nil, xerrors.Errorf("loading tipset %s: %w", mlkp.TipSet, err)
		}

		ts, err = a.Chain.LoadTipSet(ctx, executionTs.Parents())
		if err != nil {
			return nil, xerrors.Errorf("loading parent tipset %s: %w", mlkp.TipSet, err)
		}
	} else {
		ts, err = a.Chain.LoadTipSet(ctx, tsk)
		if err != nil {
			return nil, xerrors.Errorf("loading specified tipset %s: %w", tsk, err)
		}
	}

	m, r, err := a.StateManager.Replay(ctx, ts, msgToReplay)
	if err != nil {
		return nil, err
	}

	var errstr string
	if r.ActorErr != nil {
		errstr = r.ActorErr.Error()
	}

	return &api.InvocResult{
		MsgCid:         msgToReplay,
		Msg:            m,
		MsgRct:         &r.MessageReceipt,
		GasCost:        stmgr.MakeMsgGasCost(m, r),
		ExecutionTrace: r.ExecutionTrace,
		Error:          errstr,
		Duration:       r.Duration,
	}, nil
}

func (m *StateModule) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (a *types.Actor, err error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return m.StateManager.LoadActor(ctx, actor, ts)
}

func (m *StateModule) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	ret, err := m.StateManager.LookupIDAddress(ctx, addr, ts)
	if err != nil && xerrors.Is(err, types.ErrActorNotFound) {
		return address.Undef, &api.ErrActorNotFound{}
	}

	return ret, err
}

func (a *StateAPI) StateLookupRobustAddress(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return a.StateManager.LookupRobustAddress(ctx, addr, ts)
}

func (m *StateModule) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return address.Undef, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	return m.StateManager.ResolveToDeterministicAddress(ctx, addr, ts)
}

func (a *StateAPI) StateReadState(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*api.ActorState, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	act, err := a.StateManager.LoadActor(ctx, actor, ts)
	if err != nil {
		return nil, xerrors.Errorf("getting actor: %w", err)
	}

	blk, err := a.Chain.StateBlockstore().Get(ctx, act.Head)
	if err != nil {
		return nil, xerrors.Errorf("getting actor head: %w", err)
	}

	oif, err := vm.DumpActorState(a.TsExec.NewActorRegistry(), act, blk.RawData())
	if err != nil {
		return nil, xerrors.Errorf("dumping actor state (a:%s): %w", actor, err)
	}

	return &api.ActorState{
		Balance: act.Balance,
		Code:    act.Code,
		State:   oif,
	}, nil
}

func (a *StateAPI) StateDecodeParams(ctx context.Context, toAddr address.Address, method abi.MethodNum, params []byte, tsk types.TipSetKey) (interface{}, error) {
	act, err := a.StateGetActor(ctx, toAddr, tsk)
	if err != nil {
		return nil, xerrors.Errorf("getting actor: %w", err)
	}

	paramType, err := stmgr.GetParamType(a.TsExec.NewActorRegistry(), act.Code, method)
	if err != nil {
		return nil, xerrors.Errorf("getting params type: %w", err)
	}

	if err = paramType.UnmarshalCBOR(bytes.NewReader(params)); err != nil {
		return nil, err
	}

	return paramType, nil
}

func (a *StateAPI) StateEncodeParams(ctx context.Context, toActCode cid.Cid, method abi.MethodNum, params json.RawMessage) ([]byte, error) {
	paramType, err := stmgr.GetParamType(a.TsExec.NewActorRegistry(), toActCode, method)
	if err != nil {
		return nil, xerrors.Errorf("getting params type: %w", err)
	}

	if err := json.Unmarshal(params, &paramType); err != nil {
		return nil, xerrors.Errorf("json unmarshal: %w", err)
	}

	var cbb bytes.Buffer
	if err := paramType.(cbor.Marshaler).MarshalCBOR(&cbb); err != nil {
		return nil, xerrors.Errorf("cbor marshal: %w", err)
	}

	return cbb.Bytes(), nil
}

// This is on StateAPI because miner.Miner requires this, and MinerAPI requires miner.Miner

func (a *StateAPI) MinerGetBaseInfo(ctx context.Context, maddr address.Address, epoch abi.ChainEpoch, tsk types.TipSetKey) (*api.MiningBaseInfo, error) {
	// XXX: Gets the state by computing the tipset state, instead of looking at the parent.
	return stmgr.MinerGetBaseInfo(ctx, a.StateManager, a.Beacon, tsk, epoch, maddr, a.ProofVerifier)
}

func (a *StateAPI) MinerCreateBlock(ctx context.Context, bt *api.BlockTemplate) (*types.BlockMsg, error) {
	fblk, err := a.Consensus.CreateBlock(ctx, a.Wallet, bt)
	if err != nil {
		return nil, err
	}
	if fblk == nil {
		return nil, nil
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

func (m *StateModule) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64, lookbackLimit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	ts, recpt, found, err := m.StateManager.WaitForMessage(ctx, msg, confidence, lookbackLimit, allowReplaced)
	if err != nil {
		return nil, err
	}

	var returndec interface{}
	if recpt.ExitCode == 0 && len(recpt.Return) > 0 {
		cmsg, err := m.Chain.GetCMessage(ctx, msg)
		if err != nil {
			return nil, xerrors.Errorf("failed to load message after successful receipt search: %w", err)
		}

		vmsg := cmsg.VMMessage()

		switch t, err := stmgr.GetReturnType(ctx, m.StateManager, vmsg.To, vmsg.Method, ts); {
		case errors.Is(err, stmgr.ErrMetadataNotFound):
			// This is not necessarily an error -- EVM methods (and in the future native actors) may
			// return just bytes, and in the not so distant future we'll have native wasm actors
			// that are by definition not in the registry.
			// So in this case, log a debug message and retun the raw bytes.
			log.Debugf("failed to get return type: %s", err)
			returndec = recpt.Return
		case err != nil:
			return nil, xerrors.Errorf("failed to get return type: %w", err)
		default:
			if err := t.UnmarshalCBOR(bytes.NewReader(recpt.Return)); err != nil {
				return nil, err
			}
			returndec = t
		}
	}

	return &api.MsgLookup{
		Message:   found,
		Receipt:   *recpt,
		ReturnDec: returndec,
		TipSet:    ts.Key(),
		Height:    ts.Height(),
	}, nil
}

func (m *StateModule) StateSearchMsg(ctx context.Context, tsk types.TipSetKey, msg cid.Cid, lookbackLimit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	fromTs, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	ts, recpt, found, err := m.StateManager.SearchForMessage(ctx, fromTs, msg, lookbackLimit, allowReplaced)
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

func (m *StateModule) StateListMiners(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.ListMinerActors(ctx, m.StateManager, ts)
}

func (a *StateAPI) StateListActors(ctx context.Context, tsk types.TipSetKey) ([]address.Address, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return a.StateManager.ListAllActors(ctx, ts)
}

func (m *StateModule) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return api.MarketBalance{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return m.StateManager.MarketBalance(ctx, addr, ts)
}

func (a *StateAPI) StateMarketParticipants(ctx context.Context, tsk types.TipSetKey) (map[string]api.MarketBalance, error) {
	out := map[string]api.MarketBalance{}

	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
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

func (a *StateAPI) StateMarketDeals(ctx context.Context, tsk types.TipSetKey) (map[string]*api.MarketDeal, error) {
	out := map[string]*api.MarketDeal{}

	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
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
		out[strconv.FormatInt(int64(dealID), 10)] = &api.MarketDeal{
			Proposal: d,
			State:    api.MakeDealState(s),
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *StateModule) StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.GetStorageDeal(ctx, m.StateManager, dealId, ts)
}

func (a *StateAPI) StateGetAllocationIdForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (verifreg.AllocationId, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return verifreg.NoAllocationID, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	st, err := a.StateManager.GetMarketState(ctx, ts)
	if err != nil {
		return verifreg.NoAllocationID, err
	}

	allocationId, err := st.GetAllocationIdForPendingDeal(dealId)
	if err != nil {
		return verifreg.NoAllocationID, err
	}

	return allocationId, nil
}

func (a *StateAPI) StateGetAllocationForPendingDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*verifreg.Allocation, error) {
	allocationId, err := a.StateGetAllocationIdForPendingDeal(ctx, dealId, tsk)
	if err != nil {
		return nil, err
	}
	if allocationId == verifreg.NoAllocationID {
		return nil, nil
	}

	dealState, err := a.StateMarketStorageDeal(ctx, dealId, tsk)
	if err != nil {
		return nil, err
	}

	return a.StateGetAllocation(ctx, dealState.Proposal.Client, allocationId, tsk)
}

func (a *StateAPI) StateGetAllocation(ctx context.Context, clientAddr address.Address, allocationId verifreg.AllocationId, tsk types.TipSetKey) (*verifreg.Allocation, error) {
	idAddr, err := a.StateLookupID(ctx, clientAddr, tsk)
	if err != nil {
		return nil, err
	}

	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	st, err := a.StateManager.GetVerifregState(ctx, ts)
	if err != nil {
		return nil, err
	}

	allocation, found, err := st.GetAllocation(idAddr, allocationId)
	if err != nil {
		return nil, xerrors.Errorf("getting allocation: %w", err)
	}
	if !found {
		return nil, nil
	}

	return allocation, nil
}

func (a *StateAPI) StateGetAllocations(ctx context.Context, clientAddr address.Address, tsk types.TipSetKey) (map[verifreg.AllocationId]verifreg.Allocation, error) {
	idAddr, err := a.StateLookupID(ctx, clientAddr, tsk)
	if err != nil {
		return nil, err
	}

	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	st, err := a.StateManager.GetVerifregState(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("loading verifreg state: %w", err)
	}

	allocations, err := st.GetAllocations(idAddr)
	if err != nil {
		return nil, xerrors.Errorf("getting allocations: %w", err)
	}

	return allocations, nil
}

func (a *StateAPI) StateGetAllAllocations(ctx context.Context, tsk types.TipSetKey) (map[verifreg.AllocationId]verifreg.Allocation, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	st, err := a.StateManager.GetVerifregState(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("loading verifreg state: %w", err)
	}

	allocations, err := st.GetAllAllocations()
	if err != nil {
		return nil, xerrors.Errorf("getting all allocations: %w", err)
	}

	return allocations, nil
}

func (a *StateAPI) StateGetClaim(ctx context.Context, providerAddr address.Address, claimId verifreg.ClaimId, tsk types.TipSetKey) (*verifreg.Claim, error) {
	idAddr, err := a.StateLookupID(ctx, providerAddr, tsk)
	if err != nil {
		return nil, err
	}

	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	st, err := a.StateManager.GetVerifregState(ctx, ts)
	if err != nil {
		return nil, err
	}

	claim, found, err := st.GetClaim(idAddr, claimId)
	if err != nil {
		return nil, xerrors.Errorf("getting claim: %w", err)
	}
	if !found {
		return nil, nil
	}

	return claim, nil
}

func (a *StateAPI) StateGetClaims(ctx context.Context, providerAddr address.Address, tsk types.TipSetKey) (map[verifreg.ClaimId]verifreg.Claim, error) {
	idAddr, err := a.StateLookupID(ctx, providerAddr, tsk)
	if err != nil {
		return nil, err
	}

	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	st, err := a.StateManager.GetVerifregState(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("loading verifreg state: %w", err)
	}

	claims, err := st.GetClaims(idAddr)
	if err != nil {
		return nil, xerrors.Errorf("getting claims: %w", err)
	}

	return claims, nil
}

func (a *StateAPI) StateGetAllClaims(ctx context.Context, tsk types.TipSetKey) (map[verifreg.ClaimId]verifreg.Claim, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	st, err := a.StateManager.GetVerifregState(ctx, ts)
	if err != nil {
		return nil, xerrors.Errorf("loading verifreg state: %w", err)
	}

	claims, err := st.GetAllClaims()
	if err != nil {
		return nil, xerrors.Errorf("getting all claims: %w", err)
	}

	return claims, nil
}

func (a *StateAPI) StateComputeDataCID(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tsk types.TipSetKey) (cid.Cid, error) {
	nv, err := a.StateNetworkVersion(ctx, tsk)
	if err != nil {
		return cid.Cid{}, err
	}

	if nv < network.Version13 {
		return a.stateComputeDataCIDv1(ctx, maddr, sectorType, deals, tsk)
	} else if nv < network.Version21 {
		return a.stateComputeDataCIDv2(ctx, maddr, sectorType, deals, tsk)
	}
	return a.stateComputeDataCIDv3(ctx, maddr, sectorType, deals, tsk)
}

func (a *StateAPI) stateComputeDataCIDv1(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tsk types.TipSetKey) (cid.Cid, error) {
	var err error
	ccparams, err := actors.SerializeParams(&market2.ComputeDataCommitmentParams{
		DealIDs:    deals,
		SectorType: sectorType,
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("computing params for ComputeDataCommitment: %w", err)
	}
	ccmt := &types.Message{
		To:    market.Address,
		From:  maddr,
		Value: types.NewInt(0),
		// Hard coded, because the method has since been deprecated
		Method: 8,
		Params: ccparams,
	}
	r, err := a.StateCall(ctx, ccmt, tsk)
	if err != nil {
		return cid.Undef, xerrors.Errorf("calling ComputeDataCommitment: %w", err)
	}
	if r.MsgRct.ExitCode != 0 {
		return cid.Undef, xerrors.Errorf("receipt for ComputeDataCommitment had exit code %d", r.MsgRct.ExitCode)
	}

	var c cbg.CborCid
	if err := c.UnmarshalCBOR(bytes.NewReader(r.MsgRct.Return)); err != nil {
		return cid.Undef, xerrors.Errorf("failed to unmarshal CBOR to CborCid: %w", err)
	}

	return cid.Cid(c), nil
}

func (a *StateAPI) stateComputeDataCIDv2(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tsk types.TipSetKey) (cid.Cid, error) {
	var err error
	ccparams, err := actors.SerializeParams(&market5.ComputeDataCommitmentParams{
		Inputs: []*market5.SectorDataSpec{
			{
				DealIDs:    deals,
				SectorType: sectorType,
			},
		},
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("computing params for ComputeDataCommitment: %w", err)
	}
	ccmt := &types.Message{
		To:    market.Address,
		From:  maddr,
		Value: types.NewInt(0),
		// Hard coded, because the method has since been deprecated
		Method: 8,
		Params: ccparams,
	}
	r, err := a.StateCall(ctx, ccmt, tsk)
	if err != nil {
		return cid.Undef, xerrors.Errorf("calling ComputeDataCommitment: %w", err)
	}
	if r.MsgRct.ExitCode != 0 {
		return cid.Undef, xerrors.Errorf("receipt for ComputeDataCommitment had exit code %d", r.MsgRct.ExitCode)
	}

	var cr market5.ComputeDataCommitmentReturn
	if err := cr.UnmarshalCBOR(bytes.NewReader(r.MsgRct.Return)); err != nil {
		return cid.Undef, xerrors.Errorf("failed to unmarshal CBOR to CborCid: %w", err)
	}

	if len(cr.CommDs) != 1 {
		return cid.Undef, xerrors.Errorf("CommD output must have 1 entry")
	}

	return cid.Cid(cr.CommDs[0]), nil
}

func (a *StateAPI) stateComputeDataCIDv3(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tsk types.TipSetKey) (cid.Cid, error) {
	if len(deals) == 0 {
		return cid.Undef, nil
	}

	var err error
	ccparams, err := actors.SerializeParams(&market12.VerifyDealsForActivationParams{
		Sectors: []market12.SectorDeals{{
			SectorType:   sectorType,
			SectorExpiry: math.MaxInt64,
			DealIDs:      deals,
		}},
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("computing params for VerifyDealsForActivation: %w", err)
	}
	ccmt := &types.Message{
		To:     market.Address,
		From:   maddr,
		Value:  types.NewInt(0),
		Method: market.Methods.VerifyDealsForActivation,
		Params: ccparams,
	}
	r, err := a.StateCall(ctx, ccmt, tsk)
	if err != nil {
		return cid.Undef, xerrors.Errorf("calling VerifyDealsForActivation: %w", err)
	}
	if r.MsgRct.ExitCode != 0 {
		return cid.Undef, xerrors.Errorf("receipt for VerifyDealsForActivation had exit code %d", r.MsgRct.ExitCode)
	}

	var cr market12.VerifyDealsForActivationReturn
	if err := cr.UnmarshalCBOR(bytes.NewReader(r.MsgRct.Return)); err != nil {
		return cid.Undef, xerrors.Errorf("failed to unmarshal CBOR to VerifyDealsForActivationReturn: %w", err)
	}
	if len(cr.UnsealedCIDs) != 1 {
		return cid.Undef, xerrors.Errorf("Sectors output must have 1 entry")
	}
	ucid := cr.UnsealedCIDs[0]
	if ucid == nil {
		return cid.Undef, xerrors.Errorf("computed data CID is nil")
	}
	return *ucid, nil
}

func (a *StateAPI) StateChangedActors(ctx context.Context, old cid.Cid, new cid.Cid) (map[string]types.Actor, error) {
	store := a.Chain.ActorStore(ctx)

	oldTree, err := state.LoadStateTree(store, old)
	if err != nil {
		return nil, xerrors.Errorf("failed to load old state tree: %w", err)
	}

	newTree, err := state.LoadStateTree(store, new)
	if err != nil {
		return nil, xerrors.Errorf("failed to load new state tree: %w", err)
	}

	return state.Diff(ctx, oldTree, newTree)
}

func (a *StateAPI) StateMinerSectorCount(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MinerSectors, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, addr, tsk)
	if err != nil {
		return api.MinerSectors{}, err
	}
	mas, err := miner.Load(a.Chain.ActorStore(ctx), act)
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

func (a *StateAPI) StateMinerAllocated(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*bitfield.BitField, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, addr, tsk)
	if err != nil {
		return nil, err
	}
	mas, err := miner.Load(a.Chain.ActorStore(ctx), act)
	if err != nil {
		return nil, err
	}
	return mas.GetAllocatedSectors()
}

func (a *StateAPI) StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorPreCommitOnChainInfo, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	pci, err := stmgr.PreCommitInfo(ctx, a.StateManager, maddr, n, ts)
	if err != nil {
		return nil, err
	}

	return pci, err
}

// StateSectorGetInfo returns nil, nil if sector is not found
func (m *StateModule) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	return stmgr.MinerSectorInfo(ctx, m.StateManager, maddr, n, ts)
}

func (a *StateAPI) StateSectorExpiration(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorExpiration, error) {
	act, err := a.StateManager.LoadActorTsk(ctx, maddr, tsk)
	if err != nil {
		return nil, err
	}
	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
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
	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
	if err != nil {
		return nil, err
	}
	return mas.FindSector(sectorNumber)
}

func (a *StateAPI) StateListMessages(ctx context.Context, match *api.MessageMatch, tsk types.TipSetKey, toheight abi.ChainEpoch) ([]cid.Cid, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}
	if ts == nil {
		ts = a.Chain.GetHeaviestTipSet()
	}

	if match.To == address.Undef && match.From == address.Undef {
		return nil, xerrors.Errorf("must specify at least To or From in message filter")
	} else if match.To != address.Undef {
		_, err := a.StateLookupID(ctx, match.To, tsk)

		// if the recipient doesn't exist at the start point, we're not gonna find any matches
		if xerrors.Is(err, &api.ErrActorNotFound{}) {
			return nil, nil
		}

		if err != nil {
			return nil, xerrors.Errorf("looking up match.To: %w", err)
		}
	} else if match.From != address.Undef {
		_, err := a.StateLookupID(ctx, match.From, tsk)

		// if the sender doesn't exist at the start point, we're not gonna find any matches
		if xerrors.Is(err, &api.ErrActorNotFound{}) {
			return nil, nil
		}

		if err != nil {
			return nil, xerrors.Errorf("looking up match.From: %w", err)
		}
	}

	// TODO: This should probably match on both ID and robust address, no?
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
		msgs, err := a.Chain.MessagesForTipset(ctx, ts)
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

		next, err := a.Chain.LoadTipSet(ctx, ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("loading next tipset: %w", err)
		}

		ts = next
	}

	return out, nil
}

func (a *StateAPI) StateCompute(ctx context.Context, height abi.ChainEpoch, msgs []*types.Message, tsk types.TipSetKey) (*api.ComputeStateOutput, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
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
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := m.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load multisig actor: %w", err)
	}
	msas, err := multisig.Load(m.Chain.ActorStore(ctx), act)
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
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return api.EmptyVesting, xerrors.Errorf("failed to load multisig actor: %w", err)
	}

	msas, err := multisig.Load(a.Chain.ActorStore(ctx), act)
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
	startTs, err := m.Chain.GetTipSetFromKey(ctx, start)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading start tipset %s: %w", start, err)
	}

	endTs, err := m.Chain.GetTipSetFromKey(ctx, end)
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

	msas, err := multisig.Load(m.Chain.ActorStore(ctx), act)
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

func (m *StateModule) MsigGetPending(ctx context.Context, addr address.Address, tsk types.TipSetKey) ([]*api.MsigTransaction, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := m.StateManager.LoadActor(ctx, addr, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load multisig actor: %w", err)
	}
	msas, err := multisig.Load(m.Chain.ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load multisig actor state: %w", err)
	}

	var out = []*api.MsigTransaction{}
	if err := msas.ForEachPendingTxn(func(id int64, txn multisig.Transaction) error {
		out = append(out, &api.MsigTransaction{
			ID:     id,
			To:     txn.To,
			Value:  txn.Value,
			Method: txn.Method,
			Params: txn.Params,

			Approved: txn.Approved,
		})
		return nil
	}); err != nil {
		return nil, err
	}

	return out, nil
}

var initialPledgeNum = types.NewInt(110)
var initialPledgeDen = types.NewInt(100)

func (a *StateAPI) StateMinerPreCommitDepositForPower(ctx context.Context, maddr address.Address, pci miner.SectorPreCommitInfo, tsk types.TipSetKey) (types.BigInt, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
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

	store := a.Chain.ActorStore(ctx)

	var sectorWeight abi.StoragePower
	if a.StateManager.GetNetworkVersion(ctx, ts.Height()) <= network.Version16 {
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
	} else {
		sectorWeight = miner.QAPowerMax(ssize)
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
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
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

	store := a.Chain.ActorStore(ctx)

	var sectorWeight abi.StoragePower
	if act, err := state.GetActor(market.Address); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading market actor: %w", err)
	} else if s, err := market.Load(store, act); err != nil {
		return types.EmptyInt, xerrors.Errorf("loading market actor state: %w", err)
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
		return types.EmptyInt, xerrors.Errorf("loading power actor: %w", err)
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
		return types.EmptyInt, xerrors.Errorf("loading reward actor: %w", err)
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
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActor(ctx, maddr, ts)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
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

func (a *StateAPI) StateMinerSectorAllocated(ctx context.Context, maddr address.Address, s abi.SectorNumber, tsk types.TipSetKey) (bool, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return false, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	act, err := a.StateManager.LoadActor(ctx, maddr, ts)
	if err != nil {
		return false, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
	if err != nil {
		return false, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	return mas.IsAllocated(s)
}

// StateVerifierStatus returns the data cap for the given address.
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

	vrs, err := verifreg.Load(a.StateManager.ChainStore().ActorStore(ctx), act)
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
func (m *StateModule) StateVerifiedClientStatus(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*abi.StoragePower, error) {
	aid, err := m.StateLookupID(ctx, addr, tsk)
	if err != nil {
		log.Warnf("lookup failure %v", err)
		return nil, err
	}

	nv, err := m.StateNetworkVersion(ctx, tsk)
	if err != nil {
		return nil, err
	}

	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return nil, err
	}

	var dcap abi.StoragePower
	var verified bool
	if av <= 8 {
		act, err := m.StateGetActor(ctx, verifreg.Address, tsk)
		if err != nil {
			return nil, err
		}

		vrs, err := verifreg.Load(m.StateManager.ChainStore().ActorStore(ctx), act)
		if err != nil {
			return nil, xerrors.Errorf("failed to load verified registry state: %w", err)
		}

		verified, dcap, err = vrs.VerifiedClientDataCap(aid)
		if err != nil {
			return nil, xerrors.Errorf("looking up verified client: %w", err)
		}
	} else {
		act, err := m.StateGetActor(ctx, datacap.Address, tsk)
		if err != nil {
			return nil, err
		}

		dcs, err := datacap.Load(m.StateManager.ChainStore().ActorStore(ctx), act)
		if err != nil {
			return nil, xerrors.Errorf("failed to load datacap actor state: %w", err)
		}

		verified, dcap, err = dcs.VerifiedClientDataCap(aid)
		if err != nil {
			return nil, xerrors.Errorf("looking up verified client: %w", err)
		}
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

	vst, err := verifreg.Load(a.StateManager.ChainStore().ActorStore(ctx), vact)
	if err != nil {
		return address.Undef, err
	}

	return vst.RootKey()
}

var dealProviderCollateralNum = types.NewInt(110)
var dealProviderCollateralDen = types.NewInt(100)

// StateDealProviderCollateralBounds returns the min and max collateral a storage provider
// can issue. It takes the deal size and verified status as parameters.
func (m *StateModule) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	pact, err := m.StateGetActor(ctx, power.Address, tsk)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load power actor: %w", err)
	}

	ract, err := m.StateGetActor(ctx, reward.Address, tsk)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load reward actor: %w", err)
	}

	pst, err := power.Load(m.StateManager.ChainStore().ActorStore(ctx), pact)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load power actor state: %w", err)
	}

	rst, err := reward.Load(m.StateManager.ChainStore().ActorStore(ctx), ract)
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("failed to load reward actor state: %w", err)
	}

	circ, err := stateVMCirculatingSupplyInternal(ctx, ts.Key(), m.Chain, m.StateManager)
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

	min, max, err := policy.DealProviderCollateralBounds(size,
		verified,
		powClaim.RawBytePower,
		powClaim.QualityAdjPower,
		rewPow,
		circ.FilCirculating,
		m.StateManager.GetNetworkVersion(ctx, ts.Height()))
	if err != nil {
		return api.DealCollateralBounds{}, xerrors.Errorf("getting deal provider coll bounds: %w", err)
	}
	return api.DealCollateralBounds{
		Min: types.BigDiv(types.BigMul(min, dealProviderCollateralNum), dealProviderCollateralDen),
		Max: max,
	}, nil
}

func (a *StateAPI) StateCirculatingSupply(ctx context.Context, tsk types.TipSetKey) (abi.TokenAmount, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return types.EmptyInt, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	sTree, err := a.StateManager.ParentState(ts)
	if err != nil {
		return types.EmptyInt, err
	}
	return a.StateManager.GetCirculatingSupply(ctx, ts.Height()-1, sTree)
}

func (a *StateAPI) StateVMCirculatingSupplyInternal(ctx context.Context, tsk types.TipSetKey) (api.CirculatingSupply, error) {
	return stateVMCirculatingSupplyInternal(ctx, tsk, a.Chain, a.StateManager)
}
func stateVMCirculatingSupplyInternal(
	ctx context.Context,
	tsk types.TipSetKey,
	cstore *store.ChainStore,
	smgr *stmgr.StateManager,
) (api.CirculatingSupply, error) {
	ts, err := cstore.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return api.CirculatingSupply{}, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	sTree, err := smgr.ParentState(ts)
	if err != nil {
		return api.CirculatingSupply{}, err
	}

	return smgr.GetVMCirculatingSupplyDetailed(ctx, ts.Height(), sTree)
}

func (m *StateModule) StateNetworkVersion(ctx context.Context, tsk types.TipSetKey) (network.Version, error) {
	ts, err := m.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return network.VersionMax, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	// TODO: Height-1 to be consistent with the rest of the APIs?
	// But that's likely going to break a bunch of stuff.
	return m.StateManager.GetNetworkVersion(ctx, ts.Height()), nil
}

func (a *StateAPI) StateActorCodeCIDs(ctx context.Context, nv network.Version) (map[string]cid.Cid, error) {
	actorVersion, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return nil, xerrors.Errorf("invalid network version %d: %w", nv, err)
	}

	cids, err := actors.GetActorCodeIDs(actorVersion)
	if err != nil {
		return nil, xerrors.Errorf("could not find cids for network version %d, actors version %d: %w", nv, actorVersion, err)
	}

	return cids, nil
}

func (a *StateAPI) StateActorManifestCID(ctx context.Context, nv network.Version) (cid.Cid, error) {
	actorVersion, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return cid.Undef, xerrors.Errorf("invalid network version")
	}

	c, ok := actors.GetManifest(actorVersion)
	if !ok {
		return cid.Undef, xerrors.Errorf("could not find manifest cid for network version %d, actors version %d", nv, actorVersion)
	}

	return c, nil
}

func (a *StateAPI) StateGetRandomnessFromTickets(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	return a.StateManager.GetRandomnessFromTickets(ctx, personalization, randEpoch, entropy, tsk)
}

func (a *StateAPI) StateGetRandomnessFromBeacon(ctx context.Context, personalization crypto.DomainSeparationTag, randEpoch abi.ChainEpoch, entropy []byte, tsk types.TipSetKey) (abi.Randomness, error) {
	return a.StateManager.GetRandomnessFromBeacon(ctx, personalization, randEpoch, entropy, tsk)
}

func (a *StateAPI) StateGetRandomnessDigestFromTickets(ctx context.Context, randEpoch abi.ChainEpoch, tsk types.TipSetKey) (abi.Randomness, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	ret, err := a.StateManager.GetRandomnessDigestFromTickets(ctx, randEpoch, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get randomness digest from tickets: %w", err)
	}

	return ret[:], nil
}

func (a *StateAPI) StateGetRandomnessDigestFromBeacon(ctx context.Context, randEpoch abi.ChainEpoch, tsk types.TipSetKey) (abi.Randomness, error) {
	ts, err := a.Chain.GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", tsk, err)
	}

	ret, err := a.StateManager.GetRandomnessDigestFromBeacon(ctx, randEpoch, ts.Key())
	if err != nil {
		return nil, xerrors.Errorf("failed to get randomness digest from tickets: %w", err)
	}

	return ret[:], nil
}

func (a *StateAPI) StateGetBeaconEntry(ctx context.Context, epoch abi.ChainEpoch) (*types.BeaconEntry, error) {
	b := a.Beacon.BeaconForEpoch(epoch)
	rr := b.MaxBeaconRoundForEpoch(a.StateManager.GetNetworkVersion(ctx, epoch), epoch)
	e := b.Entry(ctx, rr)

	select {
	case be, ok := <-e:
		if !ok {
			return nil, fmt.Errorf("beacon get returned no value")
		}
		if be.Err != nil {
			return nil, be.Err
		}
		return &be.Entry, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *StateAPI) StateGetNetworkParams(ctx context.Context) (*api.NetworkParams, error) {
	networkName, err := a.StateNetworkName(ctx)
	if err != nil {
		return nil, err
	}

	return &api.NetworkParams{
		NetworkName:             networkName,
		BlockDelaySecs:          buildconstants.BlockDelaySecs,
		ConsensusMinerMinPower:  buildconstants.ConsensusMinerMinPower,
		SupportedProofTypes:     buildconstants.SupportedProofTypes,
		PreCommitChallengeDelay: buildconstants.PreCommitChallengeDelay,
		Eip155ChainID:           buildconstants.Eip155ChainId,
		ForkUpgradeParams: api.ForkUpgradeParams{
			UpgradeSmokeHeight:       buildconstants.UpgradeSmokeHeight,
			UpgradeBreezeHeight:      buildconstants.UpgradeBreezeHeight,
			UpgradeIgnitionHeight:    buildconstants.UpgradeIgnitionHeight,
			UpgradeLiftoffHeight:     buildconstants.UpgradeLiftoffHeight,
			UpgradeAssemblyHeight:    buildconstants.UpgradeAssemblyHeight,
			UpgradeRefuelHeight:      buildconstants.UpgradeRefuelHeight,
			UpgradeTapeHeight:        buildconstants.UpgradeTapeHeight,
			UpgradeKumquatHeight:     buildconstants.UpgradeKumquatHeight,
			BreezeGasTampingDuration: buildconstants.BreezeGasTampingDuration,
			UpgradeCalicoHeight:      buildconstants.UpgradeCalicoHeight,
			UpgradePersianHeight:     buildconstants.UpgradePersianHeight,
			UpgradeOrangeHeight:      buildconstants.UpgradeOrangeHeight,
			UpgradeClausHeight:       buildconstants.UpgradeClausHeight,
			UpgradeTrustHeight:       buildconstants.UpgradeTrustHeight,
			UpgradeNorwegianHeight:   buildconstants.UpgradeNorwegianHeight,
			UpgradeTurboHeight:       buildconstants.UpgradeTurboHeight,
			UpgradeHyperdriveHeight:  buildconstants.UpgradeHyperdriveHeight,
			UpgradeChocolateHeight:   buildconstants.UpgradeChocolateHeight,
			UpgradeOhSnapHeight:      buildconstants.UpgradeOhSnapHeight,
			UpgradeSkyrHeight:        buildconstants.UpgradeSkyrHeight,
			UpgradeSharkHeight:       buildconstants.UpgradeSharkHeight,
			UpgradeHyggeHeight:       buildconstants.UpgradeHyggeHeight,
			UpgradeLightningHeight:   buildconstants.UpgradeLightningHeight,
			UpgradeThunderHeight:     buildconstants.UpgradeThunderHeight,
			UpgradeWatermelonHeight:  buildconstants.UpgradeWatermelonHeight,
			UpgradeDragonHeight:      buildconstants.UpgradeDragonHeight,
			UpgradePhoenixHeight:     buildconstants.UpgradePhoenixHeight,
			UpgradeWaffleHeight:      buildconstants.UpgradeWaffleHeight,
		},
	}, nil
}
