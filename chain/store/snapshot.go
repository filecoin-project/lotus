package store

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"
	carutil "github.com/ipld/go-car/util"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/certstore"
	"github.com/filecoin-project/go-state-types/abi"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

const TipsetkeyBackfillRange = 2 * policy.ChainFinality

func (cs *ChainStore) UnionStore() bstore.Blockstore {
	return bstore.Union(cs.stateBlockstore, cs.chainBlockstore)
}

func (cs *ChainStore) ExportV1(ctx context.Context, ts *types.TipSet, inclRecentRoots abi.ChainEpoch, skipOldMsgs bool, w io.Writer) error {
	h := &car.CarHeader{
		Roots:   ts.Cids(),
		Version: 1,
	}

	if err := car.WriteHeader(h, w); err != nil {
		return xerrors.Errorf("failed to write car header: %s", err)
	}

	return cs.exportToCar(ctx, ts, inclRecentRoots, skipOldMsgs, w)
}

func (cs *ChainStore) ExportV2(ctx context.Context, ts *types.TipSet, inclRecentRoots abi.ChainEpoch, skipOldMsgs bool, certStore *certstore.Store, w io.Writer) error {
	buffer := bytes.NewBuffer(nil)
	metadata := SnapshotMetadata{
		Version:       SnapshotVersion2,
		HeadTipsetKey: ts.Cids(),
	}

	var f3TempFile *os.File
	f3TempFile, err := os.CreateTemp("", "export-f3-snapshot-*.tmp")
	if err != nil {
		return xerrors.Errorf("failed to create temporary file for export F3 snapshot: %w", err)
	}

	defer func() {
		if f3TempFile != nil {
			_ = f3TempFile.Close()
			_ = os.Remove(f3TempFile.Name())
		}
	}()

	if certStore != nil {
		f3Cid, _, err := certStore.ExportLatestSnapshot(ctx, f3TempFile)
		if err != nil {
			log.Warnf("failed to export latest f3 snapshot: %v", err)
		}
		// Reset file position to beginning for later reading
		if _, err := f3TempFile.Seek(0, 0); err != nil {
			return xerrors.Errorf("failed to seek to beginning of F3 temporary file: %w", err)
		}
		if f3Cid != cid.Undef {
			metadata.F3Data = &f3Cid
		}
	}

	if err := metadata.MarshalCBOR(buffer); err != nil {
		return xerrors.Errorf("failed to marshal snapshot metadata: %w", err)
	}

	mCid, err := cid.V1Builder{Codec: cid.DagCBOR, MhType: uint64(mh.BLAKE2B_MIN + 31)}.Sum(buffer.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to calculate CID for snapshot metadata: %w", err)
	}

	h := &car.CarHeader{
		Roots:   []cid.Cid{mCid},
		Version: 1,
	}

	if err := car.WriteHeader(h, w); err != nil {
		return xerrors.Errorf("failed to write car header: %s", err)
	}
	if err := carutil.LdWrite(w, mCid.Bytes(), buffer.Bytes()); err != nil {
		return xerrors.Errorf("failed to write metadata block to car output: %w", err)
	}
	if metadata.F3Data != nil {
		if err := writeF3DataToCar(w, metadata.F3Data.Bytes(), f3TempFile); err != nil {
			return xerrors.Errorf("failed to write f3Data block to car output: %w", err)
		}
	}

	return cs.exportToCar(ctx, ts, inclRecentRoots, skipOldMsgs, w)
}

func writeF3DataToCar(w io.Writer, f3CidBytes []byte, f3Data *os.File) error {
	i, err := f3Data.Stat()
	if err != nil {
		return err
	}

	sum := uint64(len(f3CidBytes)) + uint64(i.Size())

	buf := make([]byte, 8)
	n := binary.PutUvarint(buf, sum)
	_, err = w.Write(buf[:n])
	if err != nil {
		return err
	}

	// write f3Cid
	_, err = w.Write(f3CidBytes)
	if err != nil {
		return err
	}

	// write f3Data
	_, err = io.Copy(w, f3Data)
	if err != nil {
		return err
	}

	return nil
}

func (cs *ChainStore) exportToCar(ctx context.Context, ts *types.TipSet, inclRecentRoots abi.ChainEpoch, skipOldMsgs bool, w io.Writer) error {
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

func (cs *ChainStore) Import(ctx context.Context, f3Ds dtypes.F3DS, r io.Reader) (head *types.TipSet, genesis *types.BlockHeader, err error) {
	// TODO: writing only to the state blockstore is incorrect.
	//  At this time, both the state and chain blockstores are backed by the
	//  universal store. When we physically segregate the stores, we will need
	//  to route state objects to the state blockstore, and chain objects to
	//  the chain blockstore.

	br, err := carv2.NewBlockReader(r)
	if err != nil {
		return nil, nil, xerrors.Errorf("loadcar failed: %w", err)
	}

	s := cs.StateBlockstore()

	parallelPuts := 5
	putThrottle := make(chan error, parallelPuts)
	for i := 0; i < parallelPuts; i++ {
		putThrottle <- nil
	}

	roots := br.Roots

	var nextTailCid cid.Cid

	var tailBlock types.BlockHeader
	tailBlock.Height = abi.ChainEpoch(-1)

	var buf []blocks.Block

	if len(roots) == 0 {
		return nil, nil, xerrors.Errorf("no roots in snapshot car file")
	}

	if len(roots) == V2SnapshotRootCount {
		var metadata SnapshotMetadata
		blk, err := br.Next()
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to read snapshot metadata block: %w", err)
		}

		if err := metadata.UnmarshalCBOR(bytes.NewReader(blk.RawData())); err != nil {
			// Only one cid in roots, but not metadata, maybe it's a genesis block
			log.Infof("failed to unmarshal snapshot metadata block: %v, attempting to parse the block as a genesis block instead", err)
			if err := tailBlock.UnmarshalCBOR(bytes.NewReader(blk.RawData())); err != nil {
				return nil, nil, xerrors.Errorf("failed to unmarshal genesis block: %w", err)
			}
			if len(tailBlock.Parents) > 0 {
				nextTailCid = tailBlock.Parents[0]
			} else {
				// note: even the 0th block has a parent linking to the cbor genesis block
				return nil, nil, xerrors.Errorf("current block (epoch %d cid %s) has no parents", tailBlock.Height, tailBlock.Cid())
			}

			buf = append(buf, blk)
		} else {
			if metadata.F3Data != nil {
				// import f3 snapshot
				cid, f3Reader, _, err := br.NextReader()
				if err != nil {
					return nil, nil, xerrors.Errorf("failed to read F3Data reader: %w", err)
				}
				if cid != *metadata.F3Data {
					return nil, nil, xerrors.Errorf("F3Data CID mismatch")
				}

				f3r := bufio.NewReader(f3Reader)

				prefix := F3DatastorePrefix()
				f3DsWrapper := namespace.Wrap(f3Ds, prefix)

				log.Info("Importing F3Data to datastore")
				if err := certstore.ImportSnapshotToDatastore(ctx, f3r, f3DsWrapper); err != nil {
					return nil, nil, xerrors.Errorf("failed to import f3Data to datastore: %w", err)
				}
			}

			roots = metadata.HeadTipsetKey
			nextTailCid = roots[0]
		}
	} else {
		// V1 format: roots directly contains the tipset CIDs
		nextTailCid = roots[0]
	}

	for {
		blk, err := br.Next()
		if err != nil {

			// we're at the end
			if err == io.EOF {
				if len(buf) > 0 {
					if err := s.PutMany(ctx, buf); err != nil {
						return nil, nil, err
					}
				}

				break
			}
			return nil, nil, err
		}

		// check for header block, looking for genesis
		if blk.Cid() == nextTailCid && tailBlock.Height != 0 {
			if err := tailBlock.UnmarshalCBOR(bytes.NewReader(blk.RawData())); err != nil {
				return nil, nil, xerrors.Errorf("failed to unmarshal genesis block: %w", err)
			}
			if len(tailBlock.Parents) > 0 {
				nextTailCid = tailBlock.Parents[0]
			} else {
				// note: even the 0th block has a parent linking to the cbor genesis block
				return nil, nil, xerrors.Errorf("current block (epoch %d cid %s) has no parents", tailBlock.Height, tailBlock.Cid())
			}
		}

		// append to batch
		buf = append(buf, blk)

		if len(buf) > 1000 {
			if lastErr := <-putThrottle; lastErr != nil { // consume one error to have the right to add one
				return nil, nil, lastErr
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
			return nil, nil, lastErr
		}
	}

	if tailBlock.Height != 0 {
		return nil, nil, xerrors.Errorf("expected genesis block to have height 0 (genesis), got %d: %s", tailBlock.Height, tailBlock.Cid())
	}

	root, err := cs.LoadTipSet(ctx, types.NewTipSetKey(roots...))
	if err != nil {
		return nil, nil, xerrors.Errorf("failed to load root tipset from chainfile: %w", err)
	}

	ts := root
	tssToPersist := make([]*types.TipSet, 0, TipsetkeyBackfillRange)
	for i := 0; i < int(TipsetkeyBackfillRange); i++ {
		tssToPersist = append(tssToPersist, ts)
		parentTsKey := ts.Parents()
		ts, err = cs.LoadTipSet(ctx, parentTsKey)
		if ts == nil || err != nil {
			log.Warnf("Only able to load the last %d tipsets", i)
			break
		}
	}

	if err := cs.PersistTipsets(ctx, tssToPersist); err != nil {
		return nil, nil, xerrors.Errorf("failed to persist tipsets: %w", err)
	}

	return root, &tailBlock, nil
}

func F3DatastorePrefix() datastore.Key {
	nn := buildconstants.F3Manifest().NetworkName
	return datastore.NewKey("/f3/" + string(nn))
}

type walkSchedTaskType int

const (
	finishTask walkSchedTaskType = -1
	blockTask  walkSchedTaskType = iota
	messageTask
	receiptTask
	stateTask
	dagTask
)

func (t walkSchedTaskType) String() string {
	switch t {
	case finishTask:
		return "finish"
	case blockTask:
		return "block"
	case messageTask:
		return "message"
	case receiptTask:
		return "receipt"
	case stateTask:
		return "state"
	case dagTask:
		return "dag"
	}
	panic(fmt.Sprintf("unknown task %d", t))
}

type walkTask struct {
	c                cid.Cid
	taskType         walkSchedTaskType
	topLevelTaskType walkSchedTaskType
	blockCid         cid.Cid
	epoch            abi.ChainEpoch
}

// an ever growing FIFO
type taskFifo struct {
	in   chan walkTask
	out  chan walkTask
	fifo []walkTask
}

type taskResult struct {
	c cid.Cid
	b blocks.Block
}

func newTaskFifo(bufferLen int) *taskFifo {
	f := taskFifo{
		in:   make(chan walkTask, bufferLen),
		out:  make(chan walkTask, bufferLen),
		fifo: make([]walkTask, 0),
	}

	go f.run()

	return &f
}

func (f *taskFifo) Close() error {
	close(f.in)
	return nil
}

func (f *taskFifo) run() {
	for {
		if len(f.fifo) > 0 {
			// we have items in slice
			// try to put next out or read something in.
			// blocks if nothing works.
			next := f.fifo[0]
			select {
			case f.out <- next:
				f.fifo = f.fifo[1:]
			case elem, ok := <-f.in:
				if !ok {
					// drain and close out.
					for _, elem := range f.fifo {
						f.out <- elem
					}
					close(f.out)
					return
				}
				f.fifo = append(f.fifo, elem)
			}
		} else {
			// no elements in fifo to put out.
			// Try to read in and block.
			// When done, try to put out or add to fifo.
			select {
			case elem, ok := <-f.in:
				if !ok {
					close(f.out)
					return
				}
				select {
				case f.out <- elem:
				default:
					f.fifo = append(f.fifo, elem)
				}
			}
		}
	}
}

type walkSchedulerConfig struct {
	numWorkers int

	head            *types.TipSet // Tipset to start walking from.
	tail            *types.TipSet // Tipset to end at.
	includeMessages bool
	includeReceipts bool
	includeState    bool
}

type walkScheduler struct {
	ctx    context.Context
	cancel context.CancelFunc

	store  bstore.Blockstore
	cfg    walkSchedulerConfig
	writer io.Writer

	workerTasks    *taskFifo
	totalTasks     atomic.Int64
	results        chan taskResult
	writeErrorChan chan error

	// tracks number of inflight tasks
	//taskWg sync.WaitGroup
	// launches workers and collects errors if any occur
	workers *errgroup.Group
	// set of CIDs already exported
	seen sync.Map
}

func newWalkScheduler(ctx context.Context, store bstore.Blockstore, cfg walkSchedulerConfig, w io.Writer) (*walkScheduler, error) {
	ctx, cancel := context.WithCancel(ctx)
	workers, ctx := errgroup.WithContext(ctx)
	s := &walkScheduler{
		ctx:            ctx,
		cancel:         cancel,
		store:          store,
		cfg:            cfg,
		writer:         w,
		results:        make(chan taskResult, cfg.numWorkers*64),
		workerTasks:    newTaskFifo(cfg.numWorkers * 64),
		writeErrorChan: make(chan error, 1),
		workers:        workers,
	}

	go func() {
		defer close(s.writeErrorChan)
		for r := range s.results {
			// Write
			if err := carutil.LdWrite(s.writer, r.c.Bytes(), r.b.RawData()); err != nil {
				// abort operations
				cancel()
				s.writeErrorChan <- err
			}
		}
	}()

	// workers
	for i := 0; i < cfg.numWorkers; i++ {
		f := func(n int) func() error {
			return func() error {
				return s.workerFunc(n)
			}
		}(i)
		s.workers.Go(f)
	}

	s.totalTasks.Add(int64(len(cfg.head.Blocks())))
	for _, b := range cfg.head.Blocks() {
		select {
		case <-ctx.Done():
			log.Errorw("context done while sending root tasks", ctx.Err())
			cancel() // kill workers
			return nil, ctx.Err()
		case s.workerTasks.in <- walkTask{
			c:                b.Cid(),
			taskType:         blockTask,
			topLevelTaskType: blockTask,
			blockCid:         b.Cid(),
			epoch:            cfg.head.Height(),
		}:
		}
	}

	return s, nil
}

func (s *walkScheduler) Wait() error {
	err := s.workers.Wait()
	// all workers done. One would have reached genesis and notified the
	// rest to exit. Yet, there might be some pending tasks in the queue,
	// so we need to run a "single worker".
	if err != nil {
		log.Errorw("export workers finished with error", "error", err)
	}

	for {
		if n := s.totalTasks.Load(); n == 0 {
			break // finally fully done
		}
		select {
		case task := <-s.workerTasks.out:
			s.totalTasks.Add(-1)
			if err != nil {
				continue // just drain if errors happened.
			}
			err = s.processTask(task, 0)
		}
	}
	close(s.results)
	errWrite := <-s.writeErrorChan
	if errWrite != nil {
		log.Errorw("error writing to CAR file", "error", err)
		return errWrite
	}
	_ = s.workerTasks.Close()
	return err
}

func (s *walkScheduler) enqueueIfNew(task walkTask) {
	if multicodec.Code(task.c.Prefix().MhType) == multicodec.Identity {
		//log.Infow("ignored", "cid", todo.c.String())
		return
	}

	// This lets through RAW, CBOR, and DagCBOR blocks, the only types that we end up writing to
	// the exported CAR.
	switch multicodec.Code(task.c.Prefix().Codec) {
	case multicodec.Cbor, multicodec.DagCbor, multicodec.Raw:
	default:
		//log.Infow("ignored", "cid", todo.c.String())
		return
	}
	if _, loaded := s.seen.LoadOrStore(task.c, struct{}{}); loaded {
		// we already had it on the map
		return
	}

	log.Debugw("enqueue", "type", task.taskType.String(), "cid", task.c.String())
	s.totalTasks.Add(1)
	s.workerTasks.in <- task
}

func (s *walkScheduler) sendFinish(workerN int) error {
	log.Infow("worker finished work", "worker", workerN)
	s.totalTasks.Add(1)
	s.workerTasks.in <- walkTask{
		taskType: finishTask,
	}
	return nil
}

func (s *walkScheduler) workerFunc(workerN int) error {
	log.Infow("starting worker", "worker", workerN)
	for t := range s.workerTasks.out {
		s.totalTasks.Add(-1)
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
			// A worker reached genesis, so we wind down and let others do
			// the same. Exit.
			if t.taskType == finishTask {
				return s.sendFinish(workerN)
			}
		}

		err := s.processTask(t, workerN)
		if err != nil {
			return err
		}
		// continue
	}
	return nil
}

func (s *walkScheduler) processTask(t walkTask, workerN int) error {
	if t.taskType == finishTask {
		return nil
	}

	blk, err := s.store.Get(s.ctx, t.c)
	if errors.Is(err, format.ErrNotFound{}) && t.topLevelTaskType == receiptTask {
		log.Debugw("ignoring not-found block in Receipts",
			"block", t.blockCid,
			"epoch", t.epoch,
			"cid", t.c)
		return nil
	}
	if err != nil {
		return xerrors.Errorf(
			"blockstore.Get(%s). Task: %s. Block: %s (%s). Epoch: %d. Err: %w",
			t.c, t.taskType, t.topLevelTaskType, t.blockCid, t.epoch, err)
	}

	s.results <- taskResult{
		c: t.c,
		b: blk,
	}

	// We exported the ipld block. If it wasn't a CBOR block, there's nothing
	// else to do and we can bail out early as it won't have any links
	// etc.
	if multicodec.Code(t.c.Prefix().Codec) != multicodec.DagCbor ||
		multicodec.Code(t.c.Prefix().MhType) == multicodec.Identity {
		return nil
	}

	rawData := blk.RawData()

	// extract relevant dags to walk from the block
	if t.taskType == blockTask {
		var b types.BlockHeader
		if err := b.UnmarshalCBOR(bytes.NewBuffer(rawData)); err != nil {
			return xerrors.Errorf("unmarshalling block header (cid=%s): %w", blk, err)
		}
		if b.Height%1_000 == 0 {
			log.Infow("block export", "height", b.Height)
		}
		if b.Height == 0 {
			log.Info("exporting genesis block")
			for i := range b.Parents {
				s.enqueueIfNew(walkTask{
					c:                b.Parents[i],
					taskType:         dagTask,
					topLevelTaskType: blockTask,
					blockCid:         b.Parents[i],
					epoch:            0,
				})
			}
			s.enqueueIfNew(walkTask{
				c:                b.ParentStateRoot,
				taskType:         stateTask,
				topLevelTaskType: stateTask,
				blockCid:         t.c,
				epoch:            0,
			})

			return s.sendFinish(workerN)
		}
		// enqueue block parents
		for i := range b.Parents {
			s.enqueueIfNew(walkTask{
				c:                b.Parents[i],
				taskType:         blockTask,
				topLevelTaskType: blockTask,
				blockCid:         b.Parents[i],
				epoch:            b.Height,
			})
		}
		if s.cfg.tail.Height() >= b.Height {
			log.Debugw("tail reached: only blocks will be exported from now until genesis", "cid", t.c.String())
			return nil
		}

		if s.cfg.includeMessages {
			// enqueue block messages
			s.enqueueIfNew(walkTask{
				c:                b.Messages,
				taskType:         messageTask,
				topLevelTaskType: messageTask,
				blockCid:         t.c,
				epoch:            b.Height,
			})
		}
		if s.cfg.includeReceipts {
			// enqueue block receipts
			s.enqueueIfNew(walkTask{
				c:                b.ParentMessageReceipts,
				taskType:         receiptTask,
				topLevelTaskType: receiptTask,
				blockCid:         t.c,
				epoch:            b.Height,
			})
		}
		if s.cfg.includeState {
			s.enqueueIfNew(walkTask{
				c:                b.ParentStateRoot,
				taskType:         stateTask,
				topLevelTaskType: stateTask,
				blockCid:         t.c,
				epoch:            b.Height,
			})
		}

		return nil
	}

	// Not a chain-block: we scan for CIDs in the raw block-data
	err = cbg.ScanForLinks(bytes.NewReader(rawData), func(c cid.Cid) {
		s.enqueueIfNew(walkTask{
			c:                c,
			taskType:         dagTask,
			topLevelTaskType: t.topLevelTaskType,
			blockCid:         t.blockCid,
			epoch:            t.epoch,
		})
	})

	if err != nil {
		return xerrors.Errorf(
			"ScanForLinks(%s). Task: %s. Block: %s (%s). Epoch: %d. Err: %w",
			t.c, t.taskType, t.topLevelTaskType, t.blockCid, t.epoch, err)
	}
	return nil
}

func (cs *ChainStore) ExportRange(
	ctx context.Context,
	w io.Writer,
	head, tail *types.TipSet,
	messages, receipts, stateroots bool,
	workers int) error {

	h := &car.CarHeader{
		Roots:   head.Cids(),
		Version: 1,
	}

	if err := car.WriteHeader(h, w); err != nil {
		return xerrors.Errorf("failed to write car header: %s", err)
	}

	start := time.Now()
	log.Infow("walking snapshot range",
		"head", head.Key(),
		"tail", tail.Key(),
		"messages", messages,
		"receipts", receipts,
		"stateroots",
		stateroots,
		"workers", workers)

	cfg := walkSchedulerConfig{
		numWorkers:      workers,
		head:            head,
		tail:            tail,
		includeMessages: messages,
		includeState:    stateroots,
		includeReceipts: receipts,
	}

	pw, err := newWalkScheduler(ctx, cs.UnionStore(), cfg, w)
	if err != nil {
		return err
	}

	// wait until all workers are done.
	err = pw.Wait()
	if err != nil {
		log.Errorw("walker scheduler", "error", err)
		return err
	}

	log.Infow("walking snapshot range complete", "duration", time.Since(start), "success", err == nil)
	return nil
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
			blocksToWalk = append(blocksToWalk, b.Parents...)
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
				if multicodec.Code(prefix.MhType) == multicodec.Identity {
					continue
				}

				// We only include raw, cbor, and dagcbor, for now.
				switch multicodec.Code(prefix.Codec) {
				case multicodec.Cbor, multicodec.DagCbor, multicodec.Raw:
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
	exportStart := time.Now()

	for len(blocksToWalk) > 0 {
		next := blocksToWalk[0]
		blocksToWalk = blocksToWalk[1:]
		if err := walkChain(next); err != nil {
			return xerrors.Errorf("walk chain failed: %w", err)
		}
	}

	log.Infow("export finished", "duration", time.Since(exportStart).Seconds())

	return nil
}

func recurseLinks(ctx context.Context, bs bstore.Blockstore, walked *cid.Set, root cid.Cid, in []cid.Cid) ([]cid.Cid, error) {
	if multicodec.Code(root.Prefix().Codec) != multicodec.DagCbor {
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
