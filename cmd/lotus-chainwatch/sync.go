package main

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"math"
	"sync"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	parmap "github.com/filecoin-project/lotus/lib/parmap"
)

func runSyncer(ctx context.Context, api api.FullNode, st *storage, maxBatch int) {
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
					syncHead(ctx, api, st, change.Val, maxBatch)
				case store.HCRevert:
					log.Warnf("revert todo")
				}

				if change.Type == store.HCCurrent {
					go subMpool(ctx, api, st)
					go subBlocks(ctx, api, st)
				}
			}
		}
	}()
}

type minerKey struct {
	addr      address.Address
	act       types.Actor
	stateroot cid.Cid
	tsKey     types.TipSetKey
}

type minerInfo struct {
	state miner.State
	info  miner.MinerInfo

	power big.Int
	ssize uint64
	psize uint64
}

type actorInfo struct {
	stateroot cid.Cid
	tsKey     types.TipSetKey
	state     string
}

func syncHead(ctx context.Context, api api.FullNode, st *storage, headTs *types.TipSet, maxBatch int) {
	var alk sync.Mutex

	log.Infof("Getting synced block list")

	hazlist := st.hasList()

	log.Infof("Getting headers / actors")

	allToSync := map[cid.Cid]*types.BlockHeader{}
	toVisit := list.New()

	for _, header := range headTs.Blocks() {
		toVisit.PushBack(header)
	}

	for toVisit.Len() > 0 {
		bh := toVisit.Remove(toVisit.Back()).(*types.BlockHeader)

		_, has := hazlist[bh.Cid()]
		if _, seen := allToSync[bh.Cid()]; seen || has {
			continue
		}

		allToSync[bh.Cid()] = bh

		if len(allToSync)%500 == 10 {
			log.Infof("todo: (%d) %s @%d", len(allToSync), bh.Cid(), bh.Height)
		}

		if len(bh.Parents) == 0 {
			continue
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

	for len(allToSync) > 0 {
		actors := map[address.Address]map[types.Actor]actorInfo{}
		addressToID := map[address.Address]address.Address{}
		minH := abi.ChainEpoch(math.MaxInt64)

		for _, header := range allToSync {
			if header.Height < minH {
				minH = header.Height
			}
		}

		toSync := map[cid.Cid]*types.BlockHeader{}
		for c, header := range allToSync {
			if header.Height < minH+abi.ChainEpoch(maxBatch) {
				toSync[c] = header
				addressToID[header.Miner] = address.Undef
			}
		}
		for c := range toSync {
			delete(allToSync, c)
		}

		log.Infof("Syncing %d blocks", len(toSync))

		paDone := 0
		parmap.Par(50, parmap.MapArr(toSync), func(bh *types.BlockHeader) {
			paDone++
			if paDone%100 == 0 {
				log.Infof("pa: %d %d%%", paDone, (paDone*100)/len(toSync))
			}

			if len(bh.Parents) == 0 { // genesis case
				genesisTs, _ := types.NewTipSet([]*types.BlockHeader{bh})
				aadrs, err := api.StateListActors(ctx, genesisTs.Key())
				if err != nil {
					log.Error(err)
					return
				}

				parmap.Par(50, aadrs, func(addr address.Address) {
					act, err := api.StateGetActor(ctx, addr, genesisTs.Key())
					if err != nil {
						log.Error(err)
						return
					}

					ast, err := api.StateReadState(ctx, addr, genesisTs.Key())
					if err != nil {
						log.Error(err)
						return
					}
					state, err := json.Marshal(ast.State)
					if err != nil {
						log.Error(err)
						return
					}

					alk.Lock()
					_, ok := actors[addr]
					if !ok {
						actors[addr] = map[types.Actor]actorInfo{}
					}
					actors[addr][*act] = actorInfo{
						stateroot: bh.ParentStateRoot,
						tsKey:     genesisTs.Key(),
						state:     string(state),
					}
					addressToID[addr] = address.Undef
					alk.Unlock()
				})

				return
			}

			pts, err := api.ChainGetTipSet(ctx, types.NewTipSetKey(bh.Parents...))
			if err != nil {
				log.Error(err)
				return
			}

			changes, err := api.StateChangedActors(ctx, pts.ParentState(), bh.ParentStateRoot)
			if err != nil {
				log.Error(err)
				return
			}

			for a, act := range changes {
				act := act

				addr, err := address.NewFromString(a)
				if err != nil {
					log.Error(err)
					return
				}

				ast, err := api.StateReadState(ctx, addr, pts.Key())

				if err != nil {
					log.Error(err)
					return
				}

				state, err := json.Marshal(ast.State)
				if err != nil {
					log.Error(err)
					return
				}

				alk.Lock()
				_, ok := actors[addr]
				if !ok {
					actors[addr] = map[types.Actor]actorInfo{}
				}
				actors[addr][act] = actorInfo{
					stateroot: bh.ParentStateRoot,
					state:     string(state),
					tsKey:     pts.Key(),
				}
				addressToID[addr] = address.Undef
				alk.Unlock()
			}
		})

		log.Infof("Getting messages")

		msgs, incls := fetchMessages(ctx, api, toSync)

		log.Infof("Resolving addresses")

		for _, message := range msgs {
			addressToID[message.To] = address.Undef
			addressToID[message.From] = address.Undef
		}

		parmap.Par(50, parmap.KMapArr(addressToID), func(addr address.Address) {
			// FIXME: cannot use EmptyTSK here since actorID's can change during reorgs, need to use the corresponding tipset.
			// TODO: figure out a way to get the corresponding tipset...
			raddr, err := api.StateLookupID(ctx, addr, types.EmptyTSK)
			if err != nil {
				log.Warn(err)
				return
			}
			alk.Lock()
			addressToID[addr] = raddr
			alk.Unlock()
		})

		log.Infof("Getting miner info")

		miners := map[minerKey]*minerInfo{}

		for addr, m := range actors {
			for actor, c := range m {
				if actor.Code != builtin.StorageMinerActorCodeID {
					continue
				}

				miners[minerKey{
					addr:      addr,
					act:       actor,
					stateroot: c.stateroot,
					tsKey:     c.tsKey,
				}] = &minerInfo{}
			}
		}

		parmap.Par(50, parmap.KVMapArr(miners), func(it func() (minerKey, *minerInfo)) {
			k, info := it()

			// TODO: get the storage power actors state and and pull the miner power from there, currently this hits the
			// storage power actor once for each miner for each tipset, we can do better by just getting it for each tipset
			// and reading each miner power from the result.
			pow, err := api.StateMinerPower(ctx, k.addr, k.tsKey)
			if err != nil {
				log.Error(err)
				// Not sure why this would fail, but its probably worth continuing
			}
			info.power = pow.MinerPower.QualityAdjPower

			sszs, err := api.StateMinerSectorCount(ctx, k.addr, k.tsKey)
			if err != nil {
				log.Error(err)
				return
			}
			info.psize = sszs.Pset
			info.ssize = sszs.Sset

			astb, err := api.ChainReadObj(ctx, k.act.Head)
			if err != nil {
				log.Error(err)
				return
			}

			if err := info.state.UnmarshalCBOR(bytes.NewReader(astb)); err != nil {
				log.Error(err)
				return
			}

			info.info = info.state.Info
		})

		log.Info("Getting receipts")

		receipts := fetchParentReceipts(ctx, api, toSync)

		log.Info("Storing headers")

		if err := st.storeHeaders(toSync, true); err != nil {
			log.Errorf("%+v", err)
			return
		}

		log.Info("Storing address mapping")

		if err := st.storeAddressMap(addressToID); err != nil {
			log.Error(err)
			return
		}

		log.Info("Storing actors")

		if err := st.storeActors(actors); err != nil {
			log.Error(err)
			return
		}

		log.Info("Storing miners")

		if err := st.storeMiners(miners); err != nil {
			log.Error(err)
			return
		}

		log.Infof("Storing messages")

		if err := st.storeMessages(msgs); err != nil {
			log.Error(err)
			return
		}

		log.Info("Storing message inclusions")

		if err := st.storeMsgInclusions(incls); err != nil {
			log.Error(err)
			return
		}

		log.Infof("Storing parent receipts")

		if err := st.storeReceipts(receipts); err != nil {
			log.Error(err)
			return
		}
		log.Infof("Sync stage done")
	}

	log.Infof("Get deals")

	// TODO: incremental, gather expired
	deals, err := api.StateMarketDeals(ctx, headTs.Key())
	if err != nil {
		log.Error(err)
		return
	}

	log.Infof("Store deals")

	if err := st.storeDeals(deals); err != nil {
		log.Error(err)
		return
	}

	log.Infof("Refresh views")

	if err := st.refreshViews(); err != nil {
		log.Error(err)
		return
	}

	log.Infof("Sync done")
}

func fetchMessages(ctx context.Context, api api.FullNode, toSync map[cid.Cid]*types.BlockHeader) (map[cid.Cid]*types.Message, map[cid.Cid][]cid.Cid) {
	var lk sync.Mutex
	messages := map[cid.Cid]*types.Message{}
	inclusions := map[cid.Cid][]cid.Cid{} // block -> msgs

	parmap.Par(50, parmap.MapArr(toSync), func(header *types.BlockHeader) {
		msgs, err := api.ChainGetBlockMessages(ctx, header.Cid())
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
			inclusions[header.Cid()] = append(inclusions[header.Cid()], message.Cid())
		}
		lk.Unlock()
	})

	return messages, inclusions
}

type mrec struct {
	msg   cid.Cid
	state cid.Cid
	idx   int
}

func fetchParentReceipts(ctx context.Context, api api.FullNode, toSync map[cid.Cid]*types.BlockHeader) map[mrec]*types.MessageReceipt {
	var lk sync.Mutex
	out := map[mrec]*types.MessageReceipt{}

	parmap.Par(50, parmap.MapArr(toSync), func(header *types.BlockHeader) {
		recs, err := api.ChainGetParentReceipts(ctx, header.Cid())
		if err != nil {
			log.Error(err)
			return
		}
		msgs, err := api.ChainGetParentMessages(ctx, header.Cid())
		if err != nil {
			log.Error(err)
			return
		}

		lk.Lock()
		for i, r := range recs {
			out[mrec{
				msg:   msgs[i].Cid,
				state: header.ParentStateRoot,
				idx:   i,
			}] = r
		}
		lk.Unlock()
	})

	return out
}
