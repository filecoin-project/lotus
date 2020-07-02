package main

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

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

type minerStateInfo struct {
	// common
	addr      address.Address
	act       types.Actor
	stateroot cid.Cid

	// miner specific
	state miner.State
	info  miner.MinerInfo

	// tracked by power actor
	rawPower big.Int
	qalPower big.Int
	ssize    uint64
	psize    uint64
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

	// global list of all blocks that need to be synced
	allToSync := map[cid.Cid]*types.BlockHeader{}
	// a stack
	toVisit := list.New()

	for _, header := range headTs.Blocks() {
		toVisit.PushBack(header)
	}

	// TODO consider making a db query to check where syncing left off at in the case of a restart and avoid reprocessing
	// those entries, or write value to file on shutdown
	// walk the entire chain starting from headTS
	for toVisit.Len() > 0 {
		bh := toVisit.Remove(toVisit.Back()).(*types.BlockHeader)
		_, has := hazlist[bh.Cid()]
		if _, seen := allToSync[bh.Cid()]; seen || has {
			continue
		}

		allToSync[bh.Cid()] = bh
		if len(allToSync)%500 == 10 {
			log.Debugf("to visit: (%d) %s @%d", len(allToSync), bh.Cid(), bh.Height)
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

	// Main worker loop, this loop runs until all tipse from headTS to genesis have been processed.
	for len(allToSync) > 0 {
		// first map is addresses -> common actors states (head, code, balance, nonce)
		// second map common actor states -> chain state (tipset, stateroot) & unique actor state (deserialization of their head CID) represented as json.
		actors := map[address.Address]map[types.Actor]actorInfo{}

		// map of actor public key address to ID address
		addressToID := map[address.Address]address.Address{}
		minH := abi.ChainEpoch(math.MaxInt64)

		// find the blockheader with the lowest height
		for _, header := range allToSync {
			if header.Height < minH {
				minH = header.Height
			}
		}

		// toSync maps block cids to their headers and contains all block headers that will be synced in this batch
		// `maxBatch` is a tunable parameter to control how many blocks we sync per iteration.
		toSync := map[cid.Cid]*types.BlockHeader{}
		for c, header := range allToSync {
			if header.Height < minH+abi.ChainEpoch(maxBatch) {
				toSync[c] = header
				addressToID[header.Miner] = address.Undef
			}
		}
		// remove everything we are syncing this round from the global list of blocks to sync
		for c := range toSync {
			delete(allToSync, c)
		}

		log.Infow("Starting Sync", "height", minH, "numBlocks", len(toSync), "maxBatch", maxBatch)

		// map of addresses to changed actors
		var changes map[string]types.Actor
		// collect all actor state that has changes between block headers
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

				// TODO suspicious there is not a lot to be gained by doing this in parallel since the genesis state
				// is unlikely to contain a lot of actors, why not for loop here?
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

			// TODO Does this return actors that have been deleted between states?
			// collect all actors that had state changes between the blockheader parent-state and its grandparent-state.
			changes, err = api.StateChangedActors(ctx, pts.ParentState(), bh.ParentStateRoot)
			if err != nil {
				log.Error(err)
				return
			}

			// record the state of all actors that have changed
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
				// a change occurred for the actor with address `addr` and state `act` at tipset `pts`.
				actors[addr][act] = actorInfo{
					stateroot: bh.ParentStateRoot,
					state:     string(state),
					tsKey:     pts.Key(),
				}
				addressToID[addr] = address.Undef
				alk.Unlock()
			}
		})

		// map of tipset to all miners that had a head-change at that tipset.
		minerTips := make(map[types.TipSetKey][]*minerStateInfo, len(changes))
		// heads we've seen, im being paranoid
		headsSeen := make(map[cid.Cid]struct{}, len(actors))

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

		minerChanges := 0
		for addr, m := range actors {
			for actor, c := range m {
				if actor.Code != builtin.StorageMinerActorCodeID {
					continue
				}

				// only want miner actors with head change events
				if _, found := headsSeen[actor.Head]; found {
					continue
				}
				minerChanges++

				minerTips[c.tsKey] = append(minerTips[c.tsKey], &minerStateInfo{
					addr:      addr,
					act:       actor,
					stateroot: c.stateroot,

					state: miner.State{},
					info:  miner.MinerInfo{},

					rawPower: big.Zero(),
					qalPower: big.Zero(),
				})

				headsSeen[actor.Head] = struct{}{}
			}
		}

		minerProcessingStartedAt := time.Now()
		log.Infow("Processing miners", "numTips", len(minerTips), "numMinerChanges", minerChanges)
		// extract the power actor state at each tipset, loop over all miners that changed at said tipset and extract their
		// claims from the power actor state. This ensures we only fetch the power actors state once for each tipset.
		parmap.Par(50, parmap.KVMapArr(minerTips), func(it func() (types.TipSetKey, []*minerStateInfo)) {
			tsKey, minerInfo := it()

			// get the power actors claims map
			mp, err := getPowerActorClaimsMap(ctx, api, tsKey)
			if err != nil {
				log.Error(err)
				return
			}
			// Get miner raw and quality power
			for _, mi := range minerInfo {
				var claim power.Claim
				// get miner claim from power actors claim map and store if found, else the miner had no claim at
				// this tipset
				found, err := mp.Get(adt.AddrKey(mi.addr), &claim)
				if err != nil {
					log.Error(err)
				}
				if found {
					mi.qalPower = claim.QualityAdjPower
					mi.rawPower = claim.RawBytePower
				}

				// Get the miner state info
				astb, err := api.ChainReadObj(ctx, mi.act.Head)
				if err != nil {
					log.Error(err)
					return
				}
				if err := mi.state.UnmarshalCBOR(bytes.NewReader(astb)); err != nil {
					log.Error(err)
					return
				}
				mi.info = mi.state.Info
			}

			// TODO Get the Sector Count
			// FIXME this is returning a lot of "address not found" errors, which is strange given that StateChangedActors
			// retruns all actors that had a state change at tipset `k.tsKey`, maybe its returning deleted miners too??
			/*
				sszs, err := api.StateMinerSectorCount(ctx, k.addr, k.tsKey)
				if err != nil {
					info.psize = 0
					info.ssize = 0
				} else {
					info.psize = sszs.Pset
					info.ssize = sszs.Sset
				}
			*/
		})
		log.Infow("Completed Miner Processing", "duration", time.Since(minerProcessingStartedAt).String(), "processed", minerChanges)

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
		if err := st.storeMiners(minerTips); err != nil {
			log.Error(err)
			return
		}

		log.Info("Storing miner sectors")
		sectorStart := time.Now()
		if err := st.storeSectors(minerTips, api); err != nil {
			log.Error(err)
			return
		}
		log.Infow("Finished storing miner sectors", "duration", time.Since(sectorStart).String())

		log.Info("Storing miner sectors heads")
		if err := st.storeMinerSectorsHeads(minerTips, api); err != nil {
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

// load the power actor state clam as an adt.Map at the tipset `ts`.
func getPowerActorClaimsMap(ctx context.Context, api api.FullNode, ts types.TipSetKey) (*adt.Map, error) {
	powerActor, err := api.StateGetActor(ctx, builtin.StoragePowerActorAddr, ts)
	if err != nil {
		return nil, err
	}

	powerRaw, err := api.ChainReadObj(ctx, powerActor.Head)
	if err != nil {
		return nil, err
	}

	var powerActorState power.State
	if err := powerActorState.UnmarshalCBOR(bytes.NewReader(powerRaw)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal power actor state: %w", err)
	}

	s := &apiIpldStore{ctx, api}
	return adt.AsMap(s, powerActorState.Claims)
}

// require for AMT and HAMT access
// TODO extract this to a common location in lotus and reuse the code
type apiIpldStore struct {
	ctx context.Context
	api api.FullNode
}

func (ht *apiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *apiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	raw, err := ht.api.ChainReadObj(ctx, c)
	if err != nil {
		return err
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("Object does not implement CBORUnmarshaler: %T", out)
}

func (ht *apiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return cid.Undef, fmt.Errorf("Put is not implemented on apiIpldStore")
}
