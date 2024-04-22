package stages

import (
	"sort"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// pendingCommitTracker tracks pending commits per-miner for a single epoch.
type pendingCommitTracker map[address.Address]minerPendingCommits

// minerPendingCommits tracks a miner's pending commits during a single epoch (grouped by seal proof type).
type minerPendingCommits map[abi.RegisteredSealProof][]abi.SectorNumber

// finish marks count sectors of the given proof type as "prove-committed".
func (m minerPendingCommits) finish(proof abi.RegisteredSealProof, count int) {
	snos := m[proof]
	if len(snos) < count {
		panic("not enough sector numbers to finish")
	} else if len(snos) == count {
		delete(m, proof)
	} else {
		m[proof] = snos[count:]
	}
}

// empty returns true if there are no pending commits.
func (m minerPendingCommits) empty() bool {
	return len(m) == 0
}

// count returns the number of pending commits.
func (m minerPendingCommits) count() int {
	count := 0
	for _, snos := range m {
		count += len(snos)
	}
	return count
}

// commitQueue is used to track pending prove-commits.
//
// Miners are processed in round-robin where _all_ commits from a given miner are finished before
// moving on to the next. This is designed to maximize batching.
type commitQueue struct {
	minerQueue []address.Address
	queue      []pendingCommitTracker
	offset     abi.ChainEpoch
}

// ready returns the number of prove-commits ready to be proven at the current epoch. Useful for logging.
func (q *commitQueue) ready() int {
	if len(q.queue) == 0 {
		return 0
	}
	count := 0
	for _, pending := range q.queue[0] {
		count += pending.count()
	}
	return count
}

// nextMiner returns the next miner to be proved and the set of pending prove commits for that
// miner. When some number of sectors have successfully been proven, call "finish" so we don't try
// to prove them again.
func (q *commitQueue) nextMiner() (address.Address, minerPendingCommits, bool) {
	if len(q.queue) == 0 {
		return address.Undef, nil, false
	}
	next := q.queue[0]

	// Go through the queue and find the first non-empty batch.
	for len(q.minerQueue) > 0 {
		addr := q.minerQueue[0]
		q.minerQueue = q.minerQueue[1:]
		pending := next[addr]
		if !pending.empty() {
			return addr, pending, true
		}
		delete(next, addr)
	}

	return address.Undef, nil, false
}

// advanceEpoch will advance to the next epoch. If some sectors were left unproven in the current
// epoch, they will be "prepended" into the next epochs sector set.
func (q *commitQueue) advanceEpoch(epoch abi.ChainEpoch) {
	if epoch < q.offset {
		panic("cannot roll epoch backwards")
	}
	// Now we "roll forwards", merging each epoch we advance over with the next.
	for len(q.queue) > 1 && q.offset < epoch {
		curr := q.queue[0]
		q.queue[0] = nil
		q.queue = q.queue[1:]
		q.offset++

		next := q.queue[0]

		// Cleanup empty entries.
		for addr, pending := range curr {
			if pending.empty() {
				delete(curr, addr)
			}
		}

		// If the entire level is actually empty, just skip to the next one.
		if len(curr) == 0 {
			continue
		}

		// Otherwise, merge the next into the current.
		for addr, nextPending := range next {
			currPending := curr[addr]
			if currPending.empty() {
				curr[addr] = nextPending
				continue
			}
			for ty, nextSnos := range nextPending {
				currSnos := currPending[ty]
				if len(currSnos) == 0 {
					currPending[ty] = nextSnos
					continue
				}
				currPending[ty] = append(currSnos, nextSnos...)
			}
		}
		// Now replace next with the merged curr.
		q.queue[0] = curr
	}
	q.offset = epoch
	if len(q.queue) == 0 {
		return
	}

	next := q.queue[0]
	seenMiners := make(map[address.Address]struct{}, len(q.minerQueue))
	for _, addr := range q.minerQueue {
		seenMiners[addr] = struct{}{}
	}

	// Find the new miners not already in the queue.
	offset := len(q.minerQueue)
	for addr, pending := range next {
		if pending.empty() {
			delete(next, addr)
			continue
		}
		if _, ok := seenMiners[addr]; ok {
			continue
		}
		q.minerQueue = append(q.minerQueue, addr)
	}

	// Sort the new miners only.
	newMiners := q.minerQueue[offset:]
	sort.Slice(newMiners, func(i, j int) bool {
		// eh, escape analysis should be fine here...
		return string(newMiners[i].Bytes()) < string(newMiners[j].Bytes())
	})
}

// enqueueProveCommit enqueues prove-commit for the given pre-commit for the given miner.
func (q *commitQueue) enqueueProveCommit(addr address.Address, preCommitEpoch abi.ChainEpoch, info minertypes.SectorPreCommitInfo) error {
	// Compute the epoch at which we can start trying to commit.
	preCommitDelay := policy.GetPreCommitChallengeDelay()
	minCommitEpoch := preCommitEpoch + preCommitDelay + 1

	// Figure out the offset in the queue.
	i := int(minCommitEpoch - q.offset)
	if i < 0 {
		i = 0
	}

	// Expand capacity and insert.
	if cap(q.queue) <= i {
		pc := make([]pendingCommitTracker, i+1, preCommitDelay*2)
		copy(pc, q.queue)
		q.queue = pc
	} else if len(q.queue) <= i {
		q.queue = q.queue[:i+1]
	}
	tracker := q.queue[i]
	if tracker == nil {
		tracker = make(pendingCommitTracker)
		q.queue[i] = tracker
	}
	minerPending := tracker[addr]
	if minerPending == nil {
		minerPending = make(minerPendingCommits)
		tracker[addr] = minerPending
	}
	minerPending[info.SealProof] = append(minerPending[info.SealProof], info.SectorNumber)
	return nil
}
