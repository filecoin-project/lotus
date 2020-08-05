package messagepool

import (
	"context"
	"sort"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

func (mp *MessagePool) pruneExcessMessages() error {
	mp.curTsLk.Lock()
	ts := mp.curTs
	mp.curTsLk.Unlock()

	mp.lk.Lock()
	defer mp.lk.Unlock()

	if mp.currentSize < mp.maxTxPoolSizeHi {
		return nil
	}

	return mp.pruneMessages(context.TODO(), ts)
}

func (mp *MessagePool) pruneMessages(ctx context.Context, ts *types.TipSet) error {
	start := time.Now()
	defer func() {
		log.Infof("message pruning took %s", time.Since(start))
	}()

	// Collect all messages to track which ones to remove and create chains for block inclusion
	pruneMsgs := make(map[cid.Cid]*types.SignedMessage, mp.currentSize)
	var chains []*msgChain
	for actor, mset := range mp.pending {
		for _, m := range mset.msgs {
			pruneMsgs[m.Message.Cid()] = m
		}
		actorChains := mp.createMessageChains(actor, mset, ts)
		chains = append(chains, actorChains...)
	}

	// Sort the chains
	sort.Slice(chains, func(i, j int) bool {
		return chains[i].Before(chains[j])
	})

	// Keep messages (remove them from pruneMsgs) from chains while we are under the low water mark
	keepCount := 0
keepLoop:
	for _, chain := range chains {
		for _, m := range chain.msgs {
			if keepCount < MemPoolSizeLimitLoDefault {
				delete(pruneMsgs, m.Message.Cid())
				keepCount++
			} else {
				break keepLoop
			}
		}
	}

	// and remove all messages that are still in pruneMsgs after processing the chains
	log.Infof("Pruning %d messages", len(pruneMsgs))
	for _, m := range pruneMsgs {
		mp.remove(m.Message.From, m.Message.Nonce)
	}

	return nil
}
