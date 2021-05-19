package simulation

import (
	"context"
	"math/rand"
	"sort"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

type perm struct {
	miners []address.Address
	offset int
}

func (p *perm) shuffle() {
	rand.Shuffle(len(p.miners), func(i, j int) {
		p.miners[i], p.miners[j] = p.miners[j], p.miners[i]
	})
}

func (p *perm) next() address.Address {
	next := p.miners[p.offset]
	p.offset++
	p.offset %= len(p.miners)
	return next
}

func (p *perm) add(addr address.Address) {
	p.miners = append(p.miners, addr)
}

func (p *perm) len() int {
	return len(p.miners)
}

type simulationState struct {
	*Simulation

	// TODO Ideally we'd "learn" this distribution from the network. But this is good enough for
	// now. The tiers represent the top 1%, top 10%, and everyone else. When sealing sectors, we
	// seal a group of sectors for the top 1%, a group (half that size) for the top 10%, and one
	// sector for everyone else. We really should pick a better algorithm.
	minerDist struct {
		top1, top10, rest perm
	}

	// We track the window post periods per miner and assume that no new miners are ever added.
	wpostPeriods map[int][]address.Address // (epoch % (epochs in a deadline)) -> miner
	// We cache all miner infos for active miners and assume no new miners join.
	minerInfos map[address.Address]*miner.MinerInfo

	// We record all pending window post messages, and the epoch up through which we've
	// generated window post messages.
	pendingWposts  []*types.Message
	nextWpostEpoch abi.ChainEpoch

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

	// Now load miner state info.
	store := sim.Chainstore.ActorStore(ctx)
	st, err := sim.stateTree(ctx)
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
		minerActor, err := st.GetActor(addr)
		if err != nil {
			return nil, err
		}

		minerState, err := miner.Load(store, minerActor)
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
		var dist *perm
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

func (ss *simulationState) nextEpoch() abi.ChainEpoch {
	return ss.GetHead().Height() + 1
}
