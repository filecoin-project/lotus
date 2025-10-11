package splitstore

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	mh "github.com/multiformats/go-multihash"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func init() {
	CompactionThreshold = 5
	CompactionBoundary = 2
	WarmupBoundary = 0
	SyncWaitTime = time.Millisecond
	_ = logging.SetLogLevel("splitstore", "DEBUG")
}

func testSplitStore(t *testing.T, cfg *Config) {
	ctx := context.Background()
	chain := &mockChain{t: t}
	fmt.Printf("Config: %v\n", cfg)

	// the myriads of stores
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	hot := newMockStore()
	cold := newMockStore()

	// this is necessary to avoid the garbage mock puts in the blocks
	garbage := blocks.NewBlock([]byte{1, 2, 3})
	err := cold.Put(ctx, garbage)
	if err != nil {
		t.Fatal(err)
	}

	// genesis
	genBlock := mock.MkBlock(nil, 0, 0)
	genBlock.Messages = garbage.Cid()
	genBlock.ParentMessageReceipts = garbage.Cid()
	genBlock.ParentStateRoot = garbage.Cid()
	genBlock.Timestamp = uint64(time.Now().Unix())

	genTs := mock.TipSet(genBlock)
	chain.push(genTs)

	// put the genesis block to cold store
	blk, err := genBlock.ToStorageBlock()
	if err != nil {
		t.Fatal(err)
	}

	err = cold.Put(ctx, blk)
	if err != nil {
		t.Fatal(err)
	}

	// create a garbage block that is protected with a rgistered protector
	protected := blocks.NewBlock([]byte("protected!"))
	err = hot.Put(ctx, protected)
	if err != nil {
		t.Fatal(err)
	}

	// and another one that is not protected
	unprotected := blocks.NewBlock([]byte("unprotected!"))
	err = hot.Put(ctx, unprotected)
	if err != nil {
		t.Fatal(err)
	}

	path := t.TempDir()

	// open the splitstore
	ss, err := Open(path, ds, hot, cold, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close() //nolint

	// register our protector
	ss.AddProtector(func(protect func(cid.Cid) error) error {
		return protect(protected.Cid())
	})

	err = ss.Start(chain, nil)
	if err != nil {
		t.Fatal(err)
	}

	// make some tipsets, but not enough to cause compaction
	mkBlock := func(curTs *types.TipSet, i int, stateRoot blocks.Block) *types.TipSet {
		blk := mock.MkBlock(curTs, uint64(i), uint64(i))

		blk.Messages = garbage.Cid()
		blk.ParentMessageReceipts = garbage.Cid()
		blk.ParentStateRoot = stateRoot.Cid()
		blk.Timestamp = uint64(time.Now().Unix())

		sblk, err := blk.ToStorageBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = ss.Put(ctx, stateRoot)
		if err != nil {
			t.Fatal(err)
		}
		err = ss.Put(ctx, sblk)
		if err != nil {
			t.Fatal(err)
		}
		ts := mock.TipSet(blk)
		chain.push(ts)

		return ts
	}

	waitForCompaction := func() {
		ss.txnSyncMx.Lock()
		ss.txnSync = true
		ss.txnSyncCond.Broadcast()
		ss.txnSyncMx.Unlock()
		for atomic.LoadInt32(&ss.compacting) == 1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	curTs := genTs
	for i := 1; i < 5; i++ {
		stateRoot := blocks.NewBlock([]byte{byte(i), 3, 3, 7})
		curTs = mkBlock(curTs, i, stateRoot)
		waitForCompaction()
	}

	// count objects in the cold and hot stores
	countBlocks := func(bs blockstore.Blockstore) int {
		count := 0
		_ = bs.(blockstore.BlockstoreIterator).ForEachKey(func(_ cid.Cid) error {
			count++
			return nil
		})
		return count
	}

	coldCnt := countBlocks(cold)
	hotCnt := countBlocks(hot)

	if coldCnt != 2 {
		t.Errorf("expected %d blocks, but got %d", 2, coldCnt)
	}

	if hotCnt != 12 {
		t.Errorf("expected %d blocks, but got %d", 12, hotCnt)
	}

	// trigger a compaction
	for i := 5; i < 10; i++ {
		stateRoot := blocks.NewBlock([]byte{byte(i), 3, 3, 7})
		curTs = mkBlock(curTs, i, stateRoot)
		waitForCompaction()
	}

	coldCnt = countBlocks(cold)
	hotCnt = countBlocks(hot)

	if coldCnt != 6 {
		t.Errorf("expected %d cold blocks, but got %d", 6, coldCnt)
	}

	if hotCnt != 18 {
		t.Errorf("expected %d hot blocks, but got %d", 18, hotCnt)
	}

	// ensure our protected block is still there
	has, err := hot.Has(ctx, protected.Cid())
	if err != nil {
		t.Fatal(err)
	}

	if !has {
		t.Fatal("protected block is missing from hotstore")
	}

	// ensure our unprotected block is in the coldstore now
	has, err = hot.Has(ctx, unprotected.Cid())
	if err != nil {
		t.Fatal(err)
	}

	if has {
		t.Fatal("unprotected block is still in hotstore")
	}

	has, err = cold.Has(ctx, unprotected.Cid())
	if err != nil {
		t.Fatal(err)
	}

	if !has {
		t.Fatal("unprotected block is missing from coldstore")
	}

	// Make sure we can revert without panicking.
	chain.revert(2)
}

func TestSplitStoreCompaction(t *testing.T) {
	testSplitStore(t, &Config{MarkSetType: "map", UniversalColdBlocks: true})
}

func TestSplitStoreCompactionWithBadger(t *testing.T) {
	bs := badgerMarkSetBatchSize
	badgerMarkSetBatchSize = 1
	t.Cleanup(func() {
		badgerMarkSetBatchSize = bs
	})
	testSplitStore(t, &Config{MarkSetType: "badger", UniversalColdBlocks: true})
}

func TestSplitStoreSuppressCompactionNearUpgrade(t *testing.T) {
	ctx := context.Background()
	chain := &mockChain{t: t}

	// the myriads of stores
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	hot := newMockStore()
	cold := newMockStore()

	// this is necessary to avoid the garbage mock puts in the blocks
	garbage := blocks.NewBlock([]byte{1, 2, 3})
	err := cold.Put(ctx, garbage)
	if err != nil {
		t.Fatal(err)
	}

	// genesis
	genBlock := mock.MkBlock(nil, 0, 0)
	genBlock.Messages = garbage.Cid()
	genBlock.ParentMessageReceipts = garbage.Cid()
	genBlock.ParentStateRoot = garbage.Cid()
	genBlock.Timestamp = uint64(time.Now().Unix())

	genTs := mock.TipSet(genBlock)
	chain.push(genTs)

	// put the genesis block to cold store
	blk, err := genBlock.ToStorageBlock()
	if err != nil {
		t.Fatal(err)
	}

	err = cold.Put(ctx, blk)
	if err != nil {
		t.Fatal(err)
	}

	path := t.TempDir()

	// open the splitstore
	ss, err := Open(path, ds, hot, cold, &Config{MarkSetType: "map", UniversalColdBlocks: true})
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close() //nolint

	// create an upgrade schedule that will suppress compaction during the test
	upgradeBoundary = 0
	upgrade := stmgr.Upgrade{
		Height:        10,
		PreMigrations: []stmgr.PreMigration{{StartWithin: 10}},
	}

	err = ss.Start(chain, []stmgr.Upgrade{upgrade})
	if err != nil {
		t.Fatal(err)
	}

	mkBlock := func(curTs *types.TipSet, i int, stateRoot blocks.Block) *types.TipSet {
		blk := mock.MkBlock(curTs, uint64(i), uint64(i))

		blk.Messages = garbage.Cid()
		blk.ParentMessageReceipts = garbage.Cid()
		blk.ParentStateRoot = stateRoot.Cid()
		blk.Timestamp = uint64(time.Now().Unix())

		sblk, err := blk.ToStorageBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = ss.Put(ctx, stateRoot)
		if err != nil {
			t.Fatal(err)
		}
		err = ss.Put(ctx, sblk)
		if err != nil {
			t.Fatal(err)
		}
		ts := mock.TipSet(blk)
		chain.push(ts)

		return ts
	}

	waitForCompaction := func() {
		ss.txnSyncMx.Lock()
		ss.txnSync = true
		ss.txnSyncCond.Broadcast()
		ss.txnSyncMx.Unlock()
		for atomic.LoadInt32(&ss.compacting) == 1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	curTs := genTs
	for i := 1; i < 10; i++ {
		stateRoot := blocks.NewBlock([]byte{byte(i), 3, 3, 7})
		curTs = mkBlock(curTs, i, stateRoot)
		waitForCompaction()
	}

	countBlocks := func(bs blockstore.Blockstore) int {
		count := 0
		_ = bs.(blockstore.BlockstoreIterator).ForEachKey(func(_ cid.Cid) error {
			count++
			return nil
		})
		return count
	}

	// we should not have compacted due to suppression and everything should still be hot
	hotCnt := countBlocks(hot)
	coldCnt := countBlocks(cold)

	if hotCnt != 20 {
		t.Errorf("expected %d blocks, but got %d", 20, hotCnt)
	}

	if coldCnt != 2 {
		t.Errorf("expected %d blocks, but got %d", 2, coldCnt)
	}

	// put some more blocks, now we should compact
	for i := 10; i < 20; i++ {
		stateRoot := blocks.NewBlock([]byte{byte(i), 3, 3, 7})
		curTs = mkBlock(curTs, i, stateRoot)
		waitForCompaction()
	}

	hotCnt = countBlocks(hot)
	coldCnt = countBlocks(cold)

	if hotCnt != 24 {
		t.Errorf("expected %d blocks, but got %d", 24, hotCnt)
	}

	if coldCnt != 18 {
		t.Errorf("expected %d blocks, but got %d", 18, coldCnt)
	}
}

func testSplitStoreReification(t *testing.T, f func(context.Context, blockstore.Blockstore, cid.Cid) error) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	hot := newMockStore()
	cold := newMockStore()

	mkRandomBlock := func() blocks.Block {
		data := make([]byte, 128)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatal(err)
		}

		return blocks.NewBlock(data)
	}

	block1 := mkRandomBlock()
	block2 := mkRandomBlock()
	block3 := mkRandomBlock()

	hdr := mock.MkBlock(nil, 0, 0)
	hdr.Messages = block1.Cid()
	hdr.ParentMessageReceipts = block2.Cid()
	hdr.ParentStateRoot = block3.Cid()
	block4, err := hdr.ToStorageBlock()
	if err != nil {
		t.Fatal(err)
	}

	allBlocks := []blocks.Block{block1, block2, block3, block4}
	for _, blk := range allBlocks {
		err := cold.Put(context.Background(), blk)
		if err != nil {
			t.Fatal(err)
		}
	}

	path := t.TempDir()

	ss, err := Open(path, ds, hot, cold, &Config{MarkSetType: "map", UniversalColdBlocks: true})
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close() //nolint

	ss.warmupEpoch.Store(1)
	go ss.reifyOrchestrator()

	waitForReification := func() {
		for {
			ss.reifyMx.Lock()
			ready := len(ss.reifyPend) == 0 && len(ss.reifyInProgress) == 0
			ss.reifyMx.Unlock()

			if ready {
				return
			}

			time.Sleep(time.Millisecond)
		}
	}

	// first access using the standard view
	err = f(context.Background(), ss, block4.Cid())
	if err != nil {
		t.Fatal(err)
	}

	// nothing should be reified
	waitForReification()
	for _, blk := range allBlocks {
		has, err := hot.Has(context.Background(), blk.Cid())
		if err != nil {
			t.Fatal(err)
		}

		if has {
			t.Fatal("block unexpectedly reified")
		}
	}

	// now make the hot/reifying view and ensure access reifies
	err = f(blockstore.WithHotView(context.Background()), ss, block4.Cid())
	if err != nil {
		t.Fatal(err)
	}

	// everything should be reified
	waitForReification()
	for i, blk := range allBlocks {
		has, err := hot.Has(context.Background(), blk.Cid())
		if err != nil {
			t.Fatal(err)
		}

		if !has {
			t.Fatalf("block%d was not reified", i+1)
		}
	}
}

func testSplitStoreReificationLimit(t *testing.T, f func(context.Context, blockstore.Blockstore, cid.Cid) error) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	hot := newMockStore()
	cold := newMockStore()

	mkRandomBlock := func() blocks.Block {
		data := make([]byte, 128)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatal(err)
		}

		return blocks.NewBlock(data)
	}

	block1 := mkRandomBlock()
	block2 := mkRandomBlock()
	block3 := mkRandomBlock()

	hdr := mock.MkBlock(nil, 0, 0)
	hdr.Messages = block1.Cid()
	hdr.ParentMessageReceipts = block2.Cid()
	hdr.ParentStateRoot = block3.Cid()
	block4, err := hdr.ToStorageBlock()
	if err != nil {
		t.Fatal(err)
	}

	allBlocks := []blocks.Block{block1, block2, block3, block4}
	for _, blk := range allBlocks {
		err := cold.Put(context.Background(), blk)
		if err != nil {
			t.Fatal(err)
		}
	}

	path := t.TempDir()

	ss, err := Open(path, ds, hot, cold, &Config{MarkSetType: "map", UniversalColdBlocks: true})
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close() //nolint

	ss.warmupEpoch.Store(1)
	go ss.reifyOrchestrator()

	waitForReification := func() {
		for {
			ss.reifyMx.Lock()
			ready := len(ss.reifyPend) == 0 && len(ss.reifyInProgress) == 0
			ss.reifyMx.Unlock()

			if ready {
				return
			}

			time.Sleep(time.Millisecond)
		}
	}

	// do a hot access -- nothing should be reified as the limit should be exceeded
	oldReifyLimit := ReifyLimit
	ReifyLimit = 2
	t.Cleanup(func() {
		ReifyLimit = oldReifyLimit
	})

	err = f(blockstore.WithHotView(context.Background()), ss, block4.Cid())
	if err != nil {
		t.Fatal(err)
	}

	waitForReification()

	for _, blk := range allBlocks {
		has, err := hot.Has(context.Background(), blk.Cid())
		if err != nil {
			t.Fatal(err)
		}

		if has {
			t.Fatal("block unexpectedly reified")
		}
	}

}

func TestSplitStoreReification(t *testing.T) {
	t.Log("test reification with Has")
	testSplitStoreReification(t, func(ctx context.Context, s blockstore.Blockstore, c cid.Cid) error {
		_, err := s.Has(ctx, c)
		return err
	})
	t.Log("test reification with Get")
	testSplitStoreReification(t, func(ctx context.Context, s blockstore.Blockstore, c cid.Cid) error {
		_, err := s.Get(ctx, c)
		return err
	})
	t.Log("test reification with GetSize")
	testSplitStoreReification(t, func(ctx context.Context, s blockstore.Blockstore, c cid.Cid) error {
		_, err := s.GetSize(ctx, c)
		return err
	})
	t.Log("test reification with View")
	testSplitStoreReification(t, func(ctx context.Context, s blockstore.Blockstore, c cid.Cid) error {
		return s.View(ctx, c, func(_ []byte) error { return nil })
	})
	t.Log("test reification limit")
	testSplitStoreReificationLimit(t, func(ctx context.Context, s blockstore.Blockstore, c cid.Cid) error {
		_, err := s.Has(ctx, c)
		return err
	})
}

type mockChain struct {
	t testing.TB

	sync.Mutex
	genesis  *types.BlockHeader
	tipsets  []*types.TipSet
	listener func(revert []*types.TipSet, apply []*types.TipSet) error
}

func (c *mockChain) push(ts *types.TipSet) {
	c.Lock()
	c.tipsets = append(c.tipsets, ts)
	if c.genesis == nil {
		c.genesis = ts.Blocks()[0]
	}
	c.Unlock()

	if c.listener != nil {
		err := c.listener(nil, []*types.TipSet{ts})
		if err != nil {
			c.t.Errorf("mockchain: error dispatching listener: %s", err)
		}
	}
}

func (c *mockChain) revert(count int) {
	c.Lock()
	revert := make([]*types.TipSet, count)
	if count > len(c.tipsets) {
		c.Unlock()
		c.t.Fatalf("not enough tipsets to revert")
	}
	copy(revert, c.tipsets[len(c.tipsets)-count:])
	c.tipsets = c.tipsets[:len(c.tipsets)-count]
	c.Unlock()

	if c.listener != nil {
		err := c.listener(revert, nil)
		if err != nil {
			c.t.Errorf("mockchain: error dispatching listener: %s", err)
		}
	}
}

func (c *mockChain) GetTipsetByHeight(_ context.Context, epoch abi.ChainEpoch, _ *types.TipSet, _ bool) (*types.TipSet, error) {
	c.Lock()
	defer c.Unlock()

	iEpoch := int(epoch)
	if iEpoch > len(c.tipsets) {
		return nil, fmt.Errorf("bad epoch %d", epoch)
	}

	return c.tipsets[iEpoch], nil
}

func (c *mockChain) GetHeaviestTipSet() *types.TipSet {
	c.Lock()
	defer c.Unlock()

	return c.tipsets[len(c.tipsets)-1]
}

func (c *mockChain) SubscribeHeadChanges(change func(revert []*types.TipSet, apply []*types.TipSet) error) {
	c.listener = change
}

type mockStore struct {
	mx  sync.Mutex
	set map[string]blocks.Block
}

func newMockStore() *mockStore {
	return &mockStore{set: make(map[string]blocks.Block)}
}

func (b *mockStore) keyOf(c cid.Cid) string {
	return string(c.Hash())
}

func (b *mockStore) cidOf(k string) cid.Cid {
	return cid.NewCidV1(cid.Raw, mh.Multihash([]byte(k)))
}

func (b *mockStore) Has(_ context.Context, cid cid.Cid) (bool, error) {
	b.mx.Lock()
	defer b.mx.Unlock()
	_, ok := b.set[b.keyOf(cid)]
	return ok, nil
}

func (b *mockStore) Get(_ context.Context, cid cid.Cid) (blocks.Block, error) {
	b.mx.Lock()
	defer b.mx.Unlock()

	blk, ok := b.set[b.keyOf(cid)]
	if !ok {
		return nil, ipld.ErrNotFound{Cid: cid}
	}
	return blk, nil
}

func (b *mockStore) GetSize(ctx context.Context, cid cid.Cid) (int, error) {
	blk, err := b.Get(ctx, cid)
	if err != nil {
		return 0, err
	}

	return len(blk.RawData()), nil
}

func (b *mockStore) View(ctx context.Context, cid cid.Cid, f func([]byte) error) error {
	blk, err := b.Get(ctx, cid)
	if err != nil {
		return err
	}
	return f(blk.RawData())
}

func (b *mockStore) Put(_ context.Context, blk blocks.Block) error {
	b.mx.Lock()
	defer b.mx.Unlock()

	b.set[b.keyOf(blk.Cid())] = blk
	return nil
}

func (b *mockStore) PutMany(_ context.Context, blks []blocks.Block) error {
	b.mx.Lock()
	defer b.mx.Unlock()

	for _, blk := range blks {
		b.set[b.keyOf(blk.Cid())] = blk
	}
	return nil
}

func (b *mockStore) DeleteBlock(_ context.Context, cid cid.Cid) error {
	b.mx.Lock()
	defer b.mx.Unlock()

	delete(b.set, b.keyOf(cid))
	return nil
}

func (b *mockStore) DeleteMany(_ context.Context, cids []cid.Cid) error {
	b.mx.Lock()
	defer b.mx.Unlock()

	for _, c := range cids {
		delete(b.set, b.keyOf(c))
	}
	return nil
}

func (b *mockStore) Flush(context.Context) error { return nil }

func (b *mockStore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	return nil, errors.New("not implemented")
}

func (b *mockStore) ForEachKey(f func(cid.Cid) error) error {
	b.mx.Lock()
	defer b.mx.Unlock()

	for c := range b.set {
		err := f(b.cidOf(c))
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *mockStore) Close() error {
	return nil
}
