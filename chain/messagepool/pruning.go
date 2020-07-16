package messagepool

import (
	"time"

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
}
