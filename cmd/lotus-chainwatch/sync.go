package main

import (
	"container/list"
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

func runSyncer(ctx context.Context, api api.FullNode, st *storage) {
	notifs, err := api.ChainNotify(ctx)
	if err != nil {
		panic(err)
	}
	go func() {
		for notif := range notifs {
			for _, change := range notif {
				switch change.Type {
				case store.HCCurrent:
					fallthrough
				case store.HCApply:
					syncHead(ctx, api, st, change.Val)
				case store.HCRevert:
					log.Warnf("revert todo")
				}
			}
		}
	}()
}

func syncHead(ctx context.Context, api api.FullNode, st *storage, ts *types.TipSet) {
	var toSync []*types.BlockHeader
	toVisit := list.New()

	for _, header := range ts.Blocks() {
		toVisit.PushBack(header)
	}

	for toVisit.Len() > 0 {
		bh := toVisit.Remove(toVisit.Back()).(*types.BlockHeader)

		if !st.hasBlock(bh) {
			toSync = append(toSync, bh)
		}
		if len(toSync)%500 == 0 {
			log.Infof("todo: (%d) %s", len(toSync), bh.Cid())
		}

		if len(bh.Parents) == 0 {
			break
		}

		pts, err := api.ChainGetTipSet(ctx, types.NewTipSetKey(bh.Parents...))
		if err != nil {
			log.Error(err)
			continue
		}

		for _, header := range pts.Blocks() {
			toVisit.PushBack(header)
		}
	}

	log.Infof("Syncing %d blocks", len(toSync))

	log.Infof("Persisting headers")
	if err := st.storeHeaders(toSync); err != nil {
		log.Error(err)
		return
	}

	log.Infof("Getting messages")

	msgs, incls := fetchMessages(ctx, api, toSync)

	if err := st.storeMessages(msgs); err != nil {
		log.Error(err)
		return
	}

	if err := st.storeMsgInclusions(incls); err != nil {
		log.Error(err)
		return
	}

	log.Infof("Getting actors")
	// TODO: for now this assumes that actor can't be removed

	/*	aadrs, err := api.StateListActors(ctx, ts)
		if err != nil {
			return
		}*/

	log.Infof("Sync done")
}

func fetchMessages(ctx context.Context, api api.FullNode, toSync []*types.BlockHeader) (map[cid.Cid]*types.Message, map[cid.Cid][]cid.Cid) {
	var lk sync.Mutex
	messages := map[cid.Cid]*types.Message{}
	inclusions := map[cid.Cid][]cid.Cid{} // block -> msgs

	throttle := make(chan struct{}, 50)
	var wg sync.WaitGroup

	for _, header := range toSync {
		if header.Height%30 == 0 {
			fmt.Printf("\rh: %d", header.Height)
		}

		throttle <- struct{}{}
		wg.Add(1)

		go func(header cid.Cid) {
			defer wg.Done()
			defer func() {
				<-throttle
			}()

			msgs, err := api.ChainGetBlockMessages(ctx, header)
			if err != nil {
				log.Error(err)
				return
			}

			vmm := make([]*types.Message, 0, len(msgs.Cids))
			for _, m := range msgs.BlsMessages {
				vmm = append(vmm, m)
			}

			for _, m := range msgs.SecpkMessages {
				vmm = append(vmm, &m.Message)
			}

			lk.Lock()
			for _, message := range vmm {
				messages[message.Cid()] = message
				inclusions[header] = append(inclusions[header], message.Cid())
			}
			lk.Unlock()

		}(header.Cid())
	}
	wg.Wait()

	return messages, inclusions
}
