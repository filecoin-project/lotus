package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	blocks "github.com/ipfs/go-libipfs/blocks"
	"github.com/ipld/go-car"
	carutil "github.com/ipld/go-car/util"
	carv2 "github.com/ipld/go-car/v2"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
)

const TipsetkeyBackfillRange = 2 * build.Finality

func (cs *ChainStore) UnionStore() bstore.Blockstore {
	return bstore.Union(cs.stateBlockstore, cs.chainBlockstore)
}

func (cs *ChainStore) ExportRange(ctx context.Context, head, tail *types.TipSet, messages, receipts, stateroots bool, workers int64, cacheSize int, w io.Writer) error {
	h := &car.CarHeader{
		Roots:   head.Cids(),
		Version: 1,
	}

	if err := car.WriteHeader(h, w); err != nil {
		return xerrors.Errorf("failed to write car header: %s", err)
	}

	cacheStore, err := NewCachingBlockstore(cs.UnionStore(), cacheSize)
	if err != nil {
		return err
	}
	return cs.WalkSnapshotRange(ctx, cacheStore, head, tail, messages, receipts, stateroots, workers, func(c cid.Cid) error {
		blk, err := cacheStore.Get(ctx, c)
		if err != nil {
			return xerrors.Errorf("writing object to car, bs.Get: %w", err)
		}

		if err := carutil.LdWrite(w, c.Bytes(), blk.RawData()); err != nil {
			return xerrors.Errorf("failed to write block to car output: %w", err)
		}

		return nil
	})

}

func (cs *ChainStore) Export(ctx context.Context, ts *types.TipSet, inclRecentRoots abi.ChainEpoch, skipOldMsgs bool, w io.Writer) error {
	h := &car.CarHeader{
		Roots:   ts.Cids(),
		Version: 1,
	}

	if err := car.WriteHeader(h, w); err != nil {
		return xerrors.Errorf("failed to write car header: %s", err)
	}

	unionBs := cs.UnionStore()
	return cs.WalkSnapshot(ctx, ts, inclRecentRoots, skipOldMsgs, true, func(c cid.Cid) error {
		blk, err := unionBs.Get(ctx, c)
		if err != nil {
			return xerrors.Errorf("writing object to car, bs.Get: %w", err)
		}

		if err := carutil.LdWrite(w, c.Bytes(), blk.RawData()); err != nil {
			return xerrors.Errorf("failed to write block to car output: %w", err)
		}

		return nil
	})
}

func (cs *ChainStore) Import(ctx context.Context, r io.Reader) (*types.TipSet, error) {
	// TODO: writing only to the state blockstore is incorrect.
	//  At this time, both the state and chain blockstores are backed by the
	//  universal store. When we physically segregate the stores, we will need
	//  to route state objects to the state blockstore, and chain objects to
	//  the chain blockstore.

	br, err := carv2.NewBlockReader(r)
	if err != nil {
		return nil, xerrors.Errorf("loadcar failed: %w", err)
	}

	s := cs.StateBlockstore()

	parallelPuts := 5
	putThrottle := make(chan error, parallelPuts)
	for i := 0; i < parallelPuts; i++ {
		putThrottle <- nil
	}

	var buf []blocks.Block
	for {
		blk, err := br.Next()
		if err != nil {
			if err == io.EOF {
				if len(buf) > 0 {
					if err := s.PutMany(ctx, buf); err != nil {
						return nil, err
					}
				}

				break
			}
			return nil, err
		}

		buf = append(buf, blk)

		if len(buf) > 1000 {
			if lastErr := <-putThrottle; lastErr != nil { // consume one error to have the right to add one
				return nil, lastErr
			}

			go func(buf []blocks.Block) {
				putThrottle <- s.PutMany(ctx, buf)
			}(buf)
			buf = nil
		}
	}

	// check errors
	for i := 0; i < parallelPuts; i++ {
		if lastErr := <-putThrottle; lastErr != nil {
			return nil, lastErr
		}
	}

	root, err := cs.LoadTipSet(ctx, types.NewTipSetKey(br.Roots...))
	if err != nil {
		return nil, xerrors.Errorf("failed to load root tipset from chainfile: %w", err)
	}

	ts := root
	for i := 0; i < int(TipsetkeyBackfillRange); i++ {
		err = cs.PersistTipset(ctx, ts)
		if err != nil {
			return nil, err
		}
		parentTsKey := ts.Parents()
		ts, err = cs.LoadTipSet(ctx, parentTsKey)
		if ts == nil || err != nil {
			log.Warnf("Only able to load the last %d tipsets", i)
			break
		}
	}

	return root, nil
}

type walkTask struct {
	c        cid.Cid
	taskType taskType
}

type walkResult struct {
	c cid.Cid
}

type walkSchedulerConfig struct {
	numWorkers      int64
	tail            *types.TipSet
	includeMessages bool
	includeReceipts bool
	includeState    bool
}

func newWalkScheduler(ctx context.Context, store bstore.Blockstore, cfg *walkSchedulerConfig, rootTasks ...*walkTask) (*walkScheduler, context.Context) {
	tailSet := cid.NewSet()
	for i := range cfg.tail.Cids() {
		tailSet.Add(cfg.tail.Cids()[i])
	}
	grp, ctx := errgroup.WithContext(ctx)
	s := &walkScheduler{
		store:      store,
		numWorkers: cfg.numWorkers,
		stack:      rootTasks,
		in:         make(chan *walkTask, cfg.numWorkers*64),
		out:        make(chan *walkTask, cfg.numWorkers*64),
		grp:        grp,
		tail:       tailSet,
		cfg:        cfg,
	}
	s.taskWg.Add(len(rootTasks))
	return s, ctx
}

type walkScheduler struct {
	store bstore.Blockstore
	// number of worker routine to spawn
	numWorkers int64
	// buffer holds tasks until they are processed
	stack []*walkTask
	// inbound and outbound tasks
	in, out chan *walkTask
	// tracks number of inflight tasks
	taskWg sync.WaitGroup
	// launches workers and collects errors if any occur
	grp *errgroup.Group
	// set of tasks seen
	seen sync.Map

	tail *cid.Set
	cfg  *walkSchedulerConfig
}

func (s *walkScheduler) Wait() error {
	return s.grp.Wait()
}

func (s *walkScheduler) enqueueIfNew(task *walkTask) {
	if task.c.Prefix().MhType == mh.IDENTITY {
		//log.Infow("ignored", "cid", todo.c.String())
		return
	}
	if task.c.Prefix().Codec != cid.Raw && task.c.Prefix().Codec != cid.DagCBOR {
		//log.Infow("ignored", "cid", todo.c.String())
		return
	}
	if _, ok := s.seen.Load(task.c); ok {
		return
	}
	log.Debugw("enqueue", "type", task.taskType.String(), "cid", task.c.String())
	s.taskWg.Add(1)
	s.seen.Store(task.c, struct{}{})
	s.in <- task
}

func (s *walkScheduler) startScheduler(ctx context.Context) {
	s.grp.Go(func() error {
		defer func() {
			log.Infow("walkScheduler shutting down")
			close(s.out)

			// Because the workers may have exited early (due to the context being canceled).
			for range s.out {
				s.taskWg.Done()
			}
			log.Info("closed scheduler out wait group")

			// Because the workers may have enqueued additional tasks.
			for range s.in {
				s.taskWg.Done()
			}
			log.Info("closed scheduler in wait group")

			// now, the waitgroup should be at 0, and the goroutine that was _waiting_ on it should have exited.
			log.Infow("walkScheduler stopped")
		}()
		go func() {
			s.taskWg.Wait()
			close(s.in)
			log.Info("closed scheduler in channel")
		}()
		for {
			if n := len(s.stack) - 1; n >= 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case newJob, ok := <-s.in:
					if !ok {
						return nil
					}
					s.stack = append(s.stack, newJob)
				case s.out <- s.stack[n]:
					s.stack[n] = nil
					s.stack = s.stack[:n]
				}
			} else {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case newJob, ok := <-s.in:
					if !ok {
						return nil
					}
					s.stack = append(s.stack, newJob)
				}
			}
		}
	})
}

func (s *walkScheduler) startWorkers(ctx context.Context, out chan *walkResult) {
	for i := int64(0); i < s.numWorkers; i++ {
		s.grp.Go(func() error {
			for task := range s.out {
				if err := s.work(ctx, task, out); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

type taskType int

func (t taskType) String() string {
	switch t {
	case Block:
		return "block"
	case Message:
		return "message"
	case Receipt:
		return "receipt"
	case State:
		return "state"
	case Dag:
		return "dag"
	}
	panic(fmt.Sprintf("unknow task %d", t))
}

const (
	Block taskType = iota
	Message
	Receipt
	State
	Dag
)

func (s *walkScheduler) work(ctx context.Context, todo *walkTask, results chan *walkResult) error {
	defer s.taskWg.Done()
	// unseen cid, its a result
	results <- &walkResult{c: todo.c}

	// extract relevant dags to walk from the block
	if todo.taskType == Block {
		blk := todo.c
		data, err := s.store.Get(ctx, blk)
		if err != nil {
			return err
		}
		var b types.BlockHeader
		if err := b.UnmarshalCBOR(bytes.NewBuffer(data.RawData())); err != nil {
			return xerrors.Errorf("unmarshalling block header (cid=%s): %w", blk, err)
		}
		if b.Height%1_000 == 0 {
			log.Infow("block export", "height", b.Height)
		}
		if b.Height == 0 {
			log.Info("exporting genesis block")
			for i := range b.Parents {
				s.enqueueIfNew(&walkTask{
					c:        b.Parents[i],
					taskType: Dag,
				})
			}
			s.enqueueIfNew(&walkTask{
				c:        b.ParentStateRoot,
				taskType: State,
			})
			return nil
		}
		// enqueue block parents
		for i := range b.Parents {
			s.enqueueIfNew(&walkTask{
				c:        b.Parents[i],
				taskType: Block,
			})
		}
		if s.cfg.tail.Height() >= b.Height {
			log.Debugw("tail reached", "cid", blk.String())
			return nil
		}

		if s.cfg.includeMessages {
			// enqueue block messages
			s.enqueueIfNew(&walkTask{
				c:        b.Messages,
				taskType: Message,
			})
		}
		if s.cfg.includeReceipts {
			// enqueue block receipts
			s.enqueueIfNew(&walkTask{
				c:        b.ParentMessageReceipts,
				taskType: Receipt,
			})
		}
		if s.cfg.includeState {
			s.enqueueIfNew(&walkTask{
				c:        b.ParentStateRoot,
				taskType: State,
			})
		}

		return nil
	}
	data, err := s.store.Get(ctx, todo.c)
	if err != nil {
		return err
	}
	return cbg.ScanForLinks(bytes.NewReader(data.RawData()), func(c cid.Cid) {
		if todo.c.Prefix().Codec != cid.DagCBOR || todo.c.Prefix().MhType == mh.IDENTITY {
			return
		}

		s.enqueueIfNew(&walkTask{
			c:        c,
			taskType: Dag,
		})
	})
}

func (cs *ChainStore) WalkSnapshotRange(ctx context.Context, store bstore.Blockstore, head, tail *types.TipSet, messages, receipts, stateroots bool, workers int64, cb func(cid.Cid) error) error {
	start := time.Now()
	log.Infow("walking snapshot range", "head", head.Key(), "tail", tail.Key(), "messages", messages, "receipts", receipts, "stateroots", stateroots, "workers", workers, "start", start)
	var tasks []*walkTask
	for i := range head.Blocks() {
		tasks = append(tasks, &walkTask{
			c:        head.Blocks()[i].Cid(),
			taskType: 0,
		})
	}

	cfg := &walkSchedulerConfig{
		numWorkers:      workers,
		tail:            tail,
		includeMessages: messages,
		includeState:    stateroots,
		includeReceipts: receipts,
	}

	pw, ctx := newWalkScheduler(ctx, store, cfg, tasks...)
	// create a buffered channel for exported CID's scaled on the number of workers.
	results := make(chan *walkResult, workers*64)

	pw.startScheduler(ctx)
	// workers accept channel and write results to it.
	pw.startWorkers(ctx, results)

	// used to wait until result channel has been drained.
	resultsDone := make(chan struct{})
	var cbErr error
	go func() {
		// signal we are done draining results when this method exits.
		defer close(resultsDone)
		// drain the results channel until is closes.
		for res := range results {
			if err := cb(res.c); err != nil {
				log.Errorw("export range callback error", "error", err)
				cbErr = err
				return
			}
		}
	}()
	// wait until all workers are done.
	err := pw.Wait()
	if err != nil {
		log.Errorw("walker scheduler", "error", err)
	}
	// workers are done, close the results channel.
	close(results)
	// wait until all results have been consumed before exiting (its buffered).
	<-resultsDone

	// if there was a callback error return it.
	if cbErr != nil {
		return cbErr
	}
	log.Infow("walking snapshot range complete", "duration", time.Since(start), "success", err == nil)

	// return any error encountered by the walker.
	return err
}

func (cs *ChainStore) WalkSnapshot(ctx context.Context, ts *types.TipSet, inclRecentRoots abi.ChainEpoch, skipOldMsgs, skipMsgReceipts bool, cb func(cid.Cid) error) error {
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

		data, err := cs.chainBlockstore.Get(ctx, blk)
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
				mcids, err := recurseLinks(ctx, cs.chainBlockstore, walked, b.Messages, []cid.Cid{b.Messages})
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
				cids, err := recurseLinks(ctx, cs.stateBlockstore, walked, b.ParentStateRoot, []cid.Cid{b.ParentStateRoot})
				if err != nil {
					return xerrors.Errorf("recursing genesis state failed: %w", err)
				}

				out = append(out, cids...)
			}

			if !skipMsgReceipts && walked.Visit(b.ParentMessageReceipts) {
				out = append(out, b.ParentMessageReceipts)
			}
		}

		for _, c := range out {
			if seen.Visit(c) {
				prefix := c.Prefix()

				// Don't include identity CIDs.
				if prefix.MhType == mh.IDENTITY {
					continue
				}

				// We only include raw and dagcbor, for now.
				// Raw for "code" CIDs.
				switch prefix.Codec {
				case cid.Raw, cid.DagCBOR:
				default:
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

func recurseLinks(ctx context.Context, bs bstore.Blockstore, walked *cid.Set, root cid.Cid, in []cid.Cid) ([]cid.Cid, error) {
	if root.Prefix().Codec != cid.DagCBOR {
		return in, nil
	}

	data, err := bs.Get(ctx, root)
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
		in, err = recurseLinks(ctx, bs, walked, c, in)
		if err != nil {
			rerr = err
		}
	})
	if err != nil {
		return nil, xerrors.Errorf("scanning for links failed: %w", err)
	}

	return in, rerr
}
