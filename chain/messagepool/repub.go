package messagepool

import (
	"context"
	"sort"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

const repubMsgLimit = 5

func (mp *MessagePool) republishPendingMessages() error {
	mp.curTsLk.Lock()
	ts := mp.curTs
	mp.curTsLk.Unlock()

	baseFee, err := mp.api.ChainComputeBaseFee(context.TODO(), ts)
	if err != nil {
		return xerrors.Errorf("computing basefee: %w", err)
	}

	pending := make(map[address.Address]map[uint64]*types.SignedMessage)
	mp.lk.Lock()
	for actor := range mp.localAddrs {
		mset, ok := mp.pending[actor]
		if !ok {
			continue
		}
		if len(mset.msgs) == 0 {
			continue
		}
		// we need to copy this while holding the lock to avoid races with concurrent modification
		pend := make(map[uint64]*types.SignedMessage, len(mset.msgs))
		for nonce, m := range mset.msgs {
			pend[nonce] = m
		}
		pending[actor] = pend
	}
	mp.lk.Unlock()

	if len(pending) == 0 {
		return nil
	}

	var chains []*msgChain
	for actor, mset := range pending {
		next := mp.createMessageChains(actor, mset, baseFee, ts)
		chains = append(chains, next...)
	}

	if len(chains) == 0 {
		return nil
	}

	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	// we don't republish negative performing chains; this is an error that will be screamed
	// at the user
	if chains[0].gasPerf < 0 {
		return xerrors.Errorf("skipping republish: all message chains have negative gas performance; best gas performance: %f", chains[0].gasPerf)
	}

	gasLimit := int64(build.BlockGasLimit)
	var msgs []*types.SignedMessage
	for _, chain := range chains {
		// we can exceed this if we have picked (some) longer chain already
		if len(msgs) > repubMsgLimit {
			break
		}

		// we don't republish negative performing chains, as they won't be included in
		// a block anyway
		if chain.gasPerf < 0 {
			break
		}

		// we don't exceed the block gasLimit in our republish endeavor
		if chain.gasLimit > gasLimit {
			break
		}

		gasLimit -= chain.gasLimit
		msgs = append(msgs, chain.msgs...)
	}

	log.Infof("republishing %d messages", len(msgs))
	for _, m := range msgs {
		mb, err := m.Serialize()
		if err != nil {
			return xerrors.Errorf("cannot serialize message: %w", err)
		}

		err = mp.api.PubSubPublish(build.MessagesTopic(mp.netName), mb)
		if err != nil {
			return xerrors.Errorf("cannot publish: %w", err)
		}
	}

	return nil
}
