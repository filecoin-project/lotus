package simulation

import (
	"context"
	"sort"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

// simualtionState holds the "state" of the simulation. This is split from the Simulation type so we
// can load it on-dempand if and when we need to actually _run_ the simualation. Loading the
// simulation state requires walking all active miners.
type simulationState struct {
	*Simulation

	// The tiers represent the top 1%, top 10%, and everyone else. When sealing sectors, we seal
	// a group of sectors for the top 1%, a group (half that size) for the top 10%, and one
	// sector for everyone else. We determine these rates by looking at two power tables.
	// TODO Ideally we'd "learn" this distribution from the network. But this is good enough for
	// now.
	minerDist struct {
		top1, top10, rest actorIter
	}

	// We track the window post periods per miner and assume that no new miners are ever added.
	wpostPeriods map[int][]address.Address // (epoch % (epochs in a deadline)) -> miner
	// We cache all miner infos for active miners and assume no new miners join.
	minerInfos map[address.Address]*miner.MinerInfo

	// We record all pending window post messages, and the epoch up through which we've
	// generated window post messages.
	pendingWposts  []*types.Message
	nextWpostEpoch abi.ChainEpoch

	// We track the set of pending commits. On simulation load, and when a new pre-commit is
	// added to the chain, we put the commit in this queue. advanceEpoch(currentEpoch) should be
	// called on this queue at every epoch before using it.
	commitQueue commitQueue
}

func loadSimulationState(ctx context.Context, sim *Simulation) (*simulationState, error) {
	state := &simulationState{Simulation: sim}
	currentEpoch := sim.head.Height()

	// Lookup the current power table and the power table 2 weeks ago (for onboarding rate
	// projections).
	currentPowerTable, err := sim.loadClaims(ctx, currentEpoch)
	if err != nil {
		return nil, err
	}

	var lookbackEpoch abi.ChainEpoch
	//if epoch > onboardingProjectionLookback {
	//	lookbackEpoch = epoch - onboardingProjectionLookback
	//}
	// TODO: Fixme? I really want this to not suck with snapshots.
	lookbackEpoch = 770139 // hard coded for now.
	lookbackPowerTable, err := sim.loadClaims(ctx, lookbackEpoch)
	if err != nil {
		return nil, err
	}

	type onboardingInfo struct {
		addr           address.Address
		onboardingRate uint64
	}

	commitRand, err := sim.postChainCommitInfo(ctx, currentEpoch)
	if err != nil {
		return nil, err
	}

	sealList := make([]onboardingInfo, 0, len(currentPowerTable))
	state.wpostPeriods = make(map[int][]address.Address, miner.WPoStChallengeWindow)
	state.minerInfos = make(map[address.Address]*miner.MinerInfo, len(currentPowerTable))
	state.commitQueue.advanceEpoch(state.nextEpoch())
	for addr, claim := range currentPowerTable {
		// Load the miner state.
		_, minerState, err := state.getMinerState(ctx, addr)
		if err != nil {
			return nil, err
		}

		info, err := minerState.Info()
		if err != nil {
			return nil, err
		}
		state.minerInfos[addr] = &info

		// Queue up PoSts
		err = state.stepWindowPoStsMiner(ctx, addr, minerState, currentEpoch, commitRand)
		if err != nil {
			return nil, err
		}

		// Qeueu up any pending prove commits.
		err = state.loadProveCommitsMiner(ctx, addr, minerState)
		if err != nil {
			return nil, err
		}

		// Record when we need to prove for this miner.
		dinfo, err := minerState.DeadlineInfo(state.nextEpoch())
		if err != nil {
			return nil, err
		}
		dinfo = dinfo.NextNotElapsed()

		ppOffset := int(dinfo.PeriodStart % miner.WPoStChallengeWindow)
		state.wpostPeriods[ppOffset] = append(state.wpostPeriods[ppOffset], addr)

		sectorsAdded := sectorsFromClaim(info.SectorSize, claim)
		if lookbackClaim, ok := lookbackPowerTable[addr]; !ok {
			sectorsAdded -= sectorsFromClaim(info.SectorSize, lookbackClaim)
		}

		// NOTE: power _could_ have been lost, but that's too much of a pain to care
		// about. We _could_ look for faulty power by iterating through all
		// deadlines, but I'd rather not.
		if sectorsAdded > 0 {
			sealList = append(sealList, onboardingInfo{addr, uint64(sectorsAdded)})
		}
	}
	// We're already done loading for the _next_ epoch.
	// Next time, we need to load for the next, next epoch.
	// TODO: fix this insanity.
	state.nextWpostEpoch = state.nextEpoch() + 1

	// Now that we have a list of sealing miners, sort them into percentiles.
	sort.Slice(sealList, func(i, j int) bool {
		return sealList[i].onboardingRate < sealList[j].onboardingRate
	})

	for i, oi := range sealList {
		var dist *actorIter
		if i < len(sealList)/100 {
			dist = &state.minerDist.top1
		} else if i < len(sealList)/10 {
			dist = &state.minerDist.top10
		} else {
			dist = &state.minerDist.rest
		}
		dist.add(oi.addr)
	}

	state.minerDist.top1.shuffle()
	state.minerDist.top10.shuffle()
	state.minerDist.rest.shuffle()

	return state, nil
}

// nextEpoch returns the next epoch (head+1).
func (ss *simulationState) nextEpoch() abi.ChainEpoch {
	return ss.GetHead().Height() + 1
}

// getMinerInfo returns the miner's cached info.
//
// NOTE: we assume that miner infos won't change. We'll need to fix this if we start supporting arbitrary message.
func (ss *simulationState) getMinerInfo(ctx context.Context, addr address.Address) (*miner.MinerInfo, error) {
	minerInfo, ok := ss.minerInfos[addr]
	if !ok {
		_, minerState, err := ss.getMinerState(ctx, addr)
		if err != nil {
			return nil, err
		}
		info, err := minerState.Info()
		if err != nil {
			return nil, err
		}
		minerInfo = &info
		ss.minerInfos[addr] = minerInfo
	}
	return minerInfo, nil
}

// getMinerState loads the miner actor & state.
func (ss *simulationState) getMinerState(ctx context.Context, addr address.Address) (*types.Actor, miner.State, error) {
	st, err := ss.stateTree(ctx)
	if err != nil {
		return nil, nil, err
	}
	act, err := st.GetActor(addr)
	if err != nil {
		return nil, nil, err
	}
	state, err := miner.Load(ss.Chainstore.ActorStore(ctx), act)
	if err != nil {
		return nil, nil, err
	}
	return act, state, err
}
