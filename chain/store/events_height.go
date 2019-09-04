package store

import (
	"github.com/filecoin-project/go-lotus/chain/types"
)

func (e *Events) headChangeAt(rev, app []*types.TipSet) error {
	// highest tipset is always the first (see cs.ReorgOps)
	newH := app[0].Height()

	for _, ts := range rev {
		// TODO: log error if h below gcconfidence
		// revert height-based triggers

		for _, tid := range e.htHeights[ts.Height()] {
			// don't revert if newH is above this ts
			if newH >= ts.Height() {
				continue
			}

			err := e.heightTriggers[tid].revert(ts)
			if err != nil {
				log.Errorf("reverting chain trigger (@H %d): %s", ts.Height(), err)
			}
		}

		if err := e.tsc.revert(ts); err != nil {
			return err
		}
	}

	for _, ts := range app {
		if err := e.tsc.add(ts); err != nil {
			return err
		}

		// height triggers

		for _, tid := range e.htTriggerHeights[ts.Height()] {
			hnd := e.heightTriggers[tid]
			triggerH := ts.Height() - uint64(hnd.confidence)

			incTs, err := e.tsc.get(triggerH)
			if err != nil {
				return err
			}

			if err := hnd.handle(incTs, ts.Height()); err != nil {
				msgInfo := ""
				log.Errorf("chain trigger (%s@H %d, called @ %d) failed: %s", msgInfo, triggerH, ts.Height(), err)
			}
		}
	}

	return nil
}

func (e *Events) ChainAt(hnd HeightHandler, rev RevertHandler, confidence int, h uint64) error {
	e.lk.Lock()
	defer e.lk.Unlock()

	bestH := e.tsc.best().Height()

	if bestH >= h+uint64(confidence) {
		ts, err := e.tsc.get(h)
		if err != nil {
			log.Warnf("events.ChainAt: calling HandleFunc with nil tipset, not found in cache: %s", err)
		}

		if err := hnd(ts, bestH); err != nil {
			return err
		}
	}

	if bestH >= h+uint64(confidence)+e.gcConfidence {
		return nil
	}

	triggerAt := h + uint64(confidence)

	id := e.ctr
	e.ctr++

	e.heightTriggers[id] = &heightHandler{
		confidence: confidence,

		handle: hnd,
		revert: rev,
	}

	e.htHeights[h] = append(e.htHeights[h], id)
	e.htTriggerHeights[triggerAt] = append(e.htTriggerHeights[triggerAt], id)

	return nil
}
