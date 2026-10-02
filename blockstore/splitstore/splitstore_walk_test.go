package splitstore

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types/mock"
)

// mkLinkBlock returns a DAG-CBOR block holding an array of links.
func mkLinkBlock(t *testing.T, links ...cid.Cid) blocks.Block {
	t.Helper()

	var buf bytes.Buffer
	if err := cbg.WriteMajorTypeHeader(&buf, cbg.MajArray, uint64(len(links))); err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if err := cbg.WriteCid(&buf, l); err != nil {
			t.Fatal(err)
		}
	}

	data := buf.Bytes()
	c, err := abi.CidBuilder.Sum(data)
	if err != nil {
		t.Fatal(err)
	}

	blk, err := blocks.NewBlockWithCid(data, c)
	if err != nil {
		t.Fatal(err)
	}

	return blk
}

// mkRawBlock returns a raw block.
func mkRawBlock(t *testing.T, data []byte) blocks.Block {
	t.Helper()

	c, err := abi.CidBuilder.WithCodec(cid.Raw).Sum(data)
	if err != nil {
		t.Fatal(err)
	}

	blk, err := blocks.NewBlockWithCid(data, c)
	if err != nil {
		t.Fatal(err)
	}

	return blk
}

// codecTestStore is a splitstore that has compacted, discarding cold blocks, a chain whose every
// block carries stateRoot as its parent state root.
type codecTestStore struct {
	*SplitStore
	hot, cold *mockStore
}

func newCodecTestStore(t *testing.T, markSetType string, stateRoot blocks.Block, hotBlocks, coldBlocks []blocks.Block) *codecTestStore {
	t.Helper()

	ctx := context.Background()
	chain := &mockChain{t: t}

	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	hot := newMockStore()
	cold := newMockStore()

	garbage := blocks.NewBlock([]byte{1, 2, 3})
	if err := cold.Put(ctx, garbage); err != nil {
		t.Fatal(err)
	}

	for _, blk := range append(hotBlocks, stateRoot) {
		if err := hot.Put(ctx, blk); err != nil {
			t.Fatal(err)
		}
	}
	for _, blk := range coldBlocks {
		if err := cold.Put(ctx, blk); err != nil {
			t.Fatal(err)
		}
	}

	genBlock := mock.MkBlock(nil, 0, 0)
	genBlock.Messages = garbage.Cid()
	genBlock.ParentMessageReceipts = garbage.Cid()
	genBlock.ParentStateRoot = garbage.Cid()
	genBlock.Timestamp = uint64(time.Now().Unix())

	genTs := mock.TipSet(genBlock)
	chain.push(genTs)

	blk, err := genBlock.ToStorageBlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := cold.Put(ctx, blk); err != nil {
		t.Fatal(err)
	}

	ss, err := Open(t.TempDir(), ds, hot, cold, &Config{MarkSetType: markSetType, DiscardColdBlocks: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	if err := ss.Start(chain, nil); err != nil {
		t.Fatal(err)
	}

	curTs := genTs
	for i := 1; i < 10; i++ {
		hdr := mock.MkBlock(curTs, uint64(i), uint64(i))
		hdr.Messages = garbage.Cid()
		hdr.ParentMessageReceipts = garbage.Cid()
		hdr.ParentStateRoot = stateRoot.Cid()
		hdr.Timestamp = uint64(time.Now().Unix())

		sblk, err := hdr.ToStorageBlock()
		if err != nil {
			t.Fatal(err)
		}
		if err := ss.Put(ctx, sblk); err != nil {
			t.Fatal(err)
		}

		curTs = mock.TipSet(hdr)
		chain.push(curTs)
		waitForCompaction(t, ss)
	}

	if ss.compactionIndex == 0 {
		t.Fatal("no compaction ran; the test proves nothing")
	}

	return &codecTestStore{SplitStore: ss, hot: hot, cold: cold}
}

func waitForCompaction(t *testing.T, ss *SplitStore) {
	t.Helper()

	ss.txnSyncMx.Lock()
	ss.txnSync = true
	ss.txnSyncCond.Broadcast()
	ss.txnSyncMx.Unlock()

	waitIdle(t, ss)
}

func waitIdle(t *testing.T, ss *SplitStore) {
	t.Helper()

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&ss.compacting) == 0
	}, time.Minute, 10*time.Millisecond, "splitstore did not finish compacting")
}

// prune prunes the coldstore, retaining all state.
func (s *codecTestStore) prune(t *testing.T) {
	t.Helper()

	retainAll := func(int64) bool { return true }
	noGC := func() error { return nil }
	if err := s.pruneChain(retainAll, noGC); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, s.SplitStore)

	if s.pruneIndex == 0 {
		t.Fatal("no prune ran; the test proves nothing")
	}
}

func storeHas(t *testing.T, store *mockStore, c cid.Cid) bool {
	t.Helper()

	has, err := store.Has(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}

	return has
}

// TestSplitStoreCompactionCodecShadow checks that a raw CID sharing a DAG-CBOR block's multihash
// does not stop compaction from walking the block's links. See liveMarkSet.
func TestSplitStoreCompactionCodecShadow(t *testing.T) {
	for _, markSetType := range []string{"map", "badger"} {
		t.Run(markSetType, func(t *testing.T) {
			child := mkRawBlock(t, []byte("child of the shadowed state block"))
			shadowed := mkLinkBlock(t, child.Cid())
			alias := cid.NewCidV1(cid.Raw, shadowed.Cid().Hash())
			// alias first, so its mark lands before the DAG-CBOR block is reached
			stateRoot := mkLinkBlock(t, alias, shadowed.Cid())

			ss := newCodecTestStore(t, markSetType, stateRoot, []blocks.Block{child, shadowed}, nil)

			if !storeHas(t, ss.hot, shadowed.Cid()) {
				t.Error("shadowed state block was purged from the hotstore")
			}

			if !storeHas(t, ss.hot, child.Cid()) {
				t.Error("child of the shadowed state block was purged from the hotstore")
			}
		})
	}
}

// TestSplitStoreCompactionRawNotWalked checks that the walk does not follow links out of an
// object reached under a codec it does not scan, even when its bytes are valid DAG-CBOR.
func TestSplitStoreCompactionRawNotWalked(t *testing.T) {
	unlinked := mkRawBlock(t, []byte("reachable only through raw bytes"))
	dag := mkLinkBlock(t, unlinked.Cid())
	// only ever reached under the raw codec
	opaque := cid.NewCidV1(cid.Raw, dag.Cid().Hash())

	stateRoot := mkLinkBlock(t, opaque)

	ss := newCodecTestStore(t, "map", stateRoot, []blocks.Block{unlinked, dag}, nil)

	if !storeHas(t, ss.hot, opaque) {
		t.Error("raw object was purged from the hotstore")
	}

	if storeHas(t, ss.hot, unlinked.Cid()) {
		t.Error("walk followed links out of a raw object")
	}
}

// TestSplitStorePruneCodecShadow is the coldstore counterpart of
// TestSplitStoreCompactionCodecShadow.
func TestSplitStorePruneCodecShadow(t *testing.T) {
	for _, markSetType := range []string{"map", "badger"} {
		t.Run(markSetType, func(t *testing.T) {
			child := mkRawBlock(t, []byte("child of the shadowed state block"))
			shadowed := mkLinkBlock(t, child.Cid())
			alias := cid.NewCidV1(cid.Raw, shadowed.Cid().Hash())
			stateRoot := mkLinkBlock(t, alias, shadowed.Cid())

			ss := newCodecTestStore(t, markSetType, stateRoot, nil, []blocks.Block{child, shadowed})
			ss.prune(t)

			if !storeHas(t, ss.cold, shadowed.Cid()) {
				t.Error("shadowed state block was pruned from the coldstore")
			}

			if !storeHas(t, ss.cold, child.Cid()) {
				t.Error("child of the shadowed state block was pruned from the coldstore")
			}
		})
	}
}

func TestLiveMarkSet(t *testing.T) {
	for _, markSetType := range []string{"map", "badger"} {
		t.Run(markSetType, func(t *testing.T) {
			testLiveMarkSet(t, markSetType)
		})
	}
}

func testLiveMarkSet(t *testing.T, markSetType string) {
	env, err := OpenMarkSetEnv(t.TempDir(), markSetType)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Close() })

	var sets int
	newSet := func() *liveMarkSet {
		sets++
		ms, err := env.New(fmt.Sprintf("live%d", sets), 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ms.Close() })
		return newLiveMarkSet(ms)
	}

	dag := mkLinkBlock(t).Cid()
	raw := cid.NewCidV1(cid.Raw, dag.Hash())
	json := cid.NewCidV1(cid.DagJSON, dag.Hash())

	type step struct {
		c            cid.Cid
		fresh, visit bool
		walked       bool // dag's walked status after this step
	}

	for _, tc := range []struct {
		name  string
		steps []step
	}{{
		name: "dag alone is visited once",
		steps: []step{
			{c: dag, fresh: true, visit: true, walked: true},
			{c: dag, walked: true},
		},
	}, {
		name: "raw first leaves one visit for dag",
		steps: []step{
			{c: raw, fresh: true, visit: true, walked: false},
			{c: raw, walked: false},
			{c: dag, visit: true, walked: true},
			{c: dag, walked: true},
		},
	}, {
		name: "raw after dag costs at most one more visit",
		steps: []step{
			{c: dag, fresh: true, visit: true, walked: true},
			{c: raw, walked: false},
			{c: dag, visit: true, walked: true},
			{c: dag, walked: true},
		},
	}, {
		name: "any codec but DAG-CBOR shadows",
		steps: []step{
			{c: json, fresh: true, visit: true, walked: false},
			{c: raw, walked: false},
			{c: dag, visit: true, walked: true},
			{c: json, walked: true},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSet()
			for i, st := range tc.steps {
				fresh, visit, err := s.markLive(st.c)
				if err != nil {
					t.Fatal(err)
				}
				if fresh != st.fresh || visit != st.visit {
					t.Errorf("step %d (codec %#x): got fresh=%t visit=%t, want fresh=%t visit=%t",
						i, st.c.Prefix().Codec, fresh, visit, st.fresh, st.visit)
				}

				walked, err := s.walked(dag)
				if err != nil {
					t.Fatal(err)
				}
				if walked != st.walked {
					t.Errorf("step %d (codec %#x): got walked=%t, want %t", i, st.c.Prefix().Codec, walked, st.walked)
				}
			}
		})
	}
}

// pausingMarkSet holds a Visit of the paused CID between placing its mark and returning, so
// another walker can be run against the mark alone.
type pausingMarkSet struct {
	MarkSet
	paused         cid.Cid
	marked, resume chan struct{}
}

func (s *pausingMarkSet) Visit(c cid.Cid) (bool, error) {
	fresh, err := s.MarkSet.Visit(c)
	if c.Equals(s.paused) {
		close(s.marked)
		<-s.resume
	}
	return fresh, err
}

func TestLiveMarkSetConcurrent(t *testing.T) {
	newMarkSet := func(t *testing.T) MarkSet {
		env, err := OpenMarkSetEnv(t.TempDir(), "map")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = env.Close() })

		ms, err := env.New("live", 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ms.Close() })

		return ms
	}

	dag := mkLinkBlock(t).Cid()
	raw := cid.NewCidV1(cid.Raw, dag.Hash())

	t.Run("raw mark implies shadow", func(t *testing.T) {
		ms := &pausingMarkSet{
			MarkSet: newMarkSet(t),
			paused:  raw,
			marked:  make(chan struct{}),
			resume:  make(chan struct{}),
		}
		s := newLiveMarkSet(ms)

		done := make(chan error, 1)
		go func() {
			_, _, err := s.markLive(raw)
			done <- err
		}()

		<-ms.marked
		_, visit, err := s.markLive(dag)
		close(ms.resume)
		if err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}

		if !visit {
			t.Error("DAG-CBOR object was denied a visit while its multihash was marked by a raw object")
		}
	})

	t.Run("one resolver per shadow", func(t *testing.T) {
		s := newLiveMarkSet(newMarkSet(t))
		for _, c := range []cid.Cid{dag, raw} {
			if _, _, err := s.markLive(c); err != nil {
				t.Fatal(err)
			}
		}

		const contenders = 16
		var resolvers atomic.Int32
		var wg sync.WaitGroup
		for range contenders {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, visit, err := s.markLive(dag)
				if err != nil {
					t.Error(err)
				}
				if visit {
					resolvers.Add(1)
				}
			}()
		}
		wg.Wait()

		if n := resolvers.Load(); n != 1 {
			t.Errorf("got %d resolvers of one shadow, want 1", n)
		}
	})
}
