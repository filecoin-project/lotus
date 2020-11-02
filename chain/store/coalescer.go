package store

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
)

func WrapHeadChangeCoalescer(fn ReorgNotifee, delay time.Duration) ReorgNotifee {
	c := NewHeadChangeCoalescer(fn, delay)
	return c.HeadChange
}

type HeadChangeCoalescer struct {
	notify ReorgNotifee

	ctx    context.Context
	cancel func()

	eventq chan headChange

	revert []*types.TipSet
	apply  []*types.TipSet
}

type headChange struct {
	revert, apply []*types.TipSet
}

func NewHeadChangeCoalescer(fn ReorgNotifee, delay time.Duration) *HeadChangeCoalescer {
	ctx, cancel := context.WithCancel(context.Background())
	c := &HeadChangeCoalescer{
		notify: fn,
		ctx:    ctx,
		cancel: cancel,
		eventq: make(chan headChange),
	}

	go c.background(delay)

	return c
}

func (c *HeadChangeCoalescer) HeadChange(revert, apply []*types.TipSet) error {
	select {
	case c.eventq <- headChange{revert: revert, apply: apply}:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *HeadChangeCoalescer) Close() {
	select {
	case <-c.ctx.Done():
	default:
		c.cancel()
	}
}

func (c *HeadChangeCoalescer) background(delay time.Duration) {
	var timerC <-chan time.Time
	for {
		select {
		case evt := <-c.eventq:
			c.coalesce(evt.revert, evt.apply)
			if timerC == nil {
				timerC = time.After(delay)
			}

		case <-timerC:
			c.dispatch()
			timerC = nil

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *HeadChangeCoalescer) coalesce(revert, apply []*types.TipSet) {
	// newly reverted tipsets cancel out pending applied tipsets
	// we iterate through the revert set and if a tipset is pending for apply we cancel it.

	// pending tipsets for apply
	applied := make(map[types.TipSetKey]struct{})
	for _, ts := range c.apply {
		applied[ts.Key()] = struct{}{}
	}

	// freshly reverted tipsets from the pending applied set
	reverted := make(map[types.TipSetKey]struct{})

	for _, ts := range revert {
		key := ts.Key()

		_, ok := applied[key]
		if ok {
			reverted[key] = struct{}{}
			continue
		}

		c.revert = append(c.revert, ts)
	}

	newApply := make([]*types.TipSet, 0, len(c.apply)-len(reverted)+len(apply))
	for _, ts := range c.apply {
		_, ok := reverted[ts.Key()]
		if ok {
			continue
		}

		newApply = append(newApply, ts)
	}

	newApply = append(newApply, apply...)
	c.apply = newApply
}

func (c *HeadChangeCoalescer) dispatch() {
	err := c.notify(c.revert, c.apply)
	if err != nil {
		log.Errorf("error dispatching coalesced head change notification: %s", err)
	}

	c.revert = nil
	c.apply = nil
}
