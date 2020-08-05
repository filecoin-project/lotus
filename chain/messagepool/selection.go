package messagepool

import (
	"context"
	"math/big"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	abig "github.com/filecoin-project/specs-actors/actors/abi/big"
)

type msgChain struct {
	msgs      []*types.SignedMessage
	gasReward *big.Int
	gasLimit  int64
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
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	// 3. Merge the head chains to produce the list of messages selected for inclusion, subject to
	//    the block gas limit.
	result := make([]*types.SignedMessage, 0, mp.maxTxPoolSizeLo)
	gasLimit := int64(build.BlockGasLimit)
	minGas := int64(gasguess.MinGas)
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
tailLoop:
	for gasLimit >= minGas && last < len(chains) {
		// trim
		chains[last].Trim(gasLimit, mp, ts)

		// push down if it hasn't been invalidated
		if chains[last].valid {
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
			last = i
			continue tailLoop
		}

		// the merge loop ended after processing all the chains and we probably still have gas to spare
		// -- mark the end.
		last = len(chains)
	}

	return result
}

func (mp *MessagePool) getGasReward(msg *types.SignedMessage, ts *types.TipSet) *big.Int {
	al := func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*types.Actor, error) {
		return mp.api.StateGetActor(addr, ts)
	}
	gasUsed, _ := gasguess.GuessGasUsed(context.TODO(), types.EmptyTSK, msg, al)
	gasReward := abig.Mul(msg.Message.GasPrice, types.NewInt(uint64(gasUsed)))
	return gasReward.Int
}

func (mp *MessagePool) getGasPerf(gasReward *big.Int, gasLimit int64) float64 {
	// gasPerf = gasReward * build.BlockGasLimit / gasLimit
	a := new(big.Rat).SetInt(new(big.Int).Mul(gasReward, big.NewInt(build.BlockGasLimit)))
	b := big.NewRat(1, gasLimit)
	c := new(big.Rat).Mul(a, b)
	r, _ := c.Float64()
	return r
}

func (mp *MessagePool) createMessageChains(actor address.Address, mset *msgSet, ts *types.TipSet) []*msgChain {
	// collect all messages
	msgs := make([]*types.SignedMessage, 0, len(mset.msgs))
	for _, m := range mset.msgs {
		msgs = append(msgs, m)
	}

	// sort by nonce
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Message.Nonce < msgs[j].Message.Nonce
	})

	// sanity checks:
	// - there can be no gaps in nonces, starting from the current actor nonce
	//   if there is a gap, drop messages after the gap, we can't include them
	// - all messages must have minimum gas and the total gas for the candidate messages
	//   cannot exceed the block limit; drop all messages that exceed the limit
	// - the total gasReward cannot exceed the actor's balance; drop all messages that exceed
	//   the balance
	a, _ := mp.api.StateGetActor(actor, nil)
	curNonce := a.Nonce
	balance := a.Balance.Int
	gasLimit := int64(0)
	i := 0
	rewards := make([]*big.Int, 0, len(msgs))
	for i = 0; i < len(msgs); i++ {
		m := msgs[i]
		if m.Message.Nonce != curNonce {
			break
		}
		curNonce++

		minGas := vm.PricelistByEpoch(ts.Height()).OnChainMessage(m.ChainLength()).Total()
		if m.Message.GasLimit < minGas {
			break
		}

		gasLimit += m.Message.GasLimit
		if gasLimit > build.BlockGasLimit {
			break
		}

		gasReward := mp.getGasReward(m, ts)
		if gasReward.Cmp(balance) > 0 {
			break
		}
		balance = new(big.Int).Sub(balance, gasReward)
		rewards = append(rewards, gasReward)
	}

	// check we have a sane set of messages to construct the chains
	if i > 0 {
		msgs = msgs[:i]
	} else {
		return nil
	}

	// ok, now we can construct the chains using the messages we have
	// invariant: each chain has a bigger gasPerf than the next -- otherwise they can be merged
	// and increase the gasPerf of the first chain
	// We do this in two passes:
	// - in the first pass we create chains that aggreagate messages with non-decreasing gasPerf
	// - in the second pass we merge chains to maintain the invariant.
	var chains []*msgChain
	var curChain *msgChain

	newChain := func(m *types.SignedMessage, i int) *msgChain {
		chain := new(msgChain)
		chain.msgs = []*types.SignedMessage{m}
		chain.gasReward = rewards[i]
		chain.gasLimit = m.Message.GasLimit
		chain.gasPerf = mp.getGasPerf(curChain.gasReward, curChain.gasLimit)
		chain.valid = true
		return chain
	}

	// create the individual chains
	for i, m := range msgs {
		if curChain == nil {
			curChain = newChain(m, i)
			continue
		}

		gasReward := new(big.Int).Add(curChain.gasReward, rewards[i])
		gasLimit := curChain.gasLimit + m.Message.GasLimit
		gasPerf := mp.getGasPerf(gasReward, gasLimit)

		// try to add the message to the current chain -- if it decreases the gasPerf, then make a
		// new chain
		if gasPerf < curChain.gasPerf {
			chains = append(chains, curChain)
			curChain = newChain(m, i)
		} else {
			curChain.msgs = append(curChain.msgs, m)
			curChain.gasReward = gasReward
			curChain.gasLimit = gasLimit
			curChain.gasPerf = gasPerf
		}
	}
	chains = append(chains, curChain)

	// merge chains to maintain the invariant
	for {
		merged := 0

		for i := len(chains) - 1; i > 0; i-- {
			if chains[i].gasPerf > chains[i-1].gasPerf {
				chains[i-1].msgs = append(chains[i-1].msgs, chains[i].msgs...)
				chains[i-1].gasReward = new(big.Int).Add(chains[i-1].gasReward, chains[i].gasReward)
				chains[i-1].gasLimit += chains[i].gasLimit
				chains[i-1].gasPerf = mp.getGasPerf(chains[i-1].gasReward, chains[i-1].gasLimit)
				chains[i].valid = false
				merged++
			}
		}

		if merged == 0 {
			break
		}

		// drop invalidated chains
		newChains := make([]*msgChain, 0, len(chains)-merged)
		for _, c := range chains {
			if c.valid {
				newChains = append(newChains, c)
			}
		}
		chains = newChains
	}

	// link dependent chains
	for i := 0; i < len(chains)-1; i++ {
		chains[i].next = chains[i+1]
	}

	return chains
}

func (self *msgChain) Before(other *msgChain) bool {
	return self.gasPerf > other.gasPerf ||
		(self.gasPerf == other.gasPerf && self.gasReward.Cmp(other.gasReward) < 0)
}

func (mc *msgChain) Trim(gasLimit int64, mp *MessagePool, ts *types.TipSet) {
	i := len(mc.msgs) - 1
	for i >= 0 && mc.gasLimit > gasLimit {
		gasLimit -= mc.msgs[i].Message.GasLimit
		gasReward := mp.getGasReward(mc.msgs[i], ts)
		mc.gasReward = new(big.Int).Sub(mc.gasReward, gasReward)
		mc.gasLimit -= mc.msgs[i].Message.GasLimit
		if mc.gasLimit > 0 {
			mc.gasPerf = mp.getGasPerf(mc.gasReward, mc.gasLimit)
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
