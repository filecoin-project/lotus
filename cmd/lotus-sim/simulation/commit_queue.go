package simulation

import (
	"sort"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

type pendingCommitTracker map[address.Address]minerPendingCommits
type minerPendingCommits map[abi.RegisteredSealProof][]abi.SectorNumber

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

func (m minerPendingCommits) empty() bool {
	return len(m) == 0
}

func (m minerPendingCommits) count() int {
	count := 0
	for _, snos := range m {
		count += len(snos)
	}
	return count
}

type commitQueue struct {
	minerQueue []address.Address
	queue      []pendingCommitTracker
	offset     abi.ChainEpoch
}

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

func (q *commitQueue) enqueueProveCommit(addr address.Address, preCommitEpoch abi.ChainEpoch, info miner.SectorPreCommitInfo) error {
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

func (q *commitQueue) head() pendingCommitTracker {
	if len(q.queue) > 0 {
		return q.queue[0]
	}
	return nil
}
