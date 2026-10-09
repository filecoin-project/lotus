package exchange

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeStream records each write deadline relative to the start time and
// advances a fake clock by perWrite on every write, failing once fail is
// reached.
type fakeStream struct {
	start, clock time.Time
	perWrite     time.Duration
	deadlines    []time.Duration
	writes       int
	fail         int
	short        func(int) int // bytes reported written, without error
}

func (f *fakeStream) now() time.Time { return f.clock }

func (f *fakeStream) SetWriteDeadline(t time.Time) error {
	f.deadlines = append(f.deadlines, t.Sub(f.start))
	return nil
}

func (f *fakeStream) Write(p []byte) (int, error) {
	f.writes++
	if f.fail > 0 && f.writes >= f.fail {
		return 0, errors.New("deadline exceeded")
	}
	f.clock = f.clock.Add(f.perWrite)
	if f.short != nil {
		return f.short(len(p)), nil
	}
	return len(p), nil
}

func newFake(perWrite time.Duration, fail int) *fakeStream {
	start := time.Unix(1000, 0)
	return &fakeStream{start: start, clock: start, perWrite: perWrite, fail: fail}
}

// chunkAllowance is the deadline for one full chunk with a full budget.
var chunkAllowance = ReadResDeadline + transferTime(progressChunk)

func TestProgressWriterZeroProgress(t *testing.T) {
	f := newFake(0, 1)
	_, err := newProgressWriter(f, f.now).Write(make([]byte, 10<<20))
	require.Error(t, err)
	// A non-reading peer holds the stream for one chunk's allowance, not the
	// whole response's.
	require.Equal(t, []time.Duration{chunkAllowance}, f.deadlines)
}

func TestProgressWriterExtends(t *testing.T) {
	f := newFake(time.Second, 0)
	n, err := newProgressWriter(f, f.now).Write(make([]byte, 3*progressChunk))
	require.NoError(t, err)
	require.Equal(t, 3*progressChunk, n)
	require.Equal(t, []time.Duration{
		chunkAllowance,
		time.Second + chunkAllowance,
		2*time.Second + chunkAllowance,
	}, f.deadlines)
}

func TestProgressWriterCapped(t *testing.T) {
	// A healthy reader late in the write is still bounded by WriteResDeadline.
	f := newFake(0, 0)
	pw := newProgressWriter(f, f.now)
	f.clock = f.start.Add(WriteResDeadline - time.Second)
	_, err := pw.Write(make([]byte, progressChunk))
	require.NoError(t, err)
	require.Equal(t, []time.Duration{WriteResDeadline}, f.deadlines)
}

func TestProgressWriterSlowReader(t *testing.T) {
	// 64 KiB every 2s is 32 KiB/s, below ReadResMinSpeed: the budget drains by
	// 0.72s per chunk until the writer gives up.
	f := newFake(2*time.Second, 0)
	n, err := newProgressWriter(f, f.now).Write(make([]byte, 20*progressChunk))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	require.Equal(t, 7*progressChunk, n)
}

func TestProgressWriterShortWrite(t *testing.T) {
	for name, short := range map[string]func(int) int{
		"zero":    func(int) int { return 0 },
		"partial": func(n int) int { return n / 2 },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(0, 0)
			f.short = short
			n, err := newProgressWriter(f, f.now).Write(make([]byte, progressChunk))
			require.ErrorIs(t, err, io.ErrShortWrite)
			require.Equal(t, short(progressChunk), n)
		})
	}
}
