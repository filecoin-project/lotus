package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/arc/v2"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/pubsub"

	"github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/metrics"
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
//  1. a tipset cache
//  2. a block => messages references cache.
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

	mmCache *arc.ARCCache[cid.Cid, mmCids]
	tsCache *arc.ARCCache[types.TipSetKey, *types.TipSet]

	evtTypes [1]journal.EventType
	journal  journal.Journal

	storeEvents bool

	cancelFn context.CancelFunc
	wg       sync.WaitGroup
}

func NewChainStore(chainBs bstore.Blockstore, stateBs bstore.Blockstore, ds dstore.Batching, weight WeightFunc, j journal.Journal) *ChainStore {
	c, _ := arc.NewARC[cid.Cid, mmCids](DefaultMsgMetaCacheSize)
	tsc, _ := arc.NewARC[types.TipSetKey, *types.TipSet](DefaultTipSetCacheSize)
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

func (cs *ChainStore) Load(ctx context.Context) error {
	if err := cs.loadHead(ctx); err != nil {
		return err
	}
	if err := cs.loadCheckpoint(ctx); err != nil {
		return err
	}
	return nil
}
func (cs *ChainStore) loadHead(ctx context.Context) error {
	head, err := cs.metadataDs.Get(ctx, chainHeadKey)
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

	ts, err := cs.LoadTipSet(ctx, types.NewTipSetKey(tscids...))
	if err != nil {
		return xerrors.Errorf("loading tipset: %w", err)
	}

	cs.heaviest = ts

	return nil
}

func (cs *ChainStore) loadCheckpoint(ctx context.Context) error {
	tskBytes, err := cs.metadataDs.Get(ctx, checkpointKey)
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

	ts, err := cs.LoadTipSet(ctx, tsk)
	if err != nil {
		return xerrors.Errorf("loading tipset: %w", err)
	}

	cs.checkpoint = ts

	return nil
}

func (cs *ChainStore) writeHead(ctx context.Context, ts *types.TipSet) error {
	data, err := json.Marshal(ts.Cids())
	if err != nil {
		return xerrors.Errorf("failed to marshal tipset: %w", err)
	}

	if err := cs.metadataDs.Put(ctx, chainHeadKey, data); err != nil {
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

			// revive:disable-next-line:empty-block
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

	return cs.metadataDs.Has(ctx, key)
}

func (cs *ChainStore) MarkBlockAsValidated(ctx context.Context, blkid cid.Cid) error {
	key := blockValidationCacheKeyPrefix.Instance(blkid.String())

	if err := cs.metadataDs.Put(ctx, key, []byte{0}); err != nil {
		return xerrors.Errorf("cache block validation: %w", err)
	}

	return nil
}

func (cs *ChainStore) UnmarkBlockAsValidated(ctx context.Context, blkid cid.Cid) error {
	key := blockValidationCacheKeyPrefix.Instance(blkid.String())

	if err := cs.metadataDs.Delete(ctx, key); err != nil {
		return xerrors.Errorf("removing from valid block cache: %w", err)
	}

	return nil
}

func (cs *ChainStore) SetGenesis(ctx context.Context, b *types.BlockHeader) error {
	ts, err := types.NewTipSet([]*types.BlockHeader{b})
	if err != nil {
		return xerrors.Errorf("failed to construct genesis tipset: %w", err)
	}

	if err := cs.PersistTipsets(ctx, []*types.TipSet{ts}); err != nil {
		return xerrors.Errorf("failed to persist genesis tipset: %w", err)
	}

	if err := cs.AddToTipSetTracker(ctx, b); err != nil {
		return xerrors.Errorf("failed to add genesis tipset to tracker: %w", err)
	}

	if err := cs.RefreshHeaviestTipSet(ctx, ts.Height()); err != nil {
		return xerrors.Errorf("failed to put genesis tipset: %w", err)
	}

	return cs.metadataDs.Put(ctx, dstore.NewKey("0"), b.Cid().Bytes())
}

// RefreshHeaviestTipSet receives a newTsHeight at which a new tipset might exist. It then:
// - "refreshes" the heaviest tipset that can be formed at its current heaviest height
//   - if equivocation is detected among the miners of the current heaviest tipset, the head is immediately updated to the heaviest tipset that can be formed in a range of 5 epochs
//
// - forms the best tipset that can be formed at the _input_ height
// - compares the three tipset weights: "current" heaviest tipset, "refreshed" tipset, and best tipset at newTsHeight
// - updates "current" heaviest to the heaviest of those 3 tipsets (if an update is needed), assuming it doesn't violate the maximum fork rule
func (cs *ChainStore) RefreshHeaviestTipSet(ctx context.Context, newTsHeight abi.ChainEpoch) error {
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

	heaviestWeight, err := cs.weight(ctx, cs.StateBlockstore(), cs.heaviest)
	if err != nil {
		return xerrors.Errorf("failed to calculate currentHeaviest's weight: %w", err)
	}

	heaviestHeight := abi.ChainEpoch(0)
	if cs.heaviest != nil {
		heaviestHeight = cs.heaviest.Height()
	}

	// Before we look at newTs, let's refresh best tipset at current head's height -- this is done to detect equivocation
	newHeaviest, newHeaviestWeight, err := cs.FormHeaviestTipSetForHeight(ctx, heaviestHeight)
	if err != nil {
		return xerrors.Errorf("failed to reform head at same height: %w", err)
	}

	// Equivocation has occurred! We need a new head NOW!
	if newHeaviest == nil || newHeaviestWeight.LessThan(heaviestWeight) {
		log.Warnf("chainstore heaviest tipset's weight SHRANK from %d (%s) to %d (%s) due to equivocation", heaviestWeight, cs.heaviest, newHeaviestWeight, newHeaviest)
		// Unfortunately, we don't know what the right height to form a new heaviest tipset is.
		// It is _probably_, but not _necessarily_, heaviestHeight.
		// So, we need to explore a range of epochs, finding the heaviest tipset in that range.
		// We thus try to form the heaviest tipset for 5 epochs above heaviestHeight (most of which will likely not exist),
		// as well as for 5 below.
		// This is slow, but we expect to almost-never be here (only if miners are equivocating, which carries a hefty penalty).
		for i := heaviestHeight + 5; i > heaviestHeight-5; i-- {
			possibleHeaviestTs, possibleHeaviestWeight, err := cs.FormHeaviestTipSetForHeight(ctx, i)
			if err != nil {
				return xerrors.Errorf("failed to produce head at height %d: %w", i, err)
			}

			if possibleHeaviestWeight.GreaterThan(newHeaviestWeight) {
				newHeaviestWeight = possibleHeaviestWeight
				newHeaviest = possibleHeaviestTs
			}
		}

		// if we've found something, we know it's the heaviest equivocation-free head, take it IMMEDIATELY
		if newHeaviest != nil {
			errTake := cs.takeHeaviestTipSet(ctx, newHeaviest)
			if errTake != nil {
				return xerrors.Errorf("failed to take newHeaviest tipset as head: %w", err)
			}
		} else {
			// if we haven't found something, just stay with our equivocation-y head
			newHeaviest = cs.heaviest
		}
	}

	// if the new height we were notified about isn't what we just refreshed at, see if we have a heavier tipset there
	if newTsHeight != newHeaviest.Height() {
		bestTs, bestTsWeight, err := cs.FormHeaviestTipSetForHeight(ctx, newTsHeight)
		if err != nil {
			return xerrors.Errorf("failed to form new heaviest tipset at height %d: %w", newTsHeight, err)
		}

		heavier := bestTsWeight.GreaterThan(newHeaviestWeight)
		if bestTsWeight.Equals(newHeaviestWeight) {
			heavier = breakWeightTie(bestTs, newHeaviest)
		}

		if heavier {
			newHeaviest = bestTs
		}
	}

	// Everything's the same as before, exit early
	if newHeaviest.Equals(cs.heaviest) {
		return nil
	}

	// At this point, it MUST be true that newHeaviest is heavier than cs.heaviest -- update if fork allows
	exceeds, err := cs.exceedsForkLength(ctx, cs.heaviest, newHeaviest)
	if err != nil {
		return xerrors.Errorf("failed to check fork length: %w", err)
	}

	if exceeds {
		return nil
	}

	err = cs.takeHeaviestTipSet(ctx, newHeaviest)
	if err != nil {
		return xerrors.Errorf("failed to take heaviest tipset: %w", err)
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
//
//	`syncFork()` counts the length on both sides of the fork at the moment (we
//	need to settle on that) but here we just enforce it on the `synced` side.
func (cs *ChainStore) exceedsForkLength(ctx context.Context, synced, external *types.TipSet) (bool, error) {
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
	for forkLength := 0; forkLength < int(policy.ChainFinality); forkLength++ {
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
			external, err = cs.LoadTipSet(ctx, external.Parents())
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
		synced, err = cs.LoadTipSet(ctx, synced.Parents())
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
func (cs *ChainStore) ForceHeadSilent(ctx context.Context, ts *types.TipSet) error {
	log.Warnf("(!!!) forcing a new head silently; new head: %s", ts)

	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	if err := cs.removeCheckpoint(ctx); err != nil {
		return err
	}
	cs.heaviest = ts

	err := cs.writeHead(ctx, ts)
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
				revert, apply, err := cs.ReorgOps(ctx, r.old, r.new)
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
	span.AddAttributes(trace.BoolAttribute("newHead", true))

	log.Infof("New heaviest tipset! %s (height=%d)", ts.Cids(), ts.Height())
	prevHeaviest := cs.heaviest
	cs.heaviest = ts

	if err := cs.writeHead(ctx, ts); err != nil {
		log.Errorf("failed to write chain head: %s", err)
		return err
	}

	// write the tipsetkey block to the blockstore for EthAPI queries
	tsBlk, err := ts.Key().ToStorageBlock()
	if err != nil {
		return xerrors.Errorf("failed to get tipset key block: %w", err)
	}

	if err = cs.chainLocalBlockstore.Put(ctx, tsBlk); err != nil {
		return xerrors.Errorf("failed to put tipset key block: %w", err)
	}

	if prevHeaviest != nil { // buf
		if len(cs.reorgCh) > 0 {
			log.Warnf("Reorg channel running behind, %d reorgs buffered", len(cs.reorgCh))
		}
		cs.reorgCh <- reorg{
			old: prevHeaviest,
			new: ts,
		}
	} else {
		log.Warnf("no previous heaviest tipset found, using %s", ts.Cids())
	}

	return nil
}

// FlushValidationCache removes all results of block validation from the
// chain metadata store. Usually the first step after a new chain import.
func (cs *ChainStore) FlushValidationCache(ctx context.Context) error {
	return FlushValidationCache(ctx, cs.metadataDs)
}

func FlushValidationCache(ctx context.Context, ds dstore.Batching) error {
	log.Infof("clearing block validation cache...")

	dsWalk, err := ds.Query(ctx, query.Query{
		// Potential TODO: the validation cache is not a namespace on its own
		// but is rather constructed as prefixed-key `foo:bar` via .Instance(), which
		// in turn does not work with the filter, which can match only on `foo/bar`
		//
		// If this is addressed (blockcache goes into its own sub-namespace) then
		// strings.HasPrefix(...) below can be skipped
		//
		// Prefix: blockValidationCacheKeyPrefix.String()
		KeysOnly: true,
	})
	if err != nil {
		return xerrors.Errorf("failed to initialize key listing query: %w", err)
	}

	allKeys, err := dsWalk.Rest()
	if err != nil {
		return xerrors.Errorf("failed to run key listing query: %w", err)
	}

	batch, err := ds.Batch(ctx)
	if err != nil {
		return xerrors.Errorf("failed to open a DS batch: %w", err)
	}

	delCnt := 0
	for _, k := range allKeys {
		if strings.HasPrefix(k.Key, blockValidationCacheKeyPrefix.String()) {
			delCnt++
			_ = batch.Delete(ctx, dstore.RawKey(k.Key))
		}
	}

	if err := batch.Commit(ctx); err != nil {
		return xerrors.Errorf("failed to commit the DS batch: %w", err)
	}

	log.Infof("%d block validation entries cleared.", delCnt)

	return nil
}

// SetHead sets the chainstores current 'best' head node.
// This should only be called if something is broken and needs fixing.
//
// This function will bypass and remove any checkpoints.
func (cs *ChainStore) SetHead(ctx context.Context, ts *types.TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	if err := cs.removeCheckpoint(ctx); err != nil {
		return err
	}
	return cs.takeHeaviestTipSet(context.TODO(), ts)
}

// RemoveCheckpoint removes the current checkpoint.
func (cs *ChainStore) RemoveCheckpoint(ctx context.Context) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.removeCheckpoint(ctx)
}

func (cs *ChainStore) removeCheckpoint(ctx context.Context) error {
	if err := cs.metadataDs.Delete(ctx, checkpointKey); err != nil {
		return err
	}
	cs.checkpoint = nil
	return nil
}

// SetCheckpoint will set a checkpoint past which the chainstore will not allow forks. If the new
// checkpoint is not an ancestor of the current head, head will be set to the new checkpoint.
//
// NOTE: Checkpoints cannot revert more than policy.Finality epochs.
// NOTE: The new checkpoint must already be synced.
func (cs *ChainStore) SetCheckpoint(ctx context.Context, ts *types.TipSet) error {
	tskBytes, err := json.Marshal(ts.Key())
	if err != nil {
		return err
	}

	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()

	finality := cs.heaviest.Height() - policy.ChainFinality
	targetChain, currentChain := ts, cs.heaviest

	// First attempt to skip backwards to a common height using the chain index.
	if targetChain.Height() > currentChain.Height() {
		targetChain, err = cs.GetTipsetByHeight(ctx, currentChain.Height(), targetChain, true)
	} else if targetChain.Height() < currentChain.Height() {
		currentChain, err = cs.GetTipsetByHeight(ctx, targetChain.Height(), currentChain, true)
	}
	if err != nil {
		return xerrors.Errorf("checkpoint failed: error when finding the fork point: %w", err)
	}

	// Then walk backwards until either we find a common block (the fork height) or we reach
	// finality. If the tipsets are _equal_ on the first pass through this loop, it means one
	// chain is a prefix of the other chain because we've only walked back on one chain so far.
	// In that case, we _don't_ check finality because we're not forking.
	for !currentChain.Equals(targetChain) && currentChain.Height() > finality {
		if currentChain.Height() >= targetChain.Height() {
			currentChain, err = cs.GetTipSetFromKey(ctx, currentChain.Parents())
			if err != nil {
				return xerrors.Errorf("checkpoint failed: error when walking the current chain: %w", err)
			}
		}

		if targetChain.Height() > currentChain.Height() {
			targetChain, err = cs.GetTipSetFromKey(ctx, targetChain.Parents())
			if err != nil {
				return xerrors.Errorf("checkpoint failed: error when walking the target chain: %w", err)
			}
		}
	}

	// If we haven't found a common tipset by this point, we can't switch chains.
	if !currentChain.Equals(targetChain) {
		return xerrors.Errorf("checkpoint failed: failed to find the fork point from %s (head) to %s (target) within finality",
			cs.heaviest.Key(),
			ts.Key(),
		)
	}

	// If the target tipset isn't an ancestor of our current chain, we need to switch chains.
	if !currentChain.Equals(ts) {
		if err := cs.takeHeaviestTipSet(ctx, ts); err != nil {
			return xerrors.Errorf("failed to switch chains when setting checkpoint: %w", err)
		}
	}

	// Finally, set the checkpoint.
	err = cs.metadataDs.Put(ctx, checkpointKey, tskBytes)
	if err != nil {
		return xerrors.Errorf("checkpoint failed: failed to record checkpoint in the datastore: %w", err)
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
func (cs *ChainStore) Contains(ctx context.Context, ts *types.TipSet) (bool, error) {
	for _, c := range ts.Cids() {
		has, err := cs.chainBlockstore.Has(ctx, c)
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
func (cs *ChainStore) GetBlock(ctx context.Context, c cid.Cid) (*types.BlockHeader, error) {
	var blk *types.BlockHeader
	err := cs.chainLocalBlockstore.View(ctx, c, func(b []byte) (err error) {
		blk, err = types.DecodeBlock(b)
		return err
	})
	return blk, err
}

func (cs *ChainStore) LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	if ts, ok := cs.tsCache.Get(tsk); ok {
		return ts, nil
	}

	// Fetch tipset block headers from blockstore in parallel
	var eg errgroup.Group
	cids := tsk.Cids()
	blks := make([]*types.BlockHeader, len(cids))
	for i, c := range cids {
		eg.Go(func() error {
			b, err := cs.GetBlock(ctx, c)
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
func (cs *ChainStore) IsAncestorOf(ctx context.Context, a, b *types.TipSet) (bool, error) {
	if b.Height() <= a.Height() {
		return false, nil
	}

	target, err := cs.GetTipsetByHeight(ctx, a.Height(), b, false)
	if err != nil {
		return false, err
	}

	return target.Equals(a), nil
}

// ReorgOps takes two tipsets (which can be at different heights), and walks
// their corresponding chains backwards one step at a time until we find
// a common ancestor. It then returns the respective chain segments that fork
// from the identified ancestor, in reverse order, where the first element of
// each slice is the supplied tipset, and the last element is the common
// ancestor.
//
// If an error happens along the way, we return the error with nil slices.
func (cs *ChainStore) ReorgOps(ctx context.Context, a, b *types.TipSet) ([]*types.TipSet, []*types.TipSet, error) {
	return ReorgOps(ctx, cs.LoadTipSet, a, b)
}

func ReorgOps(ctx context.Context, lts func(ctx context.Context, _ types.TipSetKey) (*types.TipSet, error), a, b *types.TipSet) ([]*types.TipSet, []*types.TipSet, error) {
	left := a
	right := b

	var leftChain, rightChain []*types.TipSet
	for !left.Equals(right) {
		// this can take a long time and lot of memory if the tipsets are far apart
		// since it can be reached through remote calls, we need to
		// cancel early when possible to prevent resource exhaustion.
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		if left.Height() > right.Height() {
			leftChain = append(leftChain, left)
			par, err := lts(ctx, left.Parents())
			if err != nil {
				return nil, nil, err
			}

			left = par
		} else {
			rightChain = append(rightChain, right)
			par, err := lts(ctx, right.Parents())
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

func (cs *ChainStore) AddToTipSetTracker(ctx context.Context, b *types.BlockHeader) error {
	cs.tstLk.Lock()
	defer cs.tstLk.Unlock()

	tss := cs.tipsets[b.Height]
	for _, oc := range tss {
		if oc == b.Cid() {
			log.Debug("tried to add block to tipset tracker that was already there")
			return nil
		}
		h, err := cs.GetBlock(ctx, oc)
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
	// ~136 garbage entries in the `cs.tipsets` map. (solve for 1-(1-x/(900+x))^5 == 0.5)
	// Seems good enough to me

	for height := range cs.tipsets {
		if height < b.Height-policy.ChainFinality {
			delete(cs.tipsets, height)
		}
		break
	}

	cs.tipsets[b.Height] = append(tss, b.Cid())

	return nil
}

// PersistTipsets writes the provided blocks and the TipSetKey objects to the blockstore
func (cs *ChainStore) PersistTipsets(ctx context.Context, tipsets []*types.TipSet) error {
	toPersist := make([]*types.BlockHeader, 0, len(tipsets)*int(buildconstants.BlocksPerEpoch))
	tsBlks := make([]block.Block, 0, len(tipsets))
	for _, ts := range tipsets {
		toPersist = append(toPersist, ts.Blocks()...)
		tsBlk, err := ts.Key().ToStorageBlock()
		if err != nil {
			return xerrors.Errorf("failed to get tipset key block: %w", err)
		}

		tsBlks = append(tsBlks, tsBlk)
	}

	if err := cs.persistBlockHeaders(ctx, toPersist...); err != nil {
		return xerrors.Errorf("failed to persist block headers: %w", err)
	}

	if err := cs.chainLocalBlockstore.PutMany(ctx, tsBlks); err != nil {
		return xerrors.Errorf("failed to put tipset key blocks: %w", err)
	}

	return nil
}

func (cs *ChainStore) persistBlockHeaders(ctx context.Context, b ...*types.BlockHeader) error {
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

		err = multierr.Append(err, cs.chainLocalBlockstore.PutMany(ctx, sbs[start:end]))
	}

	return err
}

// FormHeaviestTipSetForHeight looks up all valid blocks at a given height, and returns the heaviest tipset that can be made at that height
// It does not consider ANY blocks from miners that have "equivocated" (produced 2 blocks at the same height)
func (cs *ChainStore) FormHeaviestTipSetForHeight(ctx context.Context, height abi.ChainEpoch) (*types.TipSet, types.BigInt, error) {
	cs.tstLk.Lock()
	defer cs.tstLk.Unlock()

	blockCids, ok := cs.tipsets[height]
	if !ok {
		return nil, types.NewInt(0), nil
	}

	// First, identify "bad" miners for the height

	seenMiners := map[address.Address]struct{}{}
	badMiners := map[address.Address]struct{}{}
	blocks := make([]*types.BlockHeader, 0, len(blockCids))
	for _, bhc := range blockCids {
		h, err := cs.GetBlock(ctx, bhc)
		if err != nil {
			return nil, types.NewInt(0), xerrors.Errorf("failed to load block (%s) for tipset expansion: %w", bhc, err)
		}

		if _, seen := seenMiners[h.Miner]; seen {
			badMiners[h.Miner] = struct{}{}
			continue
		}
		seenMiners[h.Miner] = struct{}{}
		blocks = append(blocks, h)
	}

	// Next, group by parent tipset

	formableTipsets := make(map[types.TipSetKey][]*types.BlockHeader, 0)
	for _, h := range blocks {
		if _, bad := badMiners[h.Miner]; bad {
			continue
		}
		ptsk := types.NewTipSetKey(h.Parents...)
		formableTipsets[ptsk] = append(formableTipsets[ptsk], h)
	}

	maxWeight := types.NewInt(0)
	var maxTs *types.TipSet
	for _, headers := range formableTipsets {
		ts, err := types.NewTipSet(headers)
		if err != nil {
			return nil, types.NewInt(0), xerrors.Errorf("unexpected error forming tipset: %w", err)
		}

		weight, err := cs.Weight(ctx, ts)
		if err != nil {
			return nil, types.NewInt(0), xerrors.Errorf("failed to calculate weight: %w", err)
		}

		heavier := weight.GreaterThan(maxWeight)
		if weight.Equals(maxWeight) {
			heavier = breakWeightTie(ts, maxTs)
		}

		if heavier {
			maxWeight = weight
			maxTs = ts
		}
	}

	return maxTs, maxWeight, nil
}

func (cs *ChainStore) GetGenesis(ctx context.Context) (*types.BlockHeader, error) {
	data, err := cs.metadataDs.Get(ctx, dstore.NewKey("0"))
	if err != nil {
		return nil, err
	}

	c, err := cid.Cast(data)
	if err != nil {
		return nil, err
	}

	return cs.GetBlock(ctx, c)
}

// GetPath returns the sequence of atomic head change operations that
// need to be applied in order to switch the head of the chain from the `from`
// tipset to the `to` tipset.
func (cs *ChainStore) GetPath(ctx context.Context, from types.TipSetKey, to types.TipSetKey) ([]*api.HeadChange, error) {
	fts, err := cs.LoadTipSet(ctx, from)
	if err != nil {
		return nil, xerrors.Errorf("loading from tipset %s: %w", from, err)
	}
	tts, err := cs.LoadTipSet(ctx, to)
	if err != nil {
		return nil, xerrors.Errorf("loading to tipset %s: %w", to, err)
	}
	revert, apply, err := cs.ReorgOps(ctx, fts, tts)
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
// stores are both backed by the same physical store, albeit with different
// caching policies, but in the future they will segregate.
func (cs *ChainStore) ChainBlockstore() bstore.Blockstore {
	return cs.chainBlockstore
}

// StateBlockstore returns the state blockstore. Currently the chain and state
// stores are both backed by the same physical store, albeit with different
// caching policies, but in the future they will segregate.
func (cs *ChainStore) StateBlockstore() bstore.Blockstore {
	return cs.stateBlockstore
}

func (cs *ChainStore) ChainLocalBlockstore() bstore.Blockstore {
	return cs.chainLocalBlockstore
}

func ActorStore(ctx context.Context, bs bstore.Blockstore) adt.Store {
	return adt.WrapStore(ctx, cbor.NewCborStore(bs))
}

func (cs *ChainStore) ActorStore(ctx context.Context) adt.Store {
	return ActorStore(ctx, cs.stateBlockstore)
}

func (cs *ChainStore) TryFillTipSet(ctx context.Context, ts *types.TipSet) (*FullTipSet, error) {
	var out []*types.FullBlock

	for _, b := range ts.Blocks() {
		bmsgs, smsgs, err := cs.MessagesForBlock(ctx, b)
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
	if h < 0 {
		return nil, xerrors.Errorf("height %d is negative", h)
	}

	if ts == nil {
		ts = cs.GetHeaviestTipSet()
	}

	if h > ts.Height() {
		return nil, xerrors.Errorf("looking for tipset with height greater than start point, req: %v, head: %d", h, ts.Height())
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
		lbts, err = cs.cindex.GetTipsetByHeightWithoutCache(ctx, ts, h)
		if err != nil {
			return nil, err
		}
	}

	if lbts.Height() == h || !prev {
		return lbts, nil
	}

	return cs.LoadTipSet(ctx, lbts.Parents())
}

func (cs *ChainStore) GetTipSetByCid(ctx context.Context, c cid.Cid) (*types.TipSet, error) {
	blk, err := cs.chainBlockstore.Get(ctx, c)
	if err != nil {
		return nil, xerrors.Errorf("cannot find tipset with cid %s: %w", c, err)
	}

	tsk := new(types.TipSetKey)
	if err := tsk.UnmarshalCBOR(bytes.NewReader(blk.RawData())); err != nil {
		return nil, xerrors.Errorf("cannot unmarshal block into tipset key: %w", err)
	}

	ts, err := cs.GetTipSetFromKey(ctx, *tsk)
	if err != nil {
		return nil, xerrors.Errorf("cannot get tipset from key: %w", err)
	}
	return ts, nil
}

func (cs *ChainStore) Weight(ctx context.Context, hts *types.TipSet) (types.BigInt, error) { // todo remove
	return cs.weight(ctx, cs.StateBlockstore(), hts)
}

// StoreEvents marks this ChainStore as storing events.
func (cs *ChainStore) StoreEvents(store bool) {
	cs.storeEvents = store
}

// IsStoringEvents indicates if this ChainStore is storing events.
func (cs *ChainStore) IsStoringEvents() bool {
	return cs.storeEvents
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
			log.Infof("weight tie broken in favour of %s against %s", ts1.Key(), ts2.Key())
			return true
		} else if ts2.Blocks()[i].Ticket.Less(ts1.Blocks()[i].Ticket) {
			log.Infof("weight tie broken in favour of %s against %s", ts2.Key(), ts1.Key())
			return false
		}
	}

	log.Warnf("weight tie between %s and %s left unbroken, default to %s", ts1.Key(), ts2.Key(), ts2.Key())
	return false
}

func (cs *ChainStore) GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	if tsk.IsEmpty() {
		return cs.GetHeaviestTipSet(), nil
	}
	return cs.LoadTipSet(ctx, tsk)
}

func (cs *ChainStore) GetLatestBeaconEntry(ctx context.Context, ts *types.TipSet) (*types.BeaconEntry, error) {
	cur := ts

	// Search for a beacon entry, in normal operation one should be in the requested tipset, but for
	// devnets where the blocktime is faster than the beacon period we may need to search back a bit
	// to find a tipset with a beacon entry.
	for i := 0; i < 20; i++ {
		cbe := cur.Blocks()[0].BeaconEntries
		if len(cbe) > 0 {
			return &cbe[len(cbe)-1], nil
		}

		if cur.Height() == 0 {
			return nil, xerrors.Errorf("made it back to genesis block without finding beacon entry")
		}

		next, err := cs.LoadTipSet(ctx, cur.Parents())
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
