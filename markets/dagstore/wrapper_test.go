package dagstore

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/dagstore/mount"
	"github.com/filecoin-project/dagstore/shard"

	"github.com/filecoin-project/lotus/node/config"
)

// TestWrapperAcquireRecovery verifies that if acquire shard returns a "not found"
// error, the wrapper will attempt to register the shard then reacquire
func TestWrapperAcquireRecovery(t *testing.T) {
	ctx := context.Background()
	pieceCid, err := cid.Parse("bafkqaaa")
	require.NoError(t, err)

	// Create a DAG store wrapper
	dagst, w, err := NewDAGStore(config.DAGStoreConfig{
		RootDir:    t.TempDir(),
		GCInterval: config.Duration(1 * time.Millisecond),
	}, mockLotusMount{})
	require.NoError(t, err)

	defer dagst.Close() //nolint:errcheck

	// Return an error from acquire shard the first time
	acquireShardErr := make(chan error, 1)
	acquireShardErr <- xerrors.Errorf("unknown shard: %w", dagstore.ErrShardUnknown)

	// Create a mock DAG store in place of the real DAG store
	mock := &mockDagStore{
		acquireShardErr: acquireShardErr,
		acquireShardRes: dagstore.ShardResult{
			Accessor: getShardAccessor(t),
		},
		register: make(chan shard.Key, 1),
	}
	w.dagst = mock

	mybs, err := w.LoadShard(ctx, pieceCid)
	require.NoError(t, err)

	// Expect the wrapper to try to recover from the error returned from
	// acquire shard by calling register shard with the same key
	tctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	select {
	case <-tctx.Done():
		require.Fail(t, "failed to call register")
	case k := <-mock.register:
		require.Equal(t, k.String(), pieceCid.String())
	}

	// Verify that we can get things from the acquired blockstore
	var count int
	ch, err := mybs.AllKeysChan(ctx)
	require.NoError(t, err)
	for range ch {
		count++
	}
	require.Greater(t, count, 0)
}

// TestWrapperBackground verifies the behaviour of the background go routine
func TestWrapperBackground(t *testing.T) {
	ctx := context.Background()

	// Create a DAG store wrapper
	dagst, w, err := NewDAGStore(config.DAGStoreConfig{
		RootDir:    t.TempDir(),
		GCInterval: config.Duration(1 * time.Millisecond),
	}, mockLotusMount{})
	require.NoError(t, err)

	defer dagst.Close() //nolint:errcheck

	// Create a mock DAG store in place of the real DAG store
	mock := &mockDagStore{
		gc:      make(chan struct{}, 1),
		recover: make(chan shard.Key, 1),
		close:   make(chan struct{}, 1),
	}
	w.dagst = mock

	// Start up the wrapper
	err = w.Start(ctx)
	require.NoError(t, err)

	// Expect GC to be called automatically
	tctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	select {
	case <-tctx.Done():
		require.Fail(t, "failed to call GC")
	case <-mock.gc:
	}

	// Expect that when the wrapper is closed it will call close on the
	// DAG store
	err = w.Close()
	require.NoError(t, err)

	tctx, cancel3 := context.WithTimeout(ctx, time.Second)
	defer cancel3()
	select {
	case <-tctx.Done():
		require.Fail(t, "failed to call close")
	case <-mock.close:
	}
}

type mockDagStore struct {
	acquireShardErr chan error
	acquireShardRes dagstore.ShardResult
	register        chan shard.Key

	gc      chan struct{}
	recover chan shard.Key
	close   chan struct{}
}

func (m *mockDagStore) DestroyShard(ctx context.Context, key shard.Key, out chan dagstore.ShardResult, _ dagstore.DestroyOpts) error {
	panic("implement me")
}

func (m *mockDagStore) GetShardInfo(k shard.Key) (dagstore.ShardInfo, error) {
	panic("implement me")
}

func (m *mockDagStore) AllShardsInfo() dagstore.AllShardsInfo {
	panic("implement me")
}

func (m *mockDagStore) Start(_ context.Context) error {
	return nil
}

func (m *mockDagStore) RegisterShard(ctx context.Context, key shard.Key, mnt mount.Mount, out chan dagstore.ShardResult, opts dagstore.RegisterOpts) error {
	m.register <- key
	out <- dagstore.ShardResult{Key: key}
	return nil
}

func (m *mockDagStore) AcquireShard(ctx context.Context, key shard.Key, out chan dagstore.ShardResult, _ dagstore.AcquireOpts) error {
	select {
	case err := <-m.acquireShardErr:
		return err
	default:
	}

	out <- m.acquireShardRes
	return nil
}

func (m *mockDagStore) RecoverShard(ctx context.Context, key shard.Key, out chan dagstore.ShardResult, _ dagstore.RecoverOpts) error {
	m.recover <- key
	return nil
}

func (m *mockDagStore) GC(ctx context.Context) (*dagstore.GCResult, error) {
	select {
	case m.gc <- struct{}{}:
	default:
	}

	return nil, nil
}

func (m *mockDagStore) Close() error {
	m.close <- struct{}{}
	return nil
}

type mockLotusMount struct {
}

func (m mockLotusMount) Start(ctx context.Context) error {
	return nil
}

func (m mockLotusMount) FetchUnsealedPiece(context.Context, cid.Cid) (mount.Reader, error) {
	panic("implement me")
}

func (m mockLotusMount) GetUnpaddedCARSize(ctx context.Context, pieceCid cid.Cid) (uint64, error) {
	panic("implement me")
}

func (m mockLotusMount) IsUnsealed(ctx context.Context, pieceCid cid.Cid) (bool, error) {
	panic("implement me")
}

func getShardAccessor(t *testing.T) *dagstore.ShardAccessor {
	data, err := os.ReadFile("./fixtures/sample-rw-bs-v2.car")
	require.NoError(t, err)
	buff := bytes.NewReader(data)
	reader := &mount.NopCloser{Reader: buff, ReaderAt: buff, Seeker: buff}
	shardAccessor, err := dagstore.NewShardAccessor(reader, nil, nil)
	require.NoError(t, err)
	return shardAccessor
}
