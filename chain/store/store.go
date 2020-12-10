package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/minio/blake2b-simd"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/journal"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/metrics"

	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"go.uber.org/multierr"

	"github.com/filecoin-project/lotus/chain/types"

	lru "github.com/hashicorp/golang-lru"
	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dstore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-car"
	carutil "github.com/ipld/go-car/util"
	cbg "github.com/whyrusleeping/cbor-gen"
	"github.com/whyrusleeping/pubsub"
	"golang.org/x/xerrors"
)

var log = logging.Logger("chainstore")

var chainHeadKey = dstore.NewKey("head")
var blockValidationCacheKeyPrefix = dstore.NewKey("blockValidation")

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
type ReorgNotifee func(rev, app []*types.TipSet) error

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
	bs      bstore.Blockstore
	localbs bstore.Blockstore
	ds      dstore.Batching

	localviewer bstore.Viewer

	heaviestLk sync.Mutex
	heaviest   *types.TipSet

	bestTips *pubsub.PubSub
	pubLk    sync.Mutex

	tstLk   sync.Mutex
	tipsets map[abi.ChainEpoch][]cid.Cid

	cindex *ChainIndex

	reorgCh        chan<- reorg
	reorgNotifeeCh chan ReorgNotifee

	mmCache *lru.ARCCache
	tsCache *lru.ARCCache

	vmcalls vm.SyscallBuilder

	evtTypes [1]journal.EventType
	journal  journal.Journal

	cancelFn context.CancelFunc
	wg       sync.WaitGroup
}

// localbs is guaranteed to fail Get* if requested block isn't stored locally
func NewChainStore(bs bstore.Blockstore, localbs bstore.Blockstore, ds dstore.Batching, vmcalls vm.SyscallBuilder, j journal.Journal) *ChainStore {
	mmCache, _ := lru.NewARC(DefaultMsgMetaCacheSize)
	tsCache, _ := lru.NewARC(DefaultTipSetCacheSize)
	if j == nil {
		j = journal.NilJournal()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cs := &ChainStore{
		bs:       bs,
		localbs:  localbs,
		ds:       ds,
		bestTips: pubsub.New(64),
		tipsets:  make(map[abi.ChainEpoch][]cid.Cid),
		mmCache:  mmCache,
		tsCache:  tsCache,
		vmcalls:  vmcalls,
		cancelFn: cancel,
		journal:  j,
	}

	if v, ok := localbs.(bstore.Viewer); ok {
		cs.localviewer = v
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
	head, err := cs.ds.Get(chainHeadKey)
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

func (cs *ChainStore) writeHead(ts *types.TipSet) error {
	data, err := json.Marshal(ts.Cids())
	if err != nil {
		return xerrors.Errorf("failed to marshal tipset: %w", err)
	}

	if err := cs.ds.Put(chainHeadKey, data); err != nil {
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
		defer close(out)
		var unsubOnce sync.Once

		for {
			select {
			case val, ok := <-subch:
				if !ok {
					log.Warn("chain head sub exit loop")
					return
				}
				if len(out) > 5 {
					log.Warnf("head change sub is slow, has %d buffered entries", len(out))
				}
				select {
				case out <- val.([]*api.HeadChange):
				case <-ctx.Done():
				}
			case <-ctx.Done():
				unsubOnce.Do(func() {
					go cs.bestTips.Unsub(subch)
				})
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

	return cs.ds.Has(key)
}

func (cs *ChainStore) MarkBlockAsValidated(ctx context.Context, blkid cid.Cid) error {
	key := blockValidationCacheKeyPrefix.Instance(blkid.String())

	if err := cs.ds.Put(key, []byte{0}); err != nil {
		return xerrors.Errorf("cache block validation: %w", err)
	}

	return nil
}

func (cs *ChainStore) UnmarkBlockAsValidated(ctx context.Context, blkid cid.Cid) error {
	key := blockValidationCacheKeyPrefix.Instance(blkid.String())

	if err := cs.ds.Delete(key); err != nil {
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

	return cs.ds.Put(dstore.NewKey("0"), b.Cid().Bytes())
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
// head.
func (cs *ChainStore) MaybeTakeHeavierTipSet(ctx context.Context, ts *types.TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	w, err := cs.Weight(ctx, ts)
	if err != nil {
		return err
	}
	heaviestW, err := cs.Weight(ctx, cs.heaviest)
	if err != nil {
		return err
	}

	if w.GreaterThan(heaviestW) {
		// TODO: don't do this for initial sync. Now that we don't have a
		// difference between 'bootstrap sync' and 'caught up' sync, we need
		// some other heuristic.
		return cs.takeHeaviestTipSet(ctx, ts)
	} else if w.Equals(heaviestW) && !ts.Equals(cs.heaviest) {
		log.Errorw("weight draw", "currTs", cs.heaviest, "ts", ts)
	}
	return nil
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

func (cs *ChainStore) reorgWorker(ctx context.Context, initialNotifees []ReorgNotifee) chan<- reorg {
	out := make(chan reorg, 32)
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
	log.Infof("clearing block validation cache...")

	dsWalk, err := cs.ds.Query(query.Query{
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

	batch, err := cs.ds.Batch()
	if err != nil {
		return xerrors.Errorf("failed to open a DS batch: %w", err)
	}

	delCnt := 0
	for _, k := range allKeys {
		if strings.HasPrefix(k.Key, blockValidationCacheKeyPrefix.String()) {
			delCnt++
			batch.Delete(datastore.RawKey(k.Key)) // nolint:errcheck
		}
	}

	if err := batch.Commit(); err != nil {
		return xerrors.Errorf("failed to commit the DS batch: %w", err)
	}

	log.Infof("%d block validation entries cleared.", delCnt)

	return nil
}

// SetHead sets the chainstores current 'best' head node.
// This should only be called if something is broken and needs fixing
func (cs *ChainStore) SetHead(ts *types.TipSet) error {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.takeHeaviestTipSet(context.TODO(), ts)
}

// Contains returns whether our BlockStore has all blocks in the supplied TipSet.
func (cs *ChainStore) Contains(ts *types.TipSet) (bool, error) {
	for _, c := range ts.Cids() {
		has, err := cs.bs.Has(c)
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
	if cs.localviewer == nil {
		sb, err := cs.localbs.Get(c)
		if err != nil {
			return nil, err
		}
		return types.DecodeBlock(sb.RawData())
	}

	var blk *types.BlockHeader
	err := cs.localviewer.View(c, func(b []byte) (err error) {
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
func (cs *ChainStore) GetHeaviestTipSet() *types.TipSet {
	cs.heaviestLk.Lock()
	defer cs.heaviestLk.Unlock()
	return cs.heaviest
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

		err = multierr.Append(err, cs.bs.PutMany(sbs[start:end]))
	}

	return err
}

type storable interface {
	ToStorageBlock() (block.Block, error)
}

func PutMessage(bs bstore.Blockstore, m storable) (cid.Cid, error) {
	b, err := m.ToStorageBlock()
	if err != nil {
		return cid.Undef, err
	}

	if err := bs.Put(b); err != nil {
		return cid.Undef, err
	}

	return b.Cid(), nil
}

func (cs *ChainStore) PutMessage(m storable) (cid.Cid, error) {
	return PutMessage(cs.bs, m)
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
	data, err := cs.ds.Get(dstore.NewKey("0"))
	if err != nil {
		return nil, err
	}

	c, err := cid.Cast(data)
	if err != nil {
		return nil, err
	}

	return cs.GetBlock(c)
}

func (cs *ChainStore) GetCMessage(c cid.Cid) (types.ChainMsg, error) {
	m, err := cs.GetMessage(c)
	if err == nil {
		return m, nil
	}
	if err != bstore.ErrNotFound {
		log.Warnf("GetCMessage: unexpected error getting unsigned message: %s", err)
	}

	return cs.GetSignedMessage(c)
}

func (cs *ChainStore) GetMessage(c cid.Cid) (*types.Message, error) {
	if cs.localviewer == nil {
		sb, err := cs.localbs.Get(c)
		if err != nil {
			log.Errorf("get message get failed: %s: %s", c, err)
			return nil, err
		}
		return types.DecodeMessage(sb.RawData())
	}

	var msg *types.Message
	err := cs.localviewer.View(c, func(b []byte) (err error) {
		msg, err = types.DecodeMessage(b)
		return err
	})
	return msg, err
}

func (cs *ChainStore) GetSignedMessage(c cid.Cid) (*types.SignedMessage, error) {
	if cs.localviewer == nil {
		sb, err := cs.localbs.Get(c)
		if err != nil {
			log.Errorf("get message get failed: %s: %s", c, err)
			return nil, err
		}
		return types.DecodeSignedMessage(sb.RawData())
	}

	var msg *types.SignedMessage
	err := cs.localviewer.View(c, func(b []byte) (err error) {
		msg, err = types.DecodeSignedMessage(b)
		return err
	})
	return msg, err
}

func (cs *ChainStore) readAMTCids(root cid.Cid) ([]cid.Cid, error) {
	ctx := context.TODO()
	// block headers use adt0, for now.
	a, err := blockadt.AsArray(cs.Store(ctx), root)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	var (
		cids    []cid.Cid
		cborCid cbg.CborCid
	)
	if err := a.ForEach(&cborCid, func(i int64) error {
		c := cid.Cid(cborCid)
		cids = append(cids, c)
		return nil
	}); err != nil {
		return nil, xerrors.Errorf("failed to traverse amt: %w", err)
	}

	if uint64(len(cids)) != a.Length() {
		return nil, xerrors.Errorf("found %d cids, expected %d", len(cids), a.Length())
	}

	return cids, nil
}

type BlockMessages struct {
	Miner         address.Address
	BlsMessages   []types.ChainMsg
	SecpkMessages []types.ChainMsg
	WinCount      int64
}

func (cs *ChainStore) BlockMsgsForTipset(ts *types.TipSet) ([]BlockMessages, error) {
	applied := make(map[address.Address]uint64)

	selectMsg := func(m *types.Message) (bool, error) {
		// The first match for a sender is guaranteed to have correct nonce -- the block isn't valid otherwise
		if _, ok := applied[m.From]; !ok {
			applied[m.From] = m.Nonce
		}

		if applied[m.From] != m.Nonce {
			return false, nil
		}

		applied[m.From]++

		return true, nil
	}

	var out []BlockMessages
	for _, b := range ts.Blocks() {

		bms, sms, err := cs.MessagesForBlock(b)
		if err != nil {
			return nil, xerrors.Errorf("failed to get messages for block: %w", err)
		}

		bm := BlockMessages{
			Miner:         b.Miner,
			BlsMessages:   make([]types.ChainMsg, 0, len(bms)),
			SecpkMessages: make([]types.ChainMsg, 0, len(sms)),
			WinCount:      b.ElectionProof.WinCount,
		}

		for _, bmsg := range bms {
			b, err := selectMsg(bmsg.VMMessage())
			if err != nil {
				return nil, xerrors.Errorf("failed to decide whether to select message for block: %w", err)
			}

			if b {
				bm.BlsMessages = append(bm.BlsMessages, bmsg)
			}
		}

		for _, smsg := range sms {
			b, err := selectMsg(smsg.VMMessage())
			if err != nil {
				return nil, xerrors.Errorf("failed to decide whether to select message for block: %w", err)
			}

			if b {
				bm.SecpkMessages = append(bm.SecpkMessages, smsg)
			}
		}

		out = append(out, bm)
	}

	return out, nil
}

func (cs *ChainStore) MessagesForTipset(ts *types.TipSet) ([]types.ChainMsg, error) {
	bmsgs, err := cs.BlockMsgsForTipset(ts)
	if err != nil {
		return nil, err
	}

	var out []types.ChainMsg
	for _, bm := range bmsgs {
		for _, blsm := range bm.BlsMessages {
			out = append(out, blsm)
		}

		for _, secm := range bm.SecpkMessages {
			out = append(out, secm)
		}
	}

	return out, nil
}

type mmCids struct {
	bls   []cid.Cid
	secpk []cid.Cid
}

func (cs *ChainStore) ReadMsgMetaCids(mmc cid.Cid) ([]cid.Cid, []cid.Cid, error) {
	o, ok := cs.mmCache.Get(mmc)
	if ok {
		mmcids := o.(*mmCids)
		return mmcids.bls, mmcids.secpk, nil
	}

	cst := cbor.NewCborStore(cs.localbs)
	var msgmeta types.MsgMeta
	if err := cst.Get(context.TODO(), mmc, &msgmeta); err != nil {
		return nil, nil, xerrors.Errorf("failed to load msgmeta (%s): %w", mmc, err)
	}

	blscids, err := cs.readAMTCids(msgmeta.BlsMessages)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading bls message cids for block: %w", err)
	}

	secpkcids, err := cs.readAMTCids(msgmeta.SecpkMessages)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading secpk message cids for block: %w", err)
	}

	cs.mmCache.Add(mmc, &mmCids{
		bls:   blscids,
		secpk: secpkcids,
	})

	return blscids, secpkcids, nil
}

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

func (cs *ChainStore) MessagesForBlock(b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	blscids, secpkcids, err := cs.ReadMsgMetaCids(b.Messages)
	if err != nil {
		return nil, nil, err
	}

	blsmsgs, err := cs.LoadMessagesFromCids(blscids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading bls messages for block: %w", err)
	}

	secpkmsgs, err := cs.LoadSignedMessagesFromCids(secpkcids)
	if err != nil {
		return nil, nil, xerrors.Errorf("loading secpk messages for block: %w", err)
	}

	return blsmsgs, secpkmsgs, nil
}

func (cs *ChainStore) GetParentReceipt(b *types.BlockHeader, i int) (*types.MessageReceipt, error) {
	ctx := context.TODO()
	// block headers use adt0, for now.
	a, err := blockadt.AsArray(cs.Store(ctx), b.ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("amt load: %w", err)
	}

	var r types.MessageReceipt
	if found, err := a.Get(uint64(i), &r); err != nil {
		return nil, err
	} else if !found {
		return nil, xerrors.Errorf("failed to find receipt %d", i)
	}

	return &r, nil
}

func (cs *ChainStore) LoadMessagesFromCids(cids []cid.Cid) ([]*types.Message, error) {
	msgs := make([]*types.Message, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetMessage(c)
		if err != nil {
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", c, i, err)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) LoadSignedMessagesFromCids(cids []cid.Cid) ([]*types.SignedMessage, error) {
	msgs := make([]*types.SignedMessage, 0, len(cids))
	for i, c := range cids {
		m, err := cs.GetSignedMessage(c)
		if err != nil {
			return nil, xerrors.Errorf("failed to get message: (%s):%d: %w", c, i, err)
		}

		msgs = append(msgs, m)
	}

	return msgs, nil
}

func (cs *ChainStore) Blockstore() bstore.Blockstore {
	return cs.bs
}

func ActorStore(ctx context.Context, bs bstore.Blockstore) adt.Store {
	return adt.WrapStore(ctx, cbor.NewCborStore(bs))
}

func (cs *ChainStore) Store(ctx context.Context) adt.Store {
	return ActorStore(ctx, cs.bs)
}

func (cs *ChainStore) VMSys() vm.SyscallBuilder {
	return cs.vmcalls
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

func DrawRandomness(rbase []byte, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	h := blake2b.New256()
	if err := binary.Write(h, binary.BigEndian, int64(pers)); err != nil {
		return nil, xerrors.Errorf("deriving randomness: %w", err)
	}
	VRFDigest := blake2b.Sum256(rbase)
	_, err := h.Write(VRFDigest[:])
	if err != nil {
		return nil, xerrors.Errorf("hashing VRFDigest: %w", err)
	}
	if err := binary.Write(h, binary.BigEndian, round); err != nil {
		return nil, xerrors.Errorf("deriving randomness: %w", err)
	}
	_, err = h.Write(entropy)
	if err != nil {
		return nil, xerrors.Errorf("hashing entropy: %w", err)
	}

	return h.Sum(nil), nil
}

func (cs *ChainStore) GetBeaconRandomness(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	_, span := trace.StartSpan(ctx, "store.GetBeaconRandomness")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("round", int64(round)))

	ts, err := cs.LoadTipSet(types.NewTipSetKey(blks...))
	if err != nil {
		return nil, err
	}

	if round > ts.Height() {
		return nil, xerrors.Errorf("cannot draw randomness from the future")
	}

	searchHeight := round
	if searchHeight < 0 {
		searchHeight = 0
	}

	randTs, err := cs.GetTipsetByHeight(ctx, searchHeight, ts, true)
	if err != nil {
		return nil, err
	}

	be, err := cs.GetLatestBeaconEntry(randTs)
	if err != nil {
		return nil, err
	}

	// if at (or just past -- for null epochs) appropriate epoch
	// or at genesis (works for negative epochs)
	return DrawRandomness(be.Data, pers, round, entropy)
}

func (cs *ChainStore) GetChainRandomness(ctx context.Context, blks []cid.Cid, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	_, span := trace.StartSpan(ctx, "store.GetChainRandomness")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("round", int64(round)))

	ts, err := cs.LoadTipSet(types.NewTipSetKey(blks...))
	if err != nil {
		return nil, err
	}

	if round > ts.Height() {
		return nil, xerrors.Errorf("cannot draw randomness from the future")
	}

	searchHeight := round
	if searchHeight < 0 {
		searchHeight = 0
	}

	randTs, err := cs.GetTipsetByHeight(ctx, searchHeight, ts, true)
	if err != nil {
		return nil, err
	}

	mtb := randTs.MinTicketBlock()

	// if at (or just past -- for null epochs) appropriate epoch
	// or at genesis (works for negative epochs)
	return DrawRandomness(mtb.Ticket.VRFProof, pers, round, entropy)
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

func recurseLinks(bs bstore.Blockstore, walked *cid.Set, root cid.Cid, in []cid.Cid) ([]cid.Cid, error) {
	if root.Prefix().Codec != cid.DagCBOR {
		return in, nil
	}

	data, err := bs.Get(root)
	if err != nil {
		return nil, xerrors.Errorf("recurse links get (%s) failed: %w", root, err)
	}

	var rerr error
	err = cbg.ScanForLinks(bytes.NewReader(data.RawData()), func(c cid.Cid) {
		if rerr != nil {
			// No error return on ScanForLinks :(
			return
		}

		// traversed this already...
		if !walked.Visit(c) {
			return
		}

		in = append(in, c)
		var err error
		in, err = recurseLinks(bs, walked, c, in)
		if err != nil {
			rerr = err
		}
	})
	if err != nil {
		return nil, xerrors.Errorf("scanning for links failed: %w", err)
	}

	return in, rerr
}

func (cs *ChainStore) Export(ctx context.Context, ts *types.TipSet, inclRecentRoots abi.ChainEpoch, skipOldMsgs bool, w io.Writer) error {
	h := &car.CarHeader{
		Roots:   ts.Cids(),
		Version: 1,
	}

	if err := car.WriteHeader(h, w); err != nil {
		return xerrors.Errorf("failed to write car header: %s", err)
	}

	return cs.WalkSnapshot(ctx, ts, inclRecentRoots, skipOldMsgs, func(c cid.Cid) error {
		blk, err := cs.bs.Get(c)
		if err != nil {
			return xerrors.Errorf("writing object to car, bs.Get: %w", err)
		}

		if err := carutil.LdWrite(w, c.Bytes(), blk.RawData()); err != nil {
			return xerrors.Errorf("failed to write block to car output: %w", err)
		}

		return nil
	})
}

func (cs *ChainStore) WalkSnapshot(ctx context.Context, ts *types.TipSet, inclRecentRoots abi.ChainEpoch, skipOldMsgs bool, cb func(cid.Cid) error) error {
	if ts == nil {
		ts = cs.GetHeaviestTipSet()
	}

	seen := cid.NewSet()
	walked := cid.NewSet()

	blocksToWalk := ts.Cids()
	currentMinHeight := ts.Height()

	walkChain := func(blk cid.Cid) error {
		if !seen.Visit(blk) {
			return nil
		}

		if err := cb(blk); err != nil {
			return err
		}

		data, err := cs.bs.Get(blk)
		if err != nil {
			return xerrors.Errorf("getting block: %w", err)
		}

		var b types.BlockHeader
		if err := b.UnmarshalCBOR(bytes.NewBuffer(data.RawData())); err != nil {
			return xerrors.Errorf("unmarshaling block header (cid=%s): %w", blk, err)
		}

		if currentMinHeight > b.Height {
			currentMinHeight = b.Height
			if currentMinHeight%builtin.EpochsInDay == 0 {
				log.Infow("export", "height", currentMinHeight)
			}
		}

		var cids []cid.Cid
		if !skipOldMsgs || b.Height > ts.Height()-inclRecentRoots {
			if walked.Visit(b.Messages) {
				mcids, err := recurseLinks(cs.bs, walked, b.Messages, []cid.Cid{b.Messages})
				if err != nil {
					return xerrors.Errorf("recursing messages failed: %w", err)
				}
				cids = mcids
			}
		}

		if b.Height > 0 {
			for _, p := range b.Parents {
				blocksToWalk = append(blocksToWalk, p)
			}
		} else {
			// include the genesis block
			cids = append(cids, b.Parents...)
		}

		out := cids

		if b.Height == 0 || b.Height > ts.Height()-inclRecentRoots {
			if walked.Visit(b.ParentStateRoot) {
				cids, err := recurseLinks(cs.bs, walked, b.ParentStateRoot, []cid.Cid{b.ParentStateRoot})
				if err != nil {
					return xerrors.Errorf("recursing genesis state failed: %w", err)
				}

				out = append(out, cids...)
			}
		}

		for _, c := range out {
			if seen.Visit(c) {
				if c.Prefix().Codec != cid.DagCBOR {
					continue
				}

				if err := cb(c); err != nil {
					return err
				}

			}
		}

		return nil
	}

	log.Infow("export started")
	exportStart := build.Clock.Now()

	for len(blocksToWalk) > 0 {
		next := blocksToWalk[0]
		blocksToWalk = blocksToWalk[1:]
		if err := walkChain(next); err != nil {
			return xerrors.Errorf("walk chain failed: %w", err)
		}
	}

	log.Infow("export finished", "duration", build.Clock.Now().Sub(exportStart).Seconds())

	return nil
}

func (cs *ChainStore) Import(r io.Reader) (*types.TipSet, error) {
	header, err := car.LoadCar(cs.Blockstore(), r)
	if err != nil {
		return nil, xerrors.Errorf("loadcar failed: %w", err)
	}

	root, err := cs.LoadTipSet(types.NewTipSetKey(header.Roots...))
	if err != nil {
		return nil, xerrors.Errorf("failed to load root tipset from chainfile: %w", err)
	}

	return root, nil
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

type chainRand struct {
	cs   *ChainStore
	blks []cid.Cid
}

func NewChainRand(cs *ChainStore, blks []cid.Cid) vm.Rand {
	return &chainRand{
		cs:   cs,
		blks: blks,
	}
}

func (cr *chainRand) GetChainRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cr.cs.GetChainRandomness(ctx, cr.blks, pers, round, entropy)
}

func (cr *chainRand) GetBeaconRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return cr.cs.GetBeaconRandomness(ctx, cr.blks, pers, round, entropy)
}

func (cs *ChainStore) GetTipSetFromKey(tsk types.TipSetKey) (*types.TipSet, error) {
	if tsk.IsEmpty() {
		return cs.GetHeaviestTipSet(), nil
	}
	return cs.LoadTipSet(tsk)
}
