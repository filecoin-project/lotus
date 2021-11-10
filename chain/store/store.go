package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/metrics"

	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"go.uber.org/multierr"

	"github.com/filecoin-project/lotus/chain/types"

	lru "github.com/hashicorp/golang-lru"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/whyrusleeping/pubsub"
	"golang.org/x/xerrors"
)

var log = logging.Logger("chainstore")

var (
	chainHeadKey                  = dstore.NewKey("head")
	checkpointKey                 = dstore.NewKey("/chain/checks")
	blockValidationCacheKeyPrefix = dstore.NewKey("blockValidation")
)

var DefaultTipSetCacheSize = 8192
var DefaultMsgMetaCacheSize = 2048

var ErrNotifeeDone = errors.New("notifee is done and should be removed")

func init() {
	if s := os.Getenv("LOTUS_CHAIN_TIPSET_CACHE"); s != "" {
		tscs, err := strconv.Atoi(s)
		if err != nil {
			log.Errorf("failed to parse 'LOTUS_CHAIN_TIPSET_CACHE' env var: %s", err)
		}
		DefaultTipSetCacheSize = tscs
	}

	if s := os.Getenv("LOTUS_CHAIN_MSGMETA_CACHE"); s != "" {
		mmcs, err := strconv.Atoi(s)
		if err != nil {
			log.Errorf("failed to parse 'LOTUS_CHAIN_MSGMETA_CACHE' env var: %s", err)
		}
		DefaultMsgMetaCacheSize = mmcs
	}
}

// ReorgNotifee represents a callback that gets called upon reorgs.
type ReorgNotifee = func(rev, app []*types.TipSet) error

// Journal event types.
const (
	evtTypeHeadChange = iota
)

type HeadChangeEvt struct {
	From        types.TipSetKey
	FromHeight  abi.ChainEpoch
	To          types.TipSetKey
	ToHeight    abi.ChainEpoch
	RevertCount int
	ApplyCount  int
}

type WeightFunc func(ctx context.Context, stateBs bstore.Blockstore, ts *types.TipSet) (types.BigInt, error)

// ChainStore is the main point of access to chain data.
//
// Raw chain data is stored in the Blockstore, with relevant markers (genesis,
// latest head tipset references) being tracked in the Datastore (key-value
// store).
//
// To alleviate disk access, the ChainStore has two ARC caches:
//   1. a tipset cache
//   2. a block => messages references cache.
type ChainStore struct {
	chainBlockstore bstore.Blockstore
	stateBlockstore bstore.Blockstore
	metadataDs      dstore.Batching

	weight WeightFunc

	chainLocalBlockstore bstore.Blockstore

	heaviestLk sync.RWMutex
	heaviest   *types.TipSet
	checkpoint *types.TipSet

	bestTips *pubsub.PubSub
	pubLk    sync.Mutex

	tstLk   sync.Mutex
	tipsets map[abi.ChainEpoch][]cid.Cid

	cindex *ChainIndex

	reorgCh        chan<- reorg
	reorgNotifeeCh chan ReorgNotifee

	mmCache *lru.ARCCache // msg meta cache (mh.Messages -> secp, bls []cid)
	tsCache *lru.ARCCache

	evtTypes [1]journal.EventType
	journal  journal.Journal

	cancelFn context.CancelFunc
	wg       sync.WaitGroup
}

func NewChainStore(chainBs bstore.Blockstore, stateBs bstore.Blockstore, ds dstore.Batching, weight WeightFunc, j journal.Journal) *ChainStore {
	c, _ := lru.NewARC(DefaultMsgMetaCacheSize)
	tsc, _ := lru.NewARC(DefaultTipSetCacheSize)
	if j == nil {
		j = journal.NilJournal()
	}

	ctx, cancel := context.WithCancel(context.Background())
	// unwraps the fallback store in case one is configured.
	// some methods _need_ to operate on a local blockstore only.
	localbs, _ := bstore.UnwrapFallbackStore(chainBs)
	cs := &ChainStore{
		chainBlockstore:      chainBs,
		stateBlockstore:      stateBs,
		chainLocalBlockstore: localbs,
		weight:               weight,
		metadataDs:           ds,
		bestTips:             pubsub.New(64),
		tipsets:              make(map[abi.ChainEpoch][]cid.Cid),
		mmCache:              c,
		tsCache:              tsc,
		cancelFn:             cancel,
		journal:              j,
	}

	cs.evtTypes = [1]journal.EventType{
		evtTypeHeadChange: j.RegisterEventType("sync", "head_change"),
	}

	ci := NewChainIndex(cs.LoadTipSet)

	cs.cindex = ci

	hcnf := func(rev, app []*types.TipSet) error {
		cs.pubLk.Lock()
		defer cs.pubLk.Unlock()

		notif := make([]*api.HeadChange, len(rev)+len(app))

		for i, r := range rev {
			notif[i] = &api.HeadChange{
				Type: HCRevert,
				Val:  r,
			}
		}
		for i, r := range app {
			notif[i+len(rev)] = &api.HeadChange{
				Type: HCApply,
				Val:  r,
			}
		}

		cs.bestTips.Pub(notif, "headchange")
		return nil
	}

	hcmetric := func(rev, app []*types.TipSet) error {
		for _, r := range app {
			stats.Record(context.Background(), metrics.ChainNodeHeight.M(int64(r.Height())))
		}
		return nil
	}

	cs.reorgNotifeeCh = make(chan ReorgNotifee)
	cs.reorgCh = cs.reorgWorker(ctx, []ReorgNotifee{hcnf, hcmetric})

	return cs
}

func (cs *ChainStore) Close() error {
	cs.cancelFn()
	cs.wg.Wait()
	return nil
}

func (cs *ChainStore) Load() error {
	if err := cs.loadHead(); err != nil {
		return err
	}
	if err := cs.loadCheckpoint(); err != nil {
		return err
	}
	return nil
}
func (cs *ChainStore) loadHead() error {
	head, err := cs.metadataDs.Get(chainHeadKey)
	if err == dstore.ErrNotFound {
		log.Warn("no previous chain state found")
		return nil
	}
	if err != nil {
		return xerrors.Errorf("failed to load chain state from datastore: %w", err)
	}

	var tscids []cid.Cid
	if err := json.Unmarshal(head, &tscids); err != nil {
		return xerrors.Errorf("failed to unmarshal stored chain head: %w", err)
	}

	ts, err := cs.LoadTipSet(types.NewTipSetKey(tscids...))
	if err != nil {
		return xerrors.Errorf("loading tipset: %w", err)
	}

	cs.heaviest = ts

	return nil
}

func (cs *ChainStore) loadCheckpoint() error {
	tskBytes, err := cs.metadataDs.Get(checkpointKey)
	if err == dstore.ErrNotFound {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("failed to load checkpoint from datastore: %w", err)
	}

	var tsk types.TipSetKey
	err = json.Unmarshal(tskBytes, &tsk)
	if err != nil {
		return err
	}

	ts, err := cs.LoadTipSet(tsk)
	if err != nil {
		return xerrors.Errorf("loading tipset: %w", err)
	}

	cs.checkpoint = ts

	return nil
}

func (cs *ChainStore) writeHead(ts *types.TipSet) error {
	data, err := json.Marshal(ts.Cids())
	if err != nil {
		return xerrors.Errorf("failed to marshal tipset: %w", err)
	}

	if err := cs.metadataDs.Put(chainHeadKey, data); err != nil {
		return xerrors.Errorf("failed to write chain head to datastore: %w", err)
	}

	return nil
}

const (
	HCRevert  = "revert"
	HCApply   = "apply"
	HCCurrent = "current"
)

func (cs *ChainStore) SubHeadChanges(ctx context.Context) chan []*api.HeadChange {
	cs.pubLk.Lock()
	subch := cs.bestTips.Sub("headchange")
	head := cs.GetHeaviestTipSet()
	cs.pubLk.Unlock()

	out := make(chan []*api.HeadChange, 16)
	out <- []*api.HeadChange{{
		Type: HCCurrent,
		Val:  head,
	}}

	go func() {
		defer func() {
			// Tell the caller we're done first, the following may block for a bit.
			close(out)

			// Unsubscribe.
			cs.bestTips.Unsub(subch)

			// Drain the channel.
			for range subch {
			}
		}()

		for {
			select {
			case val, ok := <-subch:
				if !ok {
					// Shutting down.
					return
				}
				select {
				case out <- val.([]*api.HeadChange):
				default:
					log.Errorf("closing head change subscription due to slow reader")
					return
				}
				if len(out) > 5 {
					log.Warnf("head change sub is slow, has %d buffered entries", len(out))
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func (cs *ChainStore) SubscribeHeadChanges(f ReorgNotifee) {
	cs.reorgNotifeeCh <- f
}

func (cs *ChainStore) IsBlockValidated(ctx context.Context, blkid cid.Cid) (bool, error) {
	key := blockValidationCacheKeyPrefix.Instance(blkid.String())

	return cs.metadataDs.Has(key)
}

func (cs *ChainStore) MarkBlockAsValidated(ctx context.Context, blkid cid.Cid) error {
	key := blockValidationCacheKeyPrefix.Instance(blkid.String())

	if err := cs.metadataDs.Put(key, []byte{0}); err != nil {
		return xerrors.Errorf("cache block validation: %w", err)
	}

	return nil
}

func (cs *ChainStore) UnmarkBlockAsValidated(ctx context.Context, blkid cid.Cid) error {
	key := blockValidationCacheKeyPrefix.Instance(blkid.String())

	if err := cs.metadataDs.Delete(key); err != nil {
		return xerrors.Errorf("removing from valid block cache: %w", err)
	}

	return nil
}

func (cs *ChainStore) SetGenesis(b *types.BlockHeader) error {
	ts, err := types.NewTipSet([]*types.BlockHeader{b})
	if err != nil {
		return err
	}

	if err := cs.PutTipSet(context.TODO(), ts); err != nil {
		return err
	}

	return cs.metadataDs.Put(dstore.NewKey("0"), b.Cid().Bytes())
}

func (cs *ChainStore) PutTipSet(ctx context.Context, ts *types.TipSet) error {
	for _, b := range ts.Blocks() {
		if err := cs.PersistBlockHeaders(b); err != nil {
			return err
		}
	}

	expanded, err := cs.expandTipset(ts.Blocks()[0])
	if err != nil {
		return xerrors.Errorf("errored while expanding tipset: %w", err)
	}
	log.Debugf("expanded %s into %s\n", ts.Cids(), expanded.Cids())

	if err := cs.MaybeTakeHeavierTipSet(ctx, expanded); err != nil {
		return xerrors.Errorf("MaybeTakeHeavierTipSet failed in PutTipSet: %w", err)
	}
	return nil
}

// MaybeTakeHeavierTipSet evaluates the incoming tipset and locks it in our
// internal state as our new head, if and only if it is heavier than the current
// head and does not exceed the maximum fork length.
func (cs *ChainStore) MaybeTakeHeavierTipSet(ctx context.Context, ts *types.TipSet) error {
	for {
		cs.heaviestLk.Lock()
		if len(cs.reorgCh) < reorgChBuf/2 {
			break
		}
		cs.heaviestLk.Unlock()
		log.Errorf("reorg channel is heavily backlogged, waiting a bit before trying to take process new tipsets")
		select {
		case <-time.After(time.Second / 2):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	defer cs.heaviestLk.Unlock()
	w, err := cs.weight(ctx, cs.StateBlockstore(), ts)
	if err != nil {
		return err
	}
	heaviestW, err := cs.weight(ctx, cs.StateBlockstore(), cs.heaviest)
	if err != nil {
		return err
	}

	heavier := w.GreaterThan(heaviestW)
	if w.Equals(heaviestW) && !ts.Equals(cs.heaviest) {
		log.Errorw("weight draw", "currTs", cs.heaviest, "ts", ts)
		heavier = breakWeightTie(ts, cs.heaviest)
	}

	if heavier {
		// TODO: don't do this for initial sync. Now that we don't have a
		// difference between 'bootstrap sync' and 'caught up' sync, we need
		// some other heuristic.

		exceeds, err := cs.exceedsForkLength(cs.heaviest, ts)
		if err != nil {
			return err
		}
		if exceeds {
			return nil
		}

		return cs.takeHeaviestTipSet(ctx, ts)
	}

	return nil
}

// Check if the two tipsets have a fork length above `ForkLengthThreshold`.
// `synced` is the head of the chain we are currently synced to and `external`
// is the incoming tipset potentially belonging to a forked chain. It assumes
// the external chain has already been validated and available in the ChainStore.
// The "fast forward" case is covered in this logic as a valid fork of length 0.
//
// FIXME: We may want to replace some of the logic in `syncFork()` with this.
//  `syncFork()` counts the length on both sides of the fork at the moment (we
//  need to settle on that) but here we just enforce it on the `synced` side.
func (cs *ChainStore) exceedsForkLength(synced, external *types.TipSet) (bool, error) {
	if synced == nil || external == nil {
		// FIXME: If `cs.heaviest` is nil we should just bypass the entire
		//  `MaybeTakeHeavierTipSet` logic (instead of each of the called
		//  functions having to handle the nil case on their own).
		return false, nil
	}

	var err error
	// `forkLength`: number of tipsets we need to walk back from the our `synced`
	// chain to the common ancestor with the new `external` head in order to
	// adopt the fork.
	for forkLength := 0; forkLength < int(build.ForkLengthThreshold); forkLength++ {
		// First walk back as many tipsets in the external chain to match the
		// `synced` height to compare them. If we go past the `synced` height
		// the subsequent match will fail but it will still be useful to get
		// closer to the `synced` head parent's height in the next loop.
		for external.Height() > synced.Height() {
			if external.Height() == 0 {
				// We reached the genesis of the external chain without a match;
				// this is considered a fork outside the allowed limit (of "infinite"
				// length).
				return true, nil
			}
			external, err = cs.LoadTipSet(external.Parents())
			if err != nil {
				return false, xerrors.Errorf("failed to load parent tipset in external chain: %w", err)
			}
		}

		// Now check if we arrived at the common ancestor.
		if synced.Equals(external) {
			return false, nil
		}

		// Now check to see if we've walked back to the checkpoint.
		if synced.Equals(cs.checkpoint) {
			return true, nil
		}

		// If we didn't, go back *one* tipset on the `synced` side (incrementing
		// the `forkLength`).
		if synced.Height() == 0 {
			// Same check as the `external` side, if we reach the start (genesis)
			// there is no common ancestor.
			return true, nil
		}
		synced, err = cs.LoadTipSet(synced.Parents())
		if err != nil {
			return false, xerrors.Errorf("failed to load parent tipset in synced chain: %w", err)
		}
	}

	// We traversed the fork length allowed without finding a common ancestor.
	return true, nil
}

// ForceHeadSilent forces a chain head tipset without triggering a reorg
// operation.
//
// CAUTION: Use it only for testing, such as to teleport the chain to a
// particular tipset to carry out a benchmark, verification, etc. on a chain
// segment.
func (cs *ChainStore) ForceHeadSilent(_ context.Context, ts *types.TipSet) error {
	log.Warnf("(!!!) forcing a new head silently; new head: %s", ts)

	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	if err := cs.removeCheckpoint(); err != nil {
		return err
	}
	cs.heaviest = ts

	err := cs.writeHead(ts)
	if err != nil {
		err = xerrors.Errorf("failed to write chain head: %s", err)
	}
	return err
}

type reorg struct {
	old *types.TipSet
	new *types.TipSet
}

const reorgChBuf = 32

func (cs *ChainStore) reorgWorker(ctx context.Context, initialNotifees []ReorgNotifee) chan<- reorg {
	out := make(chan reorg, reorgChBuf)
	notifees := make([]ReorgNotifee, len(initialNotifees))
	copy(notifees, initialNotifees)

	cs.wg.Add(1)
	go func() {
		defer cs.wg.Done()
		defer log.Warn("reorgWorker quit")

		for {
			select {
			case n := <-cs.reorgNotifeeCh:
				notifees = append(notifees, n)

			case r := <-out:
				revert, apply, err := cs.ReorgOps(r.old, r.new)
				if err != nil {
					log.Error("computing reorg ops failed: ", err)
					continue
				}

				cs.journal.RecordEvent(cs.evtTypes[evtTypeHeadChange], func() interface{} {
					return HeadChangeEvt{
						From:        r.old.Key(),
						FromHeight:  r.old.Height(),
						To:          r.new.Key(),
						ToHeight:    r.new.Height(),
						RevertCount: len(revert),
						ApplyCount:  len(apply),
					}
				})

				// reverse the apply array
				for i := len(apply)/2 - 1; i >= 0; i-- {
					opp := len(apply) - 1 - i
					apply[i], apply[opp] = apply[opp], apply[i]
				}

				var toremove map[int]struct{}
				for i, hcf := range notifees {
					err := hcf(revert, apply)

					switch err {
					case nil:

					case ErrNotifeeDone:
						if toremove == nil {
							toremove = make(map[int]struct{})
						}
						toremove[i] = struct{}{}

					default:
						log.Error("head change func errored (BAD): ", err)
					}
				}

				if len(toremove) > 0 {
					newNotifees := make([]ReorgNotifee, 0, len(notifees)-len(toremove))
					for i, hcf := range notifees {
						_, remove := toremove[i]
						if remove {
							continue
						}
						newNotifees = append(newNotifees, hcf)
					}
					notifees = newNotifees
				}

			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// takeHeaviestTipSet actually sets the incoming tipset as our head both in
// memory and in the ChainStore. It also sends a notification to deliver to
// ReorgNotifees.
func (cs *ChainStore) takeHeaviestTipSet(ctx context.Context, ts *types.TipSet) error {
	_, span := trace.StartSpan(ctx, "takeHeaviestTipSet")
	defer span.End()

	if cs.heaviest != nil { // buf
		if len(cs.reorgCh) > 0 {
			log.Warnf("Reorg channel running behind, %d reorgs buffered", len(cs.reorgCh))
		}
		cs.reorgCh <- reorg{
			old: cs.heaviest,
			new: ts,
		}
	} else {
		log.Warnf("no heaviest tipset found, using %s", ts.Cids())
	}

	span.AddAttributes(trace.BoolAttribute("newHead", true))

	log.Infof("New heaviest tipset! %s (height=%d)", ts.Cids(), ts.Height())
	cs.heaviest = ts

	if err := cs.writeHead(ts); err != nil {
		log.Errorf("failed to write chain head: %s", err)
		return nil
	}

	return nil
}

// FlushValidationCache removes all results of block validation from the
// chain metadata store. Usually the first step after a new chain import.
func (cs *ChainStore) FlushValidationCache() error {
	return FlushValidationCache(cs.metadataDs)
}

func FlushValidationCache(ds dstore.Batching) error {
	log.Infof("clearing block validation cache...")

	dsWalk, err := ds.Query(query.Query{
		// Potential TODO: the validation cache is not a namespace on its own
		// but is rather constructed as prefixed-key `foo:bar` via .Instance(), which
		// in turn does not work with the filter, which can match only on `foo/bar`
		//
		// If this is addressed (blockcache goes into its own sub-namespace) then
		// strings.HasPrefix(...) below can be skipped
		//
		//Prefix: blockValidationCacheKeyPrefix.String()
		KeysOnly: true,
	})
	if err != nil {
		return xerrors.Errorf("failed to initialize key listing query: %w", err)
	}

	allKeys, err := dsWalk.Rest()
	if err != nil {
		return xerrors.Errorf("failed to run key listing query: %w", err)
	}

	batch, err := ds.Batch()
	if err != nil {
		return xerrors.Errorf("failed to open a DS batch: %w", err)
	}

	delCnt := 0
	for _, k := range allKeys {
		if strings.HasPrefix(k.Key, blockValidationCacheKeyPrefix.String()) {
			delCnt++
			batch.Delete(dstore.RawKey(k.Key)) // nolint:errcheck
		}
	}

	if err := batch.Commit(); err != nil {
		return xerrors.Errorf("failed to commit the DS batch: %w", err)
	}

	log.Infof("%d block validation entries cleared.", delCnt)

	return nil
}

// SetHead sets the chainstores current 'best' head node.
// This should only be called if something is broken and needs fixing.
//
// This function will bypass and remove any checkpoints.
func (cs *ChainStore) SetHead(ts *types.TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	if err := cs.removeCheckpoint(); err != nil {
		return err
	}
	return cs.takeHeaviestTipSet(context.TODO(), ts)
}

// RemoveCheckpoint removes the current checkpoint.
func (cs *ChainStore) RemoveCheckpoint() error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.removeCheckpoint()
}

func (cs *ChainStore) removeCheckpoint() error {
	if err := cs.metadataDs.Delete(checkpointKey); err != nil {
		return err
	}
	cs.checkpoint = nil
	return nil
}

// SetCheckpoint will set a checkpoint past which the chainstore will not allow forks.
//
// NOTE: Checkpoints cannot be set beyond ForkLengthThreshold epochs in the past.
func (cs *ChainStore) SetCheckpoint(ts *types.TipSet) error {
	tskBytes, err := json.Marshal(ts.Key())
	if err != nil {
		return err
	}

	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()

	if ts.Height() > cs.heaviest.Height() {
		return xerrors.Errorf("cannot set a checkpoint in the future")
	}

	// Otherwise, this operation could get _very_ expensive.
	if cs.heaviest.Height()-ts.Height() > build.ForkLengthThreshold {
		return xerrors.Errorf("cannot set a checkpoint before the fork threshold")
	}

	if !ts.Equals(cs.heaviest) {
		anc, err := cs.IsAncestorOf(ts, cs.heaviest)
		if err != nil {
			return xerrors.Errorf("cannot determine whether checkpoint tipset is in main-chain: %w", err)
		}

		if !anc {
			return xerrors.Errorf("cannot mark tipset as checkpoint, since it isn't in the main-chain: %w", err)
		}
	}
	err = cs.metadataDs.Put(checkpointKey, tskBytes)
	if err != nil {
		return err
	}

	cs.checkpoint = ts
	return nil
}

func (cs *ChainStore) GetCheckpoint() *types.TipSet {
	cs.heaviestLk.RLock()
	chkpt := cs.checkpoint
	cs.heaviestLk.RUnlock()
	return chkpt
}

// Contains returns whether our BlockStore has all blocks in the supplied TipSet.
func (cs *ChainStore) Contains(ts *types.TipSet) (bool, error) {
	for _, c := range ts.Cids() {
		has, err := cs.chainBlockstore.Has(c)
		if err != nil {
			return false, err
		}

		if !has {
			return false, nil
		}
	}
	return true, nil
}

// GetBlock fetches a BlockHeader with the supplied CID. It returns
// blockstore.ErrNotFound if the block was not found in the BlockStore.
func (cs *ChainStore) GetBlock(c cid.Cid) (*types.BlockHeader, error) {
	var blk *types.BlockHeader
	err := cs.chainLocalBlockstore.View(c, func(b []byte) (err error) {
		blk, err = types.DecodeBlock(b)
		return err
	})
	return blk, err
}

func (cs *ChainStore) LoadTipSet(tsk types.TipSetKey) (*types.TipSet, error) {
	v, ok := cs.tsCache.Get(tsk)
	if ok {
		return v.(*types.TipSet), nil
	}

	// Fetch tipset block headers from blockstore in parallel
	var eg errgroup.Group
	cids := tsk.Cids()
	blks := make([]*types.BlockHeader, len(cids))
	for i, c := range cids {
		i, c := i, c
		eg.Go(func() error {
			b, err := cs.GetBlock(c)
			if err != nil {
				return xerrors.Errorf("get block %s: %w", c, err)
			}

			blks[i] = b
			return nil
		})
	}
	err := eg.Wait()
	if err != nil {
		return nil, err
	}

	ts, err := types.NewTipSet(blks)
	if err != nil {
		return nil, err
	}

	cs.tsCache.Add(tsk, ts)

	return ts, nil
}

// IsAncestorOf returns true if 'a' is an ancestor of 'b'
func (cs *ChainStore) IsAncestorOf(a, b *types.TipSet) (bool, error) {
	if b.Height() <= a.Height() {
		return false, nil
	}

	cur := b
	for !a.Equals(cur) && cur.Height() > a.Height() {
		next, err := cs.LoadTipSet(cur.Parents())
		if err != nil {
			return false, err
		}

		cur = next
	}

	return cur.Equals(a), nil
}

func (cs *ChainStore) NearestCommonAncestor(a, b *types.TipSet) (*types.TipSet, error) {
	l, _, err := cs.ReorgOps(a, b)
	if err != nil {
		return nil, err
	}

	return cs.LoadTipSet(l[len(l)-1].Parents())
}

// ReorgOps takes two tipsets (which can be at different heights), and walks
// their corresponding chains backwards one step at a time until we find
// a common ancestor. It then returns the respective chain segments that fork
// from the identified ancestor, in reverse order, where the first element of
// each slice is the supplied tipset, and the last element is the common
// ancestor.
//
// If an error happens along the way, we return the error with nil slices.
func (cs *ChainStore) ReorgOps(a, b *types.TipSet) ([]*types.TipSet, []*types.TipSet, error) {
	return ReorgOps(cs.LoadTipSet, a, b)
}

func ReorgOps(lts func(types.TipSetKey) (*types.TipSet, error), a, b *types.TipSet) ([]*types.TipSet, []*types.TipSet, error) {
	left := a
	right := b

	var leftChain, rightChain []*types.TipSet
	for !left.Equals(right) {
		if left.Height() > right.Height() {
			leftChain = append(leftChain, left)
			par, err := lts(left.Parents())
			if err != nil {
				return nil, nil, err
			}

			left = par
		} else {
			rightChain = append(rightChain, right)
			par, err := lts(right.Parents())
			if err != nil {
				log.Infof("failed to fetch right.Parents: %s", err)
				return nil, nil, err
			}

			right = par
		}
	}

	return leftChain, rightChain, nil

}

// GetHeaviestTipSet returns the current heaviest tipset known (i.e. our head).
func (cs *ChainStore) GetHeaviestTipSet() (ts *types.TipSet) {
	cs.heaviestLk.RLock()
	ts = cs.heaviest
	cs.heaviestLk.RUnlock()
	return
}

func (cs *ChainStore) AddToTipSetTracker(b *types.BlockHeader) error {
	cs.tstLk.Lock()
	defer cs.tstLk.Unlock()

	tss := cs.tipsets[b.Height]
	for _, oc := range tss {
		if oc == b.Cid() {
			log.Debug("tried to add block to tipset tracker that was already there")
			return nil
		}
		h, err := cs.GetBlock(oc)
		if err == nil && h != nil {
			if h.Miner == b.Miner {
				log.Warnf("Have multiple blocks from miner %s at height %d in our tipset cache %s-%s", b.Miner, b.Height, b.Cid(), h.Cid())
			}
		}
	}
	// This function is called 5 times per epoch on average
	// It is also called with tipsets that are done with initial validation
	// so they cannot be from the future.
	// We are guaranteed not to use tipsets older than 900 epochs (fork limit)
	// This means that we ideally want to keep only most recent 900 epochs in here
	// Golang's map iteration starts at a random point in a map.
	// With 5 tries per epoch, and 900 entries to keep, on average we will have
	// ~136 garbage entires in the `cs.tipsets` map. (solve for 1-(1-x/(900+x))^5 == 0.5)
	// Seems good enough to me

	for height := range cs.tipsets {
		if height < b.Height-build.Finality {
			delete(cs.tipsets, height)
		}
		break
	}

	cs.tipsets[b.Height] = append(tss, b.Cid())

	return nil
}

func (cs *ChainStore) PersistBlockHeaders(b ...*types.BlockHeader) error {
	sbs := make([]block.Block, len(b))

	for i, header := range b {
		var err error
		sbs[i], err = header.ToStorageBlock()
		if err != nil {
			return err
		}
	}

	batchSize := 256
	calls := len(b) / batchSize

	var err error
	for i := 0; i <= calls; i++ {
		start := batchSize * i
		end := start + batchSize
		if end > len(b) {
			end = len(b)
		}

		err = multierr.Append(err, cs.chainLocalBlockstore.PutMany(sbs[start:end]))
	}

	return err
}

func (cs *ChainStore) expandTipset(b *types.BlockHeader) (*types.TipSet, error) {
	// Hold lock for the whole function for now, if it becomes a problem we can
	// fix pretty easily
	cs.tstLk.Lock()
	defer cs.tstLk.Unlock()

	all := []*types.BlockHeader{b}

	tsets, ok := cs.tipsets[b.Height]
	if !ok {
		return types.NewTipSet(all)
	}

	inclMiners := map[address.Address]cid.Cid{b.Miner: b.Cid()}
	for _, bhc := range tsets {
		if bhc == b.Cid() {
			continue
		}

		h, err := cs.GetBlock(bhc)
		if err != nil {
			return nil, xerrors.Errorf("failed to load block (%s) for tipset expansion: %w", bhc, err)
		}

		if cid, found := inclMiners[h.Miner]; found {
			log.Warnf("Have multiple blocks from miner %s at height %d in our tipset cache %s-%s", h.Miner, h.Height, h.Cid(), cid)
			continue
		}

		if types.CidArrsEqual(h.Parents, b.Parents) {
			all = append(all, h)
			inclMiners[h.Miner] = bhc
		}
	}

	// TODO: other validation...?

	return types.NewTipSet(all)
}

func (cs *ChainStore) AddBlock(ctx context.Context, b *types.BlockHeader) error {
	if err := cs.PersistBlockHeaders(b); err != nil {
		return err
	}

	ts, err := cs.expandTipset(b)
	if err != nil {
		return err
	}

	if err := cs.MaybeTakeHeavierTipSet(ctx, ts); err != nil {
		return xerrors.Errorf("MaybeTakeHeavierTipSet failed: %w", err)
	}

	return nil
}

func (cs *ChainStore) GetGenesis() (*types.BlockHeader, error) {
	data, err := cs.metadataDs.Get(dstore.NewKey("0"))
	if err != nil {
		return nil, err
	}

	c, err := cid.Cast(data)
	if err != nil {
		return nil, err
	}

	return cs.GetBlock(c)
}

// GetPath returns the sequence of atomic head change operations that
// need to be applied in order to switch the head of the chain from the `from`
// tipset to the `to` tipset.
func (cs *ChainStore) GetPath(ctx context.Context, from types.TipSetKey, to types.TipSetKey) ([]*api.HeadChange, error) {
	fts, err := cs.LoadTipSet(from)
	if err != nil {
		return nil, xerrors.Errorf("loading from tipset %s: %w", from, err)
	}
	tts, err := cs.LoadTipSet(to)
	if err != nil {
		return nil, xerrors.Errorf("loading to tipset %s: %w", to, err)
	}
	revert, apply, err := cs.ReorgOps(fts, tts)
	if err != nil {
		return nil, xerrors.Errorf("error getting tipset branches: %w", err)
	}

	path := make([]*api.HeadChange, len(revert)+len(apply))
	for i, r := range revert {
		path[i] = &api.HeadChange{Type: HCRevert, Val: r}
	}
	for j, i := 0, len(apply)-1; i >= 0; j, i = j+1, i-1 {
		path[j+len(revert)] = &api.HeadChange{Type: HCApply, Val: apply[i]}
	}
	return path, nil
}

// ChainBlockstore returns the chain blockstore. Currently the chain and state
// // stores are both backed by the same physical store, albeit with different
// // caching policies, but in the future they will segregate.
func (cs *ChainStore) ChainBlockstore() bstore.Blockstore {
	return cs.chainBlockstore
}

// StateBlockstore returns the state blockstore. Currently the chain and state
// stores are both backed by the same physical store, albeit with different
// caching policies, but in the future they will segregate.
func (cs *ChainStore) StateBlockstore() bstore.Blockstore {
	return cs.stateBlockstore
}

func ActorStore(ctx context.Context, bs bstore.Blockstore) adt.Store {
	return adt.WrapStore(ctx, cbor.NewCborStore(bs))
}

func (cs *ChainStore) ActorStore(ctx context.Context) adt.Store {
	return ActorStore(ctx, cs.stateBlockstore)
}

func (cs *ChainStore) TryFillTipSet(ts *types.TipSet) (*FullTipSet, error) {
	var out []*types.FullBlock

	for _, b := range ts.Blocks() {
		bmsgs, smsgs, err := cs.MessagesForBlock(b)
		if err != nil {
			// TODO: check for 'not found' errors, and only return nil if this
			// is actually a 'not found' error
			return nil, nil
		}

		fb := &types.FullBlock{
			Header:        b,
			BlsMessages:   bmsgs,
			SecpkMessages: smsgs,
		}

		out = append(out, fb)
	}
	return NewFullTipSet(out), nil
}

// GetTipsetByHeight returns the tipset on the chain behind 'ts' at the given
// height. In the case that the given height is a null round, the 'prev' flag
// selects the tipset before the null round if true, and the tipset following
// the null round if false.
func (cs *ChainStore) GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, ts *types.TipSet, prev bool) (*types.TipSet, error) {
	if ts == nil {
		ts = cs.GetHeaviestTipSet()
	}

	if h > ts.Height() {
		return nil, xerrors.Errorf("looking for tipset with height greater than start point")
	}

	if h == ts.Height() {
		return ts, nil
	}

	lbts, err := cs.cindex.GetTipsetByHeight(ctx, ts, h)
	if err != nil {
		return nil, err
	}

	if lbts.Height() < h {
		log.Warnf("chain index returned the wrong tipset at height %d, using slow retrieval", h)
		lbts, err = cs.cindex.GetTipsetByHeightWithoutCache(ts, h)
		if err != nil {
			return nil, err
		}
	}

	if lbts.Height() == h || !prev {
		return lbts, nil
	}

	return cs.LoadTipSet(lbts.Parents())
}

func (cs *ChainStore) Weight(ctx context.Context, hts *types.TipSet) (types.BigInt, error) { // todo remove
	return cs.weight(ctx, cs.StateBlockstore(), hts)
}

// true if ts1 wins according to the filecoin tie-break rule
func breakWeightTie(ts1, ts2 *types.TipSet) bool {
	s := len(ts1.Blocks())
	if s > len(ts2.Blocks()) {
		s = len(ts2.Blocks())
	}

	// blocks are already sorted by ticket
	for i := 0; i < s; i++ {
		if ts1.Blocks()[i].Ticket.Less(ts2.Blocks()[i].Ticket) {
			log.Infof("weight tie broken in favour of %s", ts1.Key())
			return true
		}
	}

	log.Infof("weight tie left unbroken, default to %s", ts2.Key())
	return false
}

func (cs *ChainStore) GetTipSetFromKey(tsk types.TipSetKey) (*types.TipSet, error) {
	if tsk.IsEmpty() {
		return cs.GetHeaviestTipSet(), nil
	}
	return cs.LoadTipSet(tsk)
}

func (cs *ChainStore) GetLatestBeaconEntry(ts *types.TipSet) (*types.BeaconEntry, error) {
	cur := ts
	for i := 0; i < 20; i++ {
		cbe := cur.Blocks()[0].BeaconEntries
		if len(cbe) > 0 {
			return &cbe[len(cbe)-1], nil
		}

		if cur.Height() == 0 {
			return nil, xerrors.Errorf("made it back to genesis block without finding beacon entry")
		}

		next, err := cs.LoadTipSet(cur.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to load parents when searching back for latest beacon entry: %w", err)
		}
		cur = next
	}

	if os.Getenv("LOTUS_IGNORE_DRAND") == "_yes_" {
		return &types.BeaconEntry{
			Data: []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
		}, nil
	}

	return nil, xerrors.Errorf("found NO beacon entries in the 20 latest tipsets")
}
