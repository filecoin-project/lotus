package messagepool

import (
	"context"
	"math/big"
	"math/rand"
	"sort"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	abig "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/ipfs/go-cid"
)

var bigBlockGasLimit = big.NewInt(build.BlockGasLimit)

const MaxBlocks = 15

type msgChain struct {
	msgs      []*types.SignedMessage
	gasReward *big.Int
	gasLimit  int64
	gasPerf   float64
	effPerf   float64
	valid     bool
	merged    bool
	next      *msgChain
	prev      *msgChain
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

	// 0. Load messages from the targeti tipset; if it is the same as the current tipset in
	//    the mpoll, then this is just the pending messages
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
	log.Infow("create message chains done", "took", time.Since(startChains))

	// 2. Sort the chains
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	if len(chains) != 0 && chains[0].gasPerf < 0 {
		log.Warnw("all messages in mpool have non-positive gas performance", "bestGasPerf", chains[0].gasPerf)
		return nil, nil
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
			partitions[i] = append(partitions[i], chain)
			nextChain++
			gasLimit -= chain.gasLimit
			if gasLimit < minGas {
				break
			}
		}
	}

	// 4. Compute effective performance for each chain, based on the partition they fall into
	//    The effective performance is the gasPerf of the chain * block probability
	//    Note that we don't have to do anything special about residues that didn't fit in any
	//    partition because these will already have an effective perf of 0 and will be pushed
	//    to the end by virtue of smaller gasPerf.
	blockProb := mp.blockProbabilities(tq)
	for i := 0; i < MaxBlocks; i++ {
		for _, chain := range partitions[i] {
			chain.SetEffectivePerf(blockProb[i])
		}
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
		if chain.gasPerf < 0 {
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
			result = append(result, chain.msgs...)
			gasLimit -= chainGasLimit
			continue
		}

		// we can't fit this chain and its dependencies because of block gasLimit -- we are
		// at the edge
		last = i
		break
	}
	log.Infow("merge message chains done", "took", time.Since(startMerge))

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
			chains[last].Trim(gasLimit, mp, baseFee, ts, false)
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
			if chain.gasPerf < 0 {
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
			chain.Trim(gasLimit-depGasLimit, mp, baseFee, ts, false)
			last += i
			continue tailLoop
		}

		// the merge loop ended after processing all the chains and we we probably have still
		// gas to spare; end the loop.
		break
	}
	log.Infow("pack tail chains done", "took", time.Since(startTail))

	return result, nil
}

func (mp *MessagePool) blockProbabilities(tq float64) []float64 {
	// TODO FIXME fit in the actual probability distribution
	//      this just makes a dummy random distribution for testing purposes
	bps := make([]float64, MaxBlocks)
	norm := 0.0
	for i := 0; i < MaxBlocks; i++ {
		p := rand.Float64()
		bps[i] = p
		norm += p
	}
	// normalize to make it a distribution
	for i := 0; i < MaxBlocks; i++ {
		bps[i] /= norm
	}

	return bps
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
	log.Infow("create message chains done", "took", time.Since(startChains))

	// 2. Sort the chains
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	if len(chains) != 0 && chains[0].gasPerf < 0 {
		log.Warnw("all messages in mpool have non-positive gas performance", "bestGasPerf", chains[0].gasPerf)
		return nil, nil
	}

	// 3. Merge the head chains to produce the list of messages selected for inclusion, subject to
	//    the block gas limit.
	startMerge := time.Now()
	last := len(chains)
	for i, chain := range chains {
		// did we run out of performing chains?
		if chain.gasPerf < 0 {
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
	log.Infow("merge message chains done", "took", time.Since(startMerge))

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
		chains[last].Trim(gasLimit, mp, baseFee, ts, false)

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
			if chain.gasPerf < 0 {
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
	log.Infow("pack tail chains done", "took", time.Since(startTail))

	return result, nil
}

func (mp *MessagePool) selectPriorityMessages(pending map[address.Address]map[uint64]*types.SignedMessage, baseFee types.BigInt, ts *types.TipSet) ([]*types.SignedMessage, int64) {
	start := time.Now()
	defer func() {
		log.Infow("select priority messages done", "took", time.Since(start))
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

	// 2. Sort the chains
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	// 3. Merge chains until the block limit; we are willing to include negative performing chains
	//    as these are messages from our own miners
	last := len(chains)
	for i, chain := range chains {
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
		// trim, without discarding negative performing messages
		chains[last].Trim(gasLimit, mp, baseFee, ts, true)

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
			last += i
			continue tailLoop
		}

		// the merge loop ended after processing all the chains and we probably still have gas to spare
		// -- mark the end.
		last = len(chains)
	}

	return result, gasLimit
}

func (mp *MessagePool) getPendingMessages(curTs, ts *types.TipSet) (map[address.Address]map[uint64]*types.SignedMessage, error) {
	start := time.Now()

	result := make(map[address.Address]map[uint64]*types.SignedMessage)
	haveCids := make(map[cid.Cid]struct{})
	defer func() {
		if time.Since(start) > time.Millisecond {
			log.Infow("get pending messages done", "took", time.Since(start))
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

			// mark the messages as seen
			for _, m := range mset.msgs {
				haveCids[m.Cid()] = struct{}{}
			}
		}
	}

	// we are in sync, that's the happy path
	if inSync {
		return result, nil
	}

	// nope, we need to sync the tipsets
	for {
		if curTs.Height() == ts.Height() {
			if curTs.Equals(ts) {
				return result, nil
			}

			// different blocks in tipsets -- we mark them as seen so that they are not included in
			// in the message set we return, but *neither me (vyzo) nor why understand why*
			// this code is also probably completely untested in production, so I am adding a big fat
			// warning to revisit this case and sanity check this decision.
			log.Warnf("mpool tipset has same height as target tipset but it's not equal; beware of dragons!")

			have, err := mp.MessagesForBlocks(ts.Blocks())
			if err != nil {
				return nil, xerrors.Errorf("error retrieving messages for tipset: %w", err)
			}

			for _, m := range have {
				haveCids[m.Cid()] = struct{}{}
			}
		}

		msgs, err := mp.MessagesForBlocks(ts.Blocks())
		if err != nil {
			return nil, xerrors.Errorf("error retrieving messages for tipset: %w", err)
		}

		for _, m := range msgs {
			if _, have := haveCids[m.Cid()]; have {
				continue
			}

			haveCids[m.Cid()] = struct{}{}
			mset, ok := result[m.Message.From]
			if !ok {
				mset = make(map[uint64]*types.SignedMessage)
				result[m.Message.From] = mset
			}

			other, dupNonce := mset[m.Message.Nonce]
			if dupNonce {
				// duplicate nonce, selfishly keep the message with the highest GasPrice
				// if the gas prices are the same, keep the one with the highest GasLimit
				switch m.Message.GasPremium.Int.Cmp(other.Message.GasPremium.Int) {
				case 0:
					if m.Message.GasLimit > other.Message.GasLimit {
						mset[m.Message.Nonce] = m
					}
				case 1:
					mset[m.Message.Nonce] = m
				}
			} else {
				mset[m.Message.Nonce] = m
			}
		}

		if curTs.Height() >= ts.Height() {
			return result, nil
		}

		ts, err = mp.api.LoadTipSet(ts.Parents())
		if err != nil {
			return nil, xerrors.Errorf("error loading parent tipset: %w", err)
		}
	}
}

func (mp *MessagePool) getGasReward(msg *types.SignedMessage, baseFee types.BigInt, ts *types.TipSet) *big.Int {
	gasReward := abig.Mul(msg.Message.GasPremium, types.NewInt(uint64(msg.Message.GasLimit)))
	maxReward := types.BigSub(msg.Message.GasFeeCap, baseFee)
	if types.BigCmp(maxReward, gasReward) < 0 {
		gasReward = maxReward
	}
	return gasReward.Int
}

func (mp *MessagePool) getGasPerf(gasReward *big.Int, gasLimit int64) float64 {
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
	a, _ := mp.api.StateGetActor(actor, ts)
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

		gasReward := mp.getGasReward(m, baseFee, ts)
		rewards = append(rewards, gasReward)
	}

	// check we have a sane set of messages to construct the chains
	if i > 0 {
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

func (mc *msgChain) Trim(gasLimit int64, mp *MessagePool, baseFee types.BigInt, ts *types.TipSet, priority bool) {
	i := len(mc.msgs) - 1
	for i >= 0 && (mc.gasLimit > gasLimit || (!priority && mc.gasPerf < 0)) {
		gasReward := mp.getGasReward(mc.msgs[i], baseFee, ts)
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
	mc.effPerf = mc.gasPerf * bp
}

func (mc *msgChain) BeforeEffective(other *msgChain) bool {
	return mc.effPerf > other.effPerf ||
		(mc.effPerf == other.effPerf && mc.gasPerf > other.gasPerf) ||
		(mc.effPerf == other.effPerf && mc.gasPerf == other.gasPerf && mc.gasReward.Cmp(other.gasReward) > 0)
}
