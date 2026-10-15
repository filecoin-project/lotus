package blockstore

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	block "github.com/ipfs/go-block-format"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-msgio"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestNetBstore(t *testing.T) {
	ctx := context.Background()

	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	cm := msgio.Combine(msgio.NewWriter(cw), msgio.NewReader(cr))
	sm := msgio.Combine(msgio.NewWriter(sw), msgio.NewReader(sr))

	bbs := NewMemorySync()
	_ = HandleNetBstoreStream(ctx, bbs, sm)

	nbs := NewNetworkStore(cm)

	tb1 := block.NewBlock([]byte("aoeu"))

	h, err := nbs.Has(ctx, tb1.Cid())
	require.NoError(t, err)
	require.False(t, h)

	err = nbs.Put(ctx, tb1)
	require.NoError(t, err)

	h, err = nbs.Has(ctx, tb1.Cid())
	require.NoError(t, err)
	require.True(t, h)

	sz, err := nbs.GetSize(ctx, tb1.Cid())
	require.NoError(t, err)
	require.Equal(t, 4, sz)

	err = nbs.DeleteBlock(ctx, tb1.Cid())
	require.NoError(t, err)

	h, err = nbs.Has(ctx, tb1.Cid())
	require.NoError(t, err)
	require.False(t, h)

	_, err = nbs.Get(ctx, tb1.Cid())
	fmt.Println(err)
	require.True(t, ipld.IsNotFound(err))

	err = nbs.Put(ctx, tb1)
	require.NoError(t, err)

	b, err := nbs.Get(ctx, tb1.Cid())
	require.NoError(t, err)
	require.Equal(t, "aoeu", string(b.RawData()))
}

// idleStream models a connection on which the remote peer never sends
// anything: ReadMsg parks until the stream is closed. WriteMsg accepts
// requests but no response ever arrives.
type idleStream struct {
	readOnce sync.Once
	reading  chan struct{}

	closeOnce  sync.Once
	closed     chan struct{}
	closeCalls atomic.Int64
}

func newIdleStream() *idleStream {
	return &idleStream{
		reading: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (s *idleStream) ReadMsg() ([]byte, error) {
	s.readOnce.Do(func() { close(s.reading) })

	<-s.closed
	return nil, io.EOF
}

func (s *idleStream) NextMsgLen() (int, error) {
	<-s.closed
	return 0, io.EOF
}

func (s *idleStream) ReleaseMsg([]byte) {}

func (s *idleStream) WriteMsg([]byte) error { return nil }

func (s *idleStream) Read([]byte) (int, error) { return 0, xerrors.New("read unsupported") }

func (s *idleStream) Write([]byte) (int, error) { return 0, xerrors.New("write unsupported") }

func (s *idleStream) Close() error {
	s.closeCalls.Add(1)
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

// awaitParked waits until the receive loop is blocked inside ReadMsg. A Stop
// that runs before the loop gets that far is served by the check at the top of
// the loop, which leaves the case these tests cover unexercised.
func (s *idleStream) awaitParked(ctx context.Context, t *testing.T) {
	t.Helper()

	select {
	case <-s.reading:
	case <-ctx.Done():
		t.Fatal("receive loop never reached ReadMsg")
	}
}

var _ msgio.ReadWriteCloser = (*idleStream)(nil)

// testCtx bounds each case so a regression fails the test instead of hanging
// it. The deadline is never reached on the happy path.
func testCtx(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func TestNetBstoreStopIdle(t *testing.T) {
	ctx := testCtx(t)

	stream := newIdleStream()
	nbs := NewNetworkStore(stream)
	stream.awaitParked(ctx, t)

	// The receive loop is parked in ReadMsg, so Stop only returns if it also
	// closes the stream; signalling n.closing alone leaves it there.
	require.NoError(t, nbs.Stop(ctx))

	// Stop is idempotent: a second call used to panic closing a closed channel.
	require.NoError(t, nbs.Stop(ctx))

	// Both Stop and the receive loop's shutdown reach for the stream, but it is
	// closed exactly once.
	require.Equal(t, int64(1), stream.closeCalls.Load())

	// A stopped store rejects new requests rather than blocking on them.
	_, err := nbs.Has(ctx, block.NewBlock([]byte("aoeu")).Cid())
	require.ErrorContains(t, err, "netstore closed")
}

func TestNetBstoreStopConcurrent(t *testing.T) {
	ctx := testCtx(t)

	stream := newIdleStream()
	nbs := NewNetworkStore(stream)
	stream.awaitParked(ctx, t)

	const callers = 8

	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			errs <- nbs.Stop(ctx)
		}()
	}

	for i := 0; i < callers; i++ {
		require.NoError(t, <-errs)
	}

	require.Equal(t, int64(1), stream.closeCalls.Load())
}

func TestNetBstoreStopFailsInflightRequest(t *testing.T) {
	ctx := testCtx(t)

	stream := newIdleStream()
	nbs := NewNetworkStore(stream)
	stream.awaitParked(ctx, t)

	reqErr := make(chan error, 1)
	go func() {
		_, err := nbs.Has(ctx, block.NewBlock([]byte("aoeu")).Cid())
		reqErr <- err
	}()

	// Stop only releases requests that shutdown can see in respMap, so wait for
	// the call above to register before stopping.
	require.Eventually(t, func() bool {
		nbs.respLk.Lock()
		defer nbs.respLk.Unlock()

		return len(nbs.respMap) == 1
	}, 30*time.Second, time.Millisecond)

	require.NoError(t, nbs.Stop(ctx))

	select {
	case err := <-reqErr:
		// A deliberate Stop is reported as such, not as the ReadMsg error that
		// closing the stream produces.
		require.ErrorContains(t, err, "netstore stopping")
	case <-ctx.Done():
		t.Fatal("in-flight request was not released by Stop")
	}
}
