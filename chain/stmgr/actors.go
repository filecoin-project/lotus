package stmgr

import (
	"bytes"
	"context"
	"errors"
	"os"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/proofs"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func GetMinerWorkerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (address.Address, error) {
	state, err := sm.StateTree(st)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load state tree: %w", err)
	}
	act, err := state.GetActor(maddr)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}
	mas, err := miner.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return address.Undef, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	info, err := mas.Info()
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to load actor info: %w", err)
	}

	return vm.ResolveToDeterministicAddr(state, sm.cs.ActorStore(ctx), info.Worker)
}

func GetPower(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (power.Claim, power.Claim, bool, error) {
	return GetPowerRaw(ctx, sm, ts.ParentState(), maddr)
}

func GetPowerRaw(ctx context.Context, sm *StateManager, st cid.Cid, maddr address.Address) (power.Claim, power.Claim, bool, error) {
	act, err := sm.LoadActorRaw(ctx, power.Address, st)
	if err != nil {
		return power.Claim{}, power.Claim{}, false, xerrors.Errorf("(get sset) failed to load power actor state: %w", err)
	}

	pas, err := power.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return power.Claim{}, power.Claim{}, false, err
	}

	tpow, err := pas.TotalPower()
	if err != nil {
		return power.Claim{}, power.Claim{}, false, err
	}

	var mpow power.Claim
	var minpow bool
	if maddr != address.Undef {
		var found bool
		mpow, found, err = pas.MinerPower(maddr)
		if err != nil || !found {
			return power.Claim{}, tpow, false, err
		}

		minpow, err = pas.MinerNominalPowerMeetsConsensusMinimum(maddr)
		if err != nil {
			return power.Claim{}, power.Claim{}, false, err
		}
	}

	return mpow, tpow, minpow, nil
}

func PreCommitInfo(ctx context.Context, sm *StateManager, maddr address.Address, sid abi.SectorNumber, ts *types.TipSet) (*miner.SectorPreCommitOnChainInfo, error) {
	act, err := sm.LoadActor(ctx, maddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.GetPrecommittedSector(sid)
}

// MinerSectorInfo returns nil, nil if sector is not found
func MinerSectorInfo(ctx context.Context, sm *StateManager, maddr address.Address, sid abi.SectorNumber, ts *types.TipSet) (*miner.SectorOnChainInfo, error) {
	act, err := sm.LoadActor(ctx, maddr, ts)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("(get sset) failed to load miner actor state: %w", err)
	}

	return mas.GetSector(sid)
}

func GetSectorsForWinningPoSt(ctx context.Context, nv network.Version, pv proofs.Verifier, sm *StateManager, st cid.Cid, maddr address.Address, rand abi.PoStRandomness) ([]builtin.ExtendedSectorInfo, error) {
	act, err := sm.LoadActorRaw(ctx, maddr, st)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	var provingSectors bitfield.BitField
	if nv < network.Version7 {
		allSectors, err := miner.AllPartSectors(mas, miner.Partition.AllSectors)
		if err != nil {
			return nil, xerrors.Errorf("get all sectors: %w", err)
		}

		faultySectors, err := miner.AllPartSectors(mas, miner.Partition.FaultySectors)
		if err != nil {
			return nil, xerrors.Errorf("get faulty sectors: %w", err)
		}

		provingSectors, err = bitfield.SubtractBitField(allSectors, faultySectors)
		if err != nil {
			return nil, xerrors.Errorf("calc proving sectors: %w", err)
		}
	} else {
		provingSectors, err = miner.AllPartSectors(mas, miner.Partition.ActiveSectors)
		if err != nil {
			return nil, xerrors.Errorf("get active sectors sectors: %w", err)
		}
	}

	numProvSect, err := provingSectors.Count()
	if err != nil {
		return nil, xerrors.Errorf("failed to count bits: %w", err)
	}

	// TODO(review): is this right? feels fishy to me
	if numProvSect == 0 {
		return nil, nil
	}

	info, err := mas.Info()
	if err != nil {
		return nil, xerrors.Errorf("getting miner info: %w", err)
	}

	mid, err := address.IDFromAddress(maddr)
	if err != nil {
		return nil, xerrors.Errorf("getting miner ID: %w", err)
	}

	proofType, err := miner.WinningPoStProofTypeFromWindowPoStProofType(nv, info.WindowPoStProofType)
	if err != nil {
		return nil, xerrors.Errorf("determining winning post proof type: %w", err)
	}

	ids, err := pv.GenerateWinningPoStSectorChallenge(ctx, proofType, abi.ActorID(mid), rand, numProvSect)
	if err != nil {
		return nil, xerrors.Errorf("generating winning post challenges: %w", err)
	}

	iter, err := provingSectors.BitIterator()
	if err != nil {
		return nil, xerrors.Errorf("iterating over proving sectors: %w", err)
	}

	// Select winning sectors by _index_ in the all-sectors bitfield.
	selectedSectors := bitfield.New()
	prev := uint64(0)
	for _, n := range ids {
		sno, err := iter.Nth(n - prev)
		if err != nil {
			return nil, xerrors.Errorf("iterating over proving sectors: %w", err)
		}
		selectedSectors.Set(sno)
		prev = n
	}

	sectors, err := mas.LoadSectors(&selectedSectors)
	if err != nil {
		return nil, xerrors.Errorf("loading proving sectors: %w", err)
	}

	out := make([]builtin.ExtendedSectorInfo, len(sectors))
	for i, sinfo := range sectors {
		out[i] = builtin.ExtendedSectorInfo{
			SealProof:    sinfo.SealProof,
			SectorNumber: sinfo.SectorNumber,
			SealedCID:    sinfo.SealedCID,
			SectorKey:    sinfo.SectorKeyCID,
		}
	}

	return out, nil
}

func GetMinerSlashed(ctx context.Context, sm *StateManager, ts *types.TipSet, maddr address.Address) (bool, error) {
	act, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return false, xerrors.Errorf("failed to load power actor: %w", err)
	}

	spas, err := power.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return false, xerrors.Errorf("failed to load power actor state: %w", err)
	}

	_, ok, err := spas.MinerPower(maddr)
	if err != nil {
		return false, xerrors.Errorf("getting miner power: %w", err)
	}

	if !ok {
		return true, nil
	}

	return false, nil
}

func GetStorageDeal(ctx context.Context, sm *StateManager, dealID abi.DealID, ts *types.TipSet) (*api.MarketDeal, error) {
	act, err := sm.LoadActor(ctx, market.Address, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load market actor: %w", err)
	}

	state, err := market.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load market actor state: %w", err)
	}

	proposals, err := state.Proposals()
	if err != nil {
		return nil, xerrors.Errorf("failed to get proposals from state : %w", err)
	}

	proposal, found, err := proposals.Get(dealID)

	if err != nil {
		return nil, xerrors.Errorf("failed to get proposal : %w", err)
	} else if !found {
		return nil, xerrors.Errorf(
			"deal %d not found "+
				"- deal may not have completed sealing before deal proposal "+
				"start epoch, or deal may have been slashed",
			dealID)
	}

	states, err := state.States()
	if err != nil {
		return nil, xerrors.Errorf("failed to get states : %w", err)
	}

	st, found, err := states.Get(dealID)
	if err != nil {
		return nil, xerrors.Errorf("failed to get state : %w", err)
	}

	if !found {
		st = market.EmptyDealState()
	}

	return &api.MarketDeal{
		Proposal: *proposal,
		State:    api.MakeDealState(st),
	}, nil
}

func ListMinerActors(ctx context.Context, sm *StateManager, ts *types.TipSet) ([]address.Address, error) {
	act, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return nil, xerrors.Errorf("failed to load power actor: %w", err)
	}

	powState, err := power.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load power actor state: %w", err)
	}

	return powState.ListAllMiners()
}

func MinerGetBaseInfo(ctx context.Context, sm *StateManager, bcs beacon.Schedule, tsk types.TipSetKey, round abi.ChainEpoch, maddr address.Address, pv proofs.Verifier) (*api.MiningBaseInfo, error) {
	ts, err := sm.ChainStore().LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, xerrors.Errorf("failed to load tipset for mining base: %w", err)
	}

	prev, err := sm.ChainStore().GetLatestBeaconEntry(ctx, ts)
	if err != nil {
		if os.Getenv("LOTUS_IGNORE_DRAND") != "_yes_" {
			return nil, xerrors.Errorf("failed to get latest beacon entry: %w", err)
		}

		prev = &types.BeaconEntry{}
	}

	entries, err := beacon.BeaconEntriesForBlock(ctx, bcs, sm.GetNetworkVersion(ctx, round), round, ts.Height(), *prev)
	if err != nil {
		return nil, err
	}

	rbase := *prev
	if len(entries) > 0 {
		rbase = entries[len(entries)-1]
	}

	lbts, lbst, err := GetLookbackTipSetForRound(ctx, sm, ts, round)
	if err != nil {
		return nil, xerrors.Errorf("getting lookback miner actor state: %w", err)
	}

	act, err := sm.LoadActorRaw(ctx, maddr, lbst)
	if errors.Is(err, types.ErrActorNotFound) {
		_, err := sm.LoadActor(ctx, maddr, ts)
		if err != nil {
			return nil, xerrors.Errorf("loading miner in current state: %w", err)
		}

		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor: %w", err)
	}

	mas, err := miner.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, xerrors.Errorf("failed to load miner actor state: %w", err)
	}

	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return nil, xerrors.Errorf("failed to marshal miner address: %w", err)
	}

	prand, err := rand.DrawRandomnessFromBase(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, round, buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("failed to get randomness for winning post: %w", err)
	}

	nv := sm.GetNetworkVersion(ctx, ts.Height())

	sectors, err := GetSectorsForWinningPoSt(ctx, nv, pv, sm, lbst, maddr, prand)
	if err != nil {
		return nil, xerrors.Errorf("getting winning post proving set: %w", err)
	}

	if len(sectors) == 0 {
		return nil, nil
	}

	mpow, tpow, _, err := GetPowerRaw(ctx, sm, lbst, maddr)
	if err != nil {
		return nil, xerrors.Errorf("failed to get power: %w", err)
	}

	info, err := mas.Info()
	if err != nil {
		return nil, err
	}

	worker, err := sm.ResolveToDeterministicAddress(ctx, info.Worker, ts)
	if err != nil {
		return nil, xerrors.Errorf("resolving worker address: %w", err)
	}

	// TODO: Not ideal performance...This method reloads miner and power state (already looked up here and in GetPowerRaw)
	eligible, err := MinerEligibleToMine(ctx, sm, maddr, ts, lbts)
	if err != nil {
		return nil, xerrors.Errorf("determining miner eligibility: %w", err)
	}

	return &api.MiningBaseInfo{
		MinerPower:        mpow.QualityAdjPower,
		NetworkPower:      tpow.QualityAdjPower,
		Sectors:           sectors,
		WorkerKey:         worker,
		SectorSize:        info.SectorSize,
		PrevBeaconEntry:   *prev,
		BeaconEntries:     entries,
		EligibleForMining: eligible,
	}, nil
}

func minerHasMinPower(ctx context.Context, sm *StateManager, addr address.Address, ts *types.TipSet) (bool, error) {
	pact, err := sm.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return false, xerrors.Errorf("loading power actor state: %w", err)
	}

	ps, err := power.Load(sm.cs.ActorStore(ctx), pact)
	if err != nil {
		return false, err
	}

	return ps.MinerNominalPowerMeetsConsensusMinimum(addr)
}

func MinerEligibleToMine(ctx context.Context, sm *StateManager, addr address.Address, baseTs *types.TipSet, lookbackTs *types.TipSet) (bool, error) {
	hmp, err := minerHasMinPower(ctx, sm, addr, lookbackTs)

	// TODO: We're blurring the lines between a "runtime network version" and a "Lotus upgrade epoch", is that unavoidable?
	if sm.GetNetworkVersion(ctx, baseTs.Height()) <= network.Version3 {
		return hmp, err
	}

	if err != nil {
		return false, err
	}

	if !hmp {
		return false, nil
	}

	// Post actors v2, also check MinerEligibleForElection with base ts

	pact, err := sm.LoadActor(ctx, power.Address, baseTs)
	if err != nil {
		return false, xerrors.Errorf("loading power actor state: %w", err)
	}

	pstate, err := power.Load(sm.cs.ActorStore(ctx), pact)
	if err != nil {
		return false, err
	}

	mact, err := sm.LoadActor(ctx, addr, baseTs)
	if err != nil {
		return false, xerrors.Errorf("loading miner actor state: %w", err)
	}

	mstate, err := miner.Load(sm.cs.ActorStore(ctx), mact)
	if err != nil {
		return false, err
	}

	// Non-empty power claim.
	if claim, found, err := pstate.MinerPower(addr); err != nil {
		return false, err
	} else if !found {
		return false, err
	} else if claim.QualityAdjPower.LessThanEqual(big.Zero()) {
		return false, err
	}

	// No fee debt.
	if debt, err := mstate.FeeDebt(); err != nil {
		return false, err
	} else if !debt.IsZero() {
		return false, err
	}

	// No active consensus faults.
	if mInfo, err := mstate.Info(); err != nil {
		return false, err
	} else if baseTs.Height() <= mInfo.ConsensusFaultElapsed {
		return false, nil
	}

	return true, nil
}

func (sm *StateManager) GetPaychState(ctx context.Context, addr address.Address, ts *types.TipSet) (*types.Actor, paych.State, error) {
	st, err := sm.ParentState(ts)
	if err != nil {
		return nil, nil, err
	}

	act, err := st.GetActor(addr)
	if err != nil {
		return nil, nil, err
	}

	actState, err := paych.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, nil, err
	}
	return act, actState, nil
}

func (sm *StateManager) GetMarketState(ctx context.Context, ts *types.TipSet) (market.State, error) {
	st, err := sm.ParentState(ts)
	if err != nil {
		return nil, err
	}

	act, err := st.GetActor(market.Address)
	if err != nil {
		return nil, err
	}

	actState, err := market.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, err
	}
	return actState, nil
}

func (sm *StateManager) GetVerifregState(ctx context.Context, ts *types.TipSet) (verifreg.State, error) {
	st, err := sm.ParentState(ts)
	if err != nil {
		return nil, err
	}

	act, err := st.GetActor(verifreg.Address)
	if err != nil {
		return nil, err
	}

	actState, err := verifreg.Load(sm.cs.ActorStore(ctx), act)
	if err != nil {
		return nil, err
	}
	return actState, nil
}

func (sm *StateManager) MarketBalance(ctx context.Context, addr address.Address, ts *types.TipSet) (api.MarketBalance, error) {
	mstate, err := sm.GetMarketState(ctx, ts)
	if err != nil {
		return api.MarketBalance{}, err
	}

	addr, err = sm.LookupIDAddress(ctx, addr, ts)
	if err != nil {
		return api.MarketBalance{}, err
	}

	var out api.MarketBalance

	et, err := mstate.EscrowTable()
	if err != nil {
		return api.MarketBalance{}, err
	}
	out.Escrow, err = et.Get(addr)
	if err != nil {
		return api.MarketBalance{}, xerrors.Errorf("getting escrow balance: %w", err)
	}

	lt, err := mstate.LockedTable()
	if err != nil {
		return api.MarketBalance{}, err
	}
	out.Locked, err = lt.Get(addr)
	if err != nil {
		return api.MarketBalance{}, xerrors.Errorf("getting locked balance: %w", err)
	}

	return out, nil
}

var _ StateManagerAPI = (*StateManager)(nil)
