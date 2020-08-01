package messagepool

import (
	"sort"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"
)

func (mp *MessagePool) pruneExcessMessages() error {
	start := time.Now()
	defer func() {
		log.Infow("message pruning complete", "took", time.Since(start))
	}()

	mp.lk.Lock()
	defer mp.lk.Unlock()

	if mp.currentSize < mp.maxTxPoolSizeHi {
		return nil
	}

	pruneCount := mp.currentSize - mp.maxTxPoolSizeLo

	// Step 1. Remove all 'future' messages (those with a nonce gap)
	npruned, err := mp.pruneFutureMessages()
	if err != nil {
		return xerrors.Errorf("failed to prune future messages: %w", err)
	}

	pruneCount -= npruned
	if pruneCount <= 0 {
		return nil
	}

	// Step 2. prune messages with the lowest gas prices

	log.Warnf("still need to prune %d messages", pruneCount)
	return nil
}

func (mp *MessagePool) pruneFutureMessages() (int, error) {
	for addr, ms := range mp.pending {
		if _, ok := mp.localAddrs[addr]; ok {
			continue
		}

		act, err := mp.api.StateGetActor(addr, nil)
		if err != nil {
			return 0, err
		}

		allmsgs := make([]*types.SignedMessage, 0, len(ms.msgs))
		for _, m := range ms.msgs {
			allmsgs = append(allmsgs, m)
		}

		sort.Slice(allmsgs, func(i, j int) bool {
			return allmsgs[i].Message.Nonce < allmsgs[j].Message.Nonce
		})

		start := act.Nonce
		for i := 0; i < len(allmsgs); i++ {
			if allmsgs[i].Message.Nonce == start {
				start++
			} else {
				ms.nextNonce = start
				for ; i < len(allmsgs); i++ {
					delete(ms.msgs, allmsgs[i].Message.Nonce)
				}
				break
			}
		}

	}
}
