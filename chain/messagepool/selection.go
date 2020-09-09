package messagepool

import (
	"context"
	"math/big"
	"math/rand"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	tbig "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

var bigBlockGasLimit = big.NewInt(build.BlockGasLimit)

// this is *temporary* mutilation until we have implemented uncapped miner penalties -- it will go
// away in the next fork.
func allowNegativeChains(epoch abi.ChainEpoch) bool {
	return epoch < build.UpgradeBreezeHeight+5
}

const MaxBlocks = 15

type msgChain struct {
	msgs         []*types.SignedMessage
	gasReward    *big.Int
	gasLimit     int64
	gasPerf      float64
	effPerf      float64
	bp           float64
	parentOffset float64
	valid        bool
	merged       bool
	next         *msgChain
	prev         *msgChain
}

func (mp *MessagePool) SelectMessages(ts *types.TipSet, tq float64) ([]*types.SignedMessage, error) {
	mp.curTsLk.Lock()
	defer mp.curTsLk.Unlock()

	mp.lk.Lock()
	defer mp.lk.Unlock()

	// if the ticket quality is high enough that the first block has higher probability
	// than any other block, then we don't bother with optimal selection because the
	// first block will always have higher effective performance
	if tq > 0.84 {
		return mp.selectMessagesGreedy(mp.curTs, ts)
	}

	return mp.selectMessagesOptimal(mp.curTs, ts, tq)
}

func (mp *MessagePool) selectMessagesOptimal(curTs, ts *types.TipSet, tq float64) ([]*types.SignedMessage, error) {
	start := time.Now()

	baseFee, err := mp.api.ChainComputeBaseFee(context.TODO(), ts)
	if err != nil {
		return nil, xerrors.Errorf("computing basefee: %w", err)
	}

	// 0. Load messages from the target tipset; if it is the same as the current tipset in
	//    the mpool, then this is just the pending messages
	pending, err := mp.getPendingMessages(curTs, ts)
	if err != nil {
		return nil, err
	}

	if len(pending) == 0 {
		return nil, nil
	}

	// defer only here so if we have no pending messages we don't spam
	defer func() {
		log.Infow("message selection done", "took", time.Since(start))
	}()

	// 0b. Select all priority messages that fit in the block
	minGas := int64(gasguess.MinGas)
	result, gasLimit := mp.selectPriorityMessages(pending, baseFee, ts)

	// have we filled the block?
	if gasLimit < minGas {
		return result, nil
	}

	// 1. Create a list of dependent message chains with maximal gas reward per limit consumed
	startChains := time.Now()
	var chains []*msgChain
	for actor, mset := range pending {
		next := mp.createMessageChains(actor, mset, baseFee, ts)
		chains = append(chains, next...)
	}
	if dt := time.Since(startChains); dt > time.Millisecond {
		log.Infow("create message chains done", "took", dt)
	}

	// 2. Sort the chains
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	if !allowNegativeChains(curTs.Height()) && len(chains) != 0 && chains[0].gasPerf < 0 {
		log.Warnw("all messages in mpool have non-positive gas performance", "bestGasPerf", chains[0].gasPerf)
		return result, nil
	}

	// 3. Parition chains into blocks (without trimming)
	//    we use the full blockGasLimit (as opposed to the residual gas limit from the
	//    priority message selection) as we have to account for what other miners are doing
	nextChain := 0
	partitions := make([][]*msgChain, MaxBlocks)
	for i := 0; i < MaxBlocks && nextChain < len(chains); i++ {
		gasLimit := int64(build.BlockGasLimit)
		for nextChain < len(chains) {
			chain := chains[nextChain]
			nextChain++
			partitions[i] = append(partitions[i], chain)
			gasLimit -= chain.gasLimit
			if gasLimit < minGas {
				break
			}
		}

	}

	// 4. Compute effective performance for each chain, based on the partition they fall into
	//    The effective performance is the gasPerf of the chain * block probability
	blockProb := mp.blockProbabilities(tq)
	effChains := 0
	for i := 0; i < MaxBlocks; i++ {
		for _, chain := range partitions[i] {
			chain.SetEffectivePerf(blockProb[i])
		}
		effChains += len(partitions[i])
	}

	// nullify the effective performance of chains that don't fit in any partition
	for _, chain := range chains[effChains:] {
		chain.SetNullEffectivePerf()
	}

	// 5. Resort the chains based on effective performance
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].BeforeEffective(chains[j])
	})

	// 6. Merge the head chains to produce the list of messages selected for inclusion
	//    subject to the residual gas limit
	//    When a chain is merged in, all its previous dependent chains *must* also be
	//    merged in or we'll have a broken block
	startMerge := time.Now()
	last := len(chains)
	for i, chain := range chains {
		// did we run out of performing chains?
		if !allowNegativeChains(curTs.Height()) && chain.gasPerf < 0 {
			break
		}

		// has it already been merged?
		if chain.merged {
			continue
		}

		// compute the dependencies that must be merged and the gas limit including deps
		chainGasLimit := chain.gasLimit
		var chainDeps []*msgChain
		for curChain := chain.prev; curChain != nil && !curChain.merged; curChain = curChain.prev {
			chainDeps = append(chainDeps, curChain)
			chainGasLimit += curChain.gasLimit
		}

		// does it all fit in the block?
		if chainGasLimit <= gasLimit {
			// include it together with all dependencies
			for i := len(chainDeps) - 1; i >= 0; i-- {
				curChain := chainDeps[i]
				curChain.merged = true
				result = append(result, curChain.msgs...)
			}

			chain.merged = true
			// adjust the effective pefromance for all subsequent chains
			if next := chain.next; next != nil && next.effPerf > 0 {
				next.effPerf += next.parentOffset
				for next = next.next; next != nil && next.effPerf > 0; next = next.next {
					next.setEffPerf()
				}
			}
			result = append(result, chain.msgs...)
			gasLimit -= chainGasLimit

			// resort to account for already merged chains and effective performance adjustments
			// the sort *must* be stable or we end up getting negative gasPerfs pushed up.
			sort.SliceStable(chains[i+1:], func(i, j int) bool {
				return chains[i].BeforeEffective(chains[j])
			})

			continue
		}

		// we can't fit this chain and its dependencies because of block gasLimit -- we are
		// at the edge
		last = i
		break
	}
	if dt := time.Since(startMerge); dt > time.Millisecond {
		log.Infow("merge message chains done", "took", dt)
	}

	// 7. We have reached the edge of what can fit wholesale; if we still hae available
	//    gasLimit to pack some more chains, then trim the last chain and push it down.
	//    Trimming invalidaates subsequent dependent chains so that they can't be selected
	//    as their dependency cannot be (fully) included.
	//    We do this in a loop because the blocker might have been inordinately large and
	//    we might have to do it multiple times to satisfy tail packing
	startTail := time.Now()
tailLoop:
	for gasLimit >= minGas && last < len(chains) {
		// trim if necessary
		if chains[last].gasLimit > gasLimit {
			chains[last].Trim(gasLimit, mp, baseFee, allowNegativeChains(curTs.Height()))
		}

		// push down if it hasn't been invalidated
		if chains[last].valid {
			for i := last; i < len(chains)-1; i++ {
				if chains[i].BeforeEffective(chains[i+1]) {
					break
				}
				chains[i], chains[i+1] = chains[i+1], chains[i]
			}
		}

		// select the next (valid and fitting) chain and its dependencies for inclusion
		for i, chain := range chains[last:] {
			// has the chain been invalidated?
			if !chain.valid {
				continue
			}

			// has it already been merged?
			if chain.merged {
				continue
			}

			// if gasPerf < 0 we have no more profitable chains
			if !allowNegativeChains(curTs.Height()) && chain.gasPerf < 0 {
				break tailLoop
			}

			// compute the dependencies that must be merged and the gas limit including deps
			chainGasLimit := chain.gasLimit
			depGasLimit := int64(0)
			var chainDeps []*msgChain
			for curChain := chain.prev; curChain != nil && !curChain.merged; curChain = curChain.prev {
				chainDeps = append(chainDeps, curChain)
				chainGasLimit += curChain.gasLimit
				depGasLimit += curChain.gasLimit
			}

			// does it all fit in the bock
			if chainGasLimit <= gasLimit {
				// include it together with all dependencies
				for i := len(chainDeps) - 1; i >= 0; i-- {
					curChain := chainDeps[i]
					curChain.merged = true
					result = append(result, curChain.msgs...)
				}

				chain.merged = true
				result = append(result, chain.msgs...)
				gasLimit -= chainGasLimit
				continue
			}

			// it doesn't all fit; now we have to take into account the dependent chains before
			// making a decision about trimming or invalidating.
			// if the dependencies exceed the gas limit, then we must invalidate the chain
			// as it can never be included.
			// Otherwise we can just trim and continue
			if depGasLimit > gasLimit {
				chain.Invalidate()
				last += i + 1
				continue tailLoop
			}

			// dependencies fit, just trim it
			chain.Trim(gasLimit-depGasLimit, mp, baseFee, allowNegativeChains(curTs.Height()))
			last += i
			continue tailLoop
		}

		// the merge loop ended after processing all the chains and we we probably have still
		// gas to spare; end the loop.
		break
	}
	if dt := time.Since(startTail); dt > time.Millisecond {
		log.Infow("pack tail chains done", "took", dt)
	}

	// if we have gasLimit to spare, pick some random (non-negative) chains to fill the block
	// we pick randomly so that we minimize the probability of duplication among all miners
	if gasLimit >= minGas {
		randomCount := 0

		startRandom := time.Now()
		shuffleChains(chains)

		for _, chain := range chains {
			// have we filled the block
			if gasLimit < minGas {
				break
			}

			// has it been merged or invalidated?
			if chain.merged || !chain.valid {
				continue
			}

			// is it negative?
			if !allowNegativeChains(curTs.Height()) && chain.gasPerf < 0 {
				continue
			}

			// compute the dependencies that must be merged and the gas limit including deps
			chainGasLimit := chain.gasLimit
			depGasLimit := int64(0)
			var chainDeps []*msgChain
			for curChain := chain.prev; curChain != nil && !curChain.merged; curChain = curChain.prev {
				chainDeps = append(chainDeps, curChain)
				chainGasLimit += curChain.gasLimit
				depGasLimit += curChain.gasLimit
			}

			// do the deps fit? if the deps won't fit, invalidate the chain
			if depGasLimit > gasLimit {
				chain.Invalidate()
				continue
			}

			// do they fit as is? if it doesn't, trim to make it fit if possible
			if chainGasLimit > gasLimit {
				chain.Trim(gasLimit-depGasLimit, mp, baseFee, allowNegativeChains(curTs.Height()))

				if !chain.valid {
					continue
				}
			}

			// include it together with all dependencies
			for i := len(chainDeps) - 1; i >= 0; i-- {
				curChain := chainDeps[i]
				curChain.merged = true
				result = append(result, curChain.msgs...)
				randomCount += len(curChain.msgs)
			}

			chain.merged = true
			result = append(result, chain.msgs...)
			randomCount += len(chain.msgs)
			gasLimit -= chainGasLimit
		}

		if dt := time.Since(startRandom); dt > time.Millisecond {
			log.Infow("pack random tail chains done", "took", dt)
		}

		if randomCount > 0 {
			log.Warnf("optimal selection failed to pack a block; picked %d messages with random selection",
				randomCount)
		}
	}

	return result, nil
}

func (mp *MessagePool) selectMessagesGreedy(curTs, ts *types.TipSet) ([]*types.SignedMessage, error) {
	start := time.Now()

	baseFee, err := mp.api.ChainComputeBaseFee(context.TODO(), ts)
	if err != nil {
		return nil, xerrors.Errorf("computing basefee: %w", err)
	}

	// 0. Load messages for the target tipset; if it is the same as the current tipset in the mpool
	//    then this is just the pending messages
	pending, err := mp.getPendingMessages(curTs, ts)
	if err != nil {
		return nil, err
	}

	if len(pending) == 0 {
		return nil, nil
	}

	// defer only here so if we have no pending messages we don't spam
	defer func() {
		log.Infow("message selection done", "took", time.Since(start))
	}()

	// 0b. Select all priority messages that fit in the block
	minGas := int64(gasguess.MinGas)
	result, gasLimit := mp.selectPriorityMessages(pending, baseFee, ts)

	// have we filled the block?
	if gasLimit < minGas {
		return result, nil
	}

	// 1. Create a list of dependent message chains with maximal gas reward per limit consumed
	startChains := time.Now()
	var chains []*msgChain
	for actor, mset := range pending {
		next := mp.createMessageChains(actor, mset, baseFee, ts)
		chains = append(chains, next...)
	}
	if dt := time.Since(startChains); dt > time.Millisecond {
		log.Infow("create message chains done", "took", dt)
	}

	// 2. Sort the chains
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	if !allowNegativeChains(curTs.Height()) && len(chains) != 0 && chains[0].gasPerf < 0 {
		log.Warnw("all messages in mpool have non-positive gas performance", "bestGasPerf", chains[0].gasPerf)
		return result, nil
	}

	// 3. Merge the head chains to produce the list of messages selected for inclusion, subject to
	//    the block gas limit.
	startMerge := time.Now()
	last := len(chains)
	for i, chain := range chains {
		// did we run out of performing chains?
		if !allowNegativeChains(curTs.Height()) && chain.gasPerf < 0 {
			break
		}

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
	if dt := time.Since(startMerge); dt > time.Millisecond {
		log.Infow("merge message chains done", "took", dt)
	}

	// 4. We have reached the edge of what we can fit wholesale; if we still have available gasLimit
	// to pack some more chains, then trim the last chain and push it down.
	// Trimming invalidates subsequent dependent chains so that they can't be selected as their
	// dependency cannot be (fully) included.
	// We do this in a loop because the blocker might have been inordinately large and we might
	// have to do it multiple times to satisfy tail packing.
	startTail := time.Now()
tailLoop:
	for gasLimit >= minGas && last < len(chains) {
		// trim
		chains[last].Trim(gasLimit, mp, baseFee, allowNegativeChains(curTs.Height()))

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
			// has the chain been invalidated?
			if !chain.valid {
				continue
			}

			// if gasPerf < 0 we have no more profitable chains
			if !allowNegativeChains(curTs.Height()) && chain.gasPerf < 0 {
				break tailLoop
			}

			// does it fit in the bock?
			if chain.gasLimit <= gasLimit {
				gasLimit -= chain.gasLimit
				result = append(result, chain.msgs...)
				continue
			}

			// this chain needs to be trimmed
			last += i
			continue tailLoop
		}

		// the merge loop ended after processing all the chains and we probably still have
		// gas to spare; end the loop
		break
	}
	if dt := time.Since(startTail); dt > time.Millisecond {
		log.Infow("pack tail chains done", "took", dt)
	}

	return result, nil
}

func (mp *MessagePool) selectPriorityMessages(pending map[address.Address]map[uint64]*types.SignedMessage, baseFee types.BigInt, ts *types.TipSet) ([]*types.SignedMessage, int64) {
	start := time.Now()
	defer func() {
		if dt := time.Since(start); dt > time.Millisecond {
			log.Infow("select priority messages done", "took", dt)
		}
	}()

	result := make([]*types.SignedMessage, 0, mp.cfg.SizeLimitLow)
	gasLimit := int64(build.BlockGasLimit)
	minGas := int64(gasguess.MinGas)

	// 1. Get priority actor chains
	var chains []*msgChain
	priority := mp.cfg.PriorityAddrs
	for _, actor := range priority {
		mset, ok := pending[actor]
		if ok {
			// remove actor from pending set as we are already processed these messages
			delete(pending, actor)
			// create chains for the priority actor
			next := mp.createMessageChains(actor, mset, baseFee, ts)
			chains = append(chains, next...)
		}
	}

	if len(chains) == 0 {
		return nil, gasLimit
	}

	// 2. Sort the chains
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	if !allowNegativeChains(ts.Height()) && len(chains) != 0 && chains[0].gasPerf < 0 {
		log.Warnw("all priority messages in mpool have negative gas performance", "bestGasPerf", chains[0].gasPerf)
		return nil, gasLimit
	}

	// 3. Merge chains until the block limit, as long as they have non-negative gas performance
	last := len(chains)
	for i, chain := range chains {
		if !allowNegativeChains(ts.Height()) && chain.gasPerf < 0 {
			break
		}

		if chain.gasLimit <= gasLimit {
			gasLimit -= chain.gasLimit
			result = append(result, chain.msgs...)
			continue
		}

		// we can't fit this chain because of block gasLimit -- we are at the edge
		last = i
		break
	}

tailLoop:
	for gasLimit >= minGas && last < len(chains) {
		// trim, discarding negative performing messages
		chains[last].Trim(gasLimit, mp, baseFee, allowNegativeChains(ts.Height()))

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

			// if gasPerf < 0 we have no more profitable chains
			if !allowNegativeChains(ts.Height()) && chain.gasPerf < 0 {
				break tailLoop
			}

			// does it fit in the bock?
			if chain.gasLimit <= gasLimit {
				gasLimit -= chain.gasLimit
				result = append(result, chain.msgs...)
				continue
			}

			// this chain needs to be trimmed
			last += i
			continue tailLoop
		}

		// the merge loop ended after processing all the chains and we probably still have gas to spare;
		// end the loop
		break
	}

	return result, gasLimit
}

func (mp *MessagePool) getPendingMessages(curTs, ts *types.TipSet) (map[address.Address]map[uint64]*types.SignedMessage, error) {
	start := time.Now()

	result := make(map[address.Address]map[uint64]*types.SignedMessage)
	defer func() {
		if dt := time.Since(start); dt > time.Millisecond {
			log.Infow("get pending messages done", "took", dt)
		}
	}()

	// are we in sync?
	inSync := false
	if curTs.Height() == ts.Height() && curTs.Equals(ts) {
		inSync = true
	}

	// first add our current pending messages
	for a, mset := range mp.pending {
		if inSync {
			// no need to copy the map
			result[a] = mset.msgs
		} else {
			// we need to copy the map to avoid clobbering it as we load more messages
			msetCopy := make(map[uint64]*types.SignedMessage, len(mset.msgs))
			for nonce, m := range mset.msgs {
				msetCopy[nonce] = m
			}
			result[a] = msetCopy

		}
	}

	// we are in sync, that's the happy path
	if inSync {
		return result, nil
	}

	if err := mp.runHeadChange(curTs, ts, result); err != nil {
		return nil, xerrors.Errorf("failed to process difference between mpool head and given head: %w", err)
	}

	return result, nil
}

func (*MessagePool) getGasReward(msg *types.SignedMessage, baseFee types.BigInt) *big.Int {
	maxPremium := types.BigSub(msg.Message.GasFeeCap, baseFee)

	if types.BigCmp(maxPremium, msg.Message.GasPremium) > 0 {
		maxPremium = msg.Message.GasPremium
	}

	gasReward := tbig.Mul(maxPremium, types.NewInt(uint64(msg.Message.GasLimit)))
	return gasReward.Int
}

func (*MessagePool) getGasPerf(gasReward *big.Int, gasLimit int64) float64 {
	// gasPerf = gasReward * build.BlockGasLimit / gasLimit
	a := new(big.Rat).SetInt(new(big.Int).Mul(gasReward, bigBlockGasLimit))
	b := big.NewRat(1, gasLimit)
	c := new(big.Rat).Mul(a, b)
	r, _ := c.Float64()
	return r
}

func (mp *MessagePool) createMessageChains(actor address.Address, mset map[uint64]*types.SignedMessage, baseFee types.BigInt, ts *types.TipSet) []*msgChain {
	// collect all messages
	msgs := make([]*types.SignedMessage, 0, len(mset))
	for _, m := range mset {
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
	a, err := mp.api.GetActorAfter(actor, ts)
	if err != nil {
		log.Errorf("failed to load actor state, not building chain for %s: %w", actor, err)
		return nil
	}

	curNonce := a.Nonce
	balance := a.Balance.Int
	gasLimit := int64(0)
	skip := 0
	i := 0
	rewards := make([]*big.Int, 0, len(msgs))
	for i = 0; i < len(msgs); i++ {
		m := msgs[i]

		if m.Message.Nonce < curNonce {
			log.Warnf("encountered message from actor %s with nonce (%d) less than the current nonce (%d)",
				actor, m.Message.Nonce, curNonce)
			skip++
			continue
		}

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

		required := m.Message.RequiredFunds().Int
		if balance.Cmp(required) < 0 {
			break
		}
		balance = new(big.Int).Sub(balance, required)

		value := m.Message.Value.Int
		if balance.Cmp(value) >= 0 {
			// Note: we only account for the value if the balance doesn't drop below 0
			//       otherwise the message will fail and the miner can reap the gas rewards
			balance = new(big.Int).Sub(balance, value)
		}

		gasReward := mp.getGasReward(m, baseFee)
		rewards = append(rewards, gasReward)
	}

	// check we have a sane set of messages to construct the chains
	if i > skip {
		msgs = msgs[skip:i]
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
		chain.gasPerf = mp.getGasPerf(chain.gasReward, chain.gasLimit)
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
			if chains[i].gasPerf >= chains[i-1].gasPerf {
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

	for i := len(chains) - 1; i > 0; i-- {
		chains[i].prev = chains[i-1]
	}

	return chains
}

func (mc *msgChain) Before(other *msgChain) bool {
	return mc.gasPerf > other.gasPerf ||
		(mc.gasPerf == other.gasPerf && mc.gasReward.Cmp(other.gasReward) > 0)
}

func (mc *msgChain) Trim(gasLimit int64, mp *MessagePool, baseFee types.BigInt, allowNegative bool) {
	i := len(mc.msgs) - 1
	for i >= 0 && (mc.gasLimit > gasLimit || (!allowNegative && mc.gasPerf < 0)) {
		gasReward := mp.getGasReward(mc.msgs[i], baseFee)
		mc.gasReward = new(big.Int).Sub(mc.gasReward, gasReward)
		mc.gasLimit -= mc.msgs[i].Message.GasLimit
		if mc.gasLimit > 0 {
			mc.gasPerf = mp.getGasPerf(mc.gasReward, mc.gasLimit)
			if mc.bp != 0 {
				mc.setEffPerf()
			}
		} else {
			mc.gasPerf = 0
			mc.effPerf = 0
		}
		i--
	}

	if i < 0 {
		mc.msgs = nil
		mc.valid = false
	} else {
		mc.msgs = mc.msgs[:i+1]
	}

	if mc.next != nil {
		mc.next.Invalidate()
		mc.next = nil
	}
}

func (mc *msgChain) Invalidate() {
	mc.valid = false
	mc.msgs = nil
	if mc.next != nil {
		mc.next.Invalidate()
		mc.next = nil
	}
}

func (mc *msgChain) SetEffectivePerf(bp float64) {
	mc.bp = bp
	mc.setEffPerf()
}

func (mc *msgChain) setEffPerf() {
	effPerf := mc.gasPerf * mc.bp
	if effPerf > 0 && mc.prev != nil {
		effPerfWithParent := (effPerf*float64(mc.gasLimit) + mc.prev.effPerf*float64(mc.prev.gasLimit)) / float64(mc.gasLimit+mc.prev.gasLimit)
		mc.parentOffset = effPerf - effPerfWithParent
		effPerf = effPerfWithParent
	}
	mc.effPerf = effPerf

}

func (mc *msgChain) SetNullEffectivePerf() {
	if mc.gasPerf < 0 {
		mc.effPerf = mc.gasPerf
	} else {
		mc.effPerf = 0
	}
}

func (mc *msgChain) BeforeEffective(other *msgChain) bool {
	// move merged chains to the front so we can discard them earlier
	return (mc.merged && !other.merged) ||
		(mc.gasPerf >= 0 && other.gasPerf < 0) ||
		mc.effPerf > other.effPerf ||
		(mc.effPerf == other.effPerf && mc.gasPerf > other.gasPerf) ||
		(mc.effPerf == other.effPerf && mc.gasPerf == other.gasPerf && mc.gasReward.Cmp(other.gasReward) > 0)
}

func shuffleChains(lst []*msgChain) {
	for i := range lst {
		j := rand.Intn(i + 1)
		lst[i], lst[j] = lst[j], lst[i]
	}
}
