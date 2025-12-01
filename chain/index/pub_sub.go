package index

import "context"

type updateSub struct {
	ctx    context.Context
	cancel context.CancelFunc

	ch chan chainIndexUpdated
}

type chainIndexUpdated struct{}

func (si *SqliteIndexer) subscribeUpdates() (chan chainIndexUpdated, func()) {
	subCtx, subCancel := context.WithCancel(si.ctx)
	ch := make(chan chainIndexUpdated)

	si.mu.Lock()
	subId := si.subIdCounter
	si.subIdCounter++
	si.updateSubs[subId] = &updateSub{
		ctx:    subCtx,
		cancel: subCancel,
		ch:     ch,
	}
	si.mu.Unlock()

	unSubscribeF := func() {
		si.mu.Lock()
		if sub, ok := si.updateSubs[subId]; ok {
			sub.cancel()
			delete(si.updateSubs, subId)
		}
		si.mu.Unlock()
	}

	return ch, unSubscribeF
}

func (si *SqliteIndexer) notifyUpdateSubs() {
	si.mu.Lock()
	tSubs := make([]*updateSub, 0, len(si.updateSubs))
	for _, tSub := range si.updateSubs {
		tSubs = append(tSubs, tSub)
	}
	si.mu.Unlock()

	for _, tSub := range tSubs {
		select {
		case tSub.ch <- chainIndexUpdated{}:
		case <-tSub.ctx.Done():
			// subscription was cancelled, ignore
		case <-si.ctx.Done():
			return
		}
	}
}
