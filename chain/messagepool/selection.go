package messagepool

import (
	"time"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
)

type msgChain struct {
	actor     address.Address
	msgs      []*types.SignedMessage
	gasReward uint64
	gasLimit  uint64
	gasPerf   float64
	valid     bool
	next      *msgChain
}

func (mp *MessagePool) SelectMessages() []*types.SignedMessage {
	start := time.Now()
	defer func() {
		log.Infof("message selection took %s", time.Since(start))
	}()

	mp.curTsLk.Lock()
	ts := mp.curTs
	mp.curTsLk.Unlock()

	mp.lk.Lock()
	defer mp.lk.Unlock()

	return mp.selectMessages(ts)
}

func (mp *MessagePool) selectMessages(ts *types.TipSet) []*types.SignedMessage {
	// 1. Create a list of dependent message chains with maximal gas reward per limit consumed
	var chains []*msgChain
	for actor, mset := range mp.pending {
		next := mp.createMessageChains(actor, mset, ts)
		chains = append(chains, next...)
	}

	// 2. Sort the chains
	slice.Sort(chains, func(i, j int) {
		return chains[i].Before(chains[j])
	})

	// 3. Merge the head chains to produce the list of messages selected for inclusion, subject to
	//    the block gas limit.
	result := make([]*types.SignedMessage, 0, mp.maxTxPoolSizeLo)
	gasLimit := build.BlockGasLimit
	minGas := guessgas.MinGas
	last := len(chains)
	for i, chain := range chains {
		// does it fit in the block?
		if chain.gasLimit <= gasLimit {
			gasLimit -= chain.gasLimit
			result = append(result, chain.msgs...)
			continue
		}

		// we can't fit this chain because of block gasLimit -- we are at the edge
		last = i
		break
	}

	// 4. We have reached the edge of what we can fit wholesale; if we still have available gasLimit
	// to pack some more chains, then trim the last chain and push it down.
	// Trimming invalidates subsequent dependent chains so that they can't be selected as their
	// dependency cannot be (fully) included.
	// We do this in a loop because the blocker might have been inordinately large and we might
	// have to do it multiple times to satisfy tail packing.
	al := func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*types.Actor, error) {
		return mp.api.StateGetActor(addr, ts)
	}

tailLoop:
	for gasLimit >= minGas && last < len(chains) {
		// trim
		chains[last].Trim(gasLimit, al)

		// push down
		if mc.valid {
			for i := last; i < len(chains)-1; i++ {
				if chains[i].Before(chains[i+1]) {
					break
				}
				chains[i], chains[i+1] = chains[i+1], chains[i]
			}
		}

		// select the next (valid and fitting) chain for inclusion
		for i, chain := range chains[last:] {
			// has the chain been invalidated
			if !chain.valid {
				continue
			}
			// does it fit in the bock?
			if chain.gasLimit <= gasLimit {
				gasLimit -= chain.gasLimit
				result = append(result, chain.msgs...)
				continue
			}
			// this chain needs to be trimmed
			last := i
			continue tailLoop
		}

		// the merge loop ended after processing all the chains -- mark the end.
		last = len(chains)
	}

	return result
}

func (mp *MessagePool) createMessageChains(actor address.Address, mset *msgSet, ts *types.TipSet) []*msgChain {
	// TODO
	return nil
}

func (self *msgChain) Before(other *msgChain) bool {
	return self.gasPerf > other.gasPerf ||
		(self.gasPerf == other.gasPerf && self.gasReward > other.gasReward)
}

func (mc *msgChain) Trim(uint64 gasLimit, al func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*types.Actor, error)) {
	i := len(mc.msgs) - 1
	for i >= 0 && mc.gasLimit > gasLimit {
		gasLimit -= mc.msgs[i].GasLimit
		gasUsed, _ := gasguess.GuessGasUsed(context.TODO(), types.EmptyTSK, mc.msgs[i], al)
		mc.gasReward -= msg.GasPrice * gasUsed
		mc.gasLimit -= mc.msgs[i].GasLimit
		if mc.gasLimit > 0 {
			mc.gasPerf = mc.gasReward * build.BlockGasLimit / mc.gasLimit
		} else {
			mc.gasPerf = 0
		}
		i--
	}

	if i < 0 {
		mc.msgs = nil
		mc.valid = false
	} else {
		mc.msgs = mc.msgs[:i]
	}

	if mc.next != nil {
		mc.next.invalidate()
		mc.next = nil
	}
}

func (mc *msgChain) invalidate() {
	mc.valid = false
	mc.msgs = nil
	if mc.next != nil {
		mc.next.invalidate()
		mc.next = nil
	}
}
