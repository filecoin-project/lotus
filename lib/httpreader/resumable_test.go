package httpreader

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewResumableReaderMissingContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("response"))
	}))
	defer server.Close()

	reader, err := NewResumableReader(context.Background(), server.URL)

	require.Nil(t, reader)
	require.ErrorContains(t, err, "invalid syntax")
}

const testETag = `"v1"`

// rangeServer serves data over HTTP/2, honouring Range requests, and
// interrupts the first response halfway. respond handles each later request,
// and returns false to serve it normally.
type rangeServer struct {
	data []byte
	// stall holds the first response open instead of resetting it
	stall   bool
	respond func(n int, w http.ResponseWriter, start int64) bool

	mu       sync.Mutex
	ranges   []string
	ifRanges []string
}

func (s *rangeServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	n := len(s.ranges)
	s.ranges = append(s.ranges, req.Header.Get("Range"))
	s.ifRanges = append(s.ifRanges, req.Header.Get("If-Range"))
	s.mu.Unlock()

	var start int64
	if rng := req.Header.Get("Range"); rng != "" {
		var err error
		start, err = strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("ETag", testETag)
	if n == 0 {
		w.Header().Set("Content-Length", strconv.Itoa(len(s.data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.data[:len(s.data)/2])
		w.(http.Flusher).Flush()
		if s.stall {
			<-req.Context().Done()
			return
		}
		panic(http.ErrAbortHandler)
	}
	if s.respond != nil && s.respond(n, w, start) {
		return
	}
	writePartial(w, s.data, start, int64(len(s.data)))
}

func writePartial(w http.ResponseWriter, data []byte, start, end int64) {
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)-int(start)))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(data[start:end])
	if end < int64(len(data)) {
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}
}

func (s *rangeServer) start(t *testing.T) *ResumableReader {
	return startReader(context.Background(), t, s)
}

func startReader(ctx context.Context, t *testing.T, h http.Handler) *ResumableReader {
	server := httptest.NewUnstartedServer(h)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)

	r, err := newResumableReader(ctx, server.URL, server.Client().Transport)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.backoff = time.Millisecond
	r.maxBackoff = time.Millisecond
	return r
}

func testData(t *testing.T) []byte {
	data := make([]byte, 1<<20)
	_, err := rand.Read(data)
	require.NoError(t, err)
	return data
}

func TestResumableReaderStreamReset(t *testing.T) {
	data := testData(t)
	s := &rangeServer{data: data}
	r := s.start(t)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, got))
	require.Equal(t, []string{"", fmt.Sprintf("bytes=%d-", len(data)/2)}, s.ranges)
	require.Equal(t, []string{"", testETag}, s.ifRanges)

	n, err := r.Read(make([]byte, 10))
	require.Zero(t, n)
	require.Equal(t, io.EOF, err)
	require.Len(t, s.ranges, 2)

	require.NoError(t, r.Close())
	_, err = r.Read(make([]byte, 10))
	require.ErrorIs(t, err, os.ErrClosed)
	require.Len(t, s.ranges, 2)
}

func TestResumableReaderIdleTimeout(t *testing.T) {
	data := testData(t)
	s := &rangeServer{data: data, stall: true}
	r := s.start(t)
	r.idleTimeout = 50 * time.Millisecond

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, got))
	require.Len(t, s.ranges, 2)
}

func TestResumableReaderProgressResetsAttempts(t *testing.T) {
	data := testData(t)
	chunk := int64(len(data) / (4 * maxResumeAttempts))
	s := &rangeServer{data: data, respond: func(_ int, w http.ResponseWriter, start int64) bool {
		writePartial(w, data, start, min(start+chunk, int64(len(data))))
		return true
	}}
	r := s.start(t)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, got))
	require.Greater(t, len(s.ranges), 2*maxResumeAttempts)
}

func TestResumableReaderRetriesServerErrors(t *testing.T) {
	data := testData(t)
	s := &rangeServer{data: data, respond: func(n int, w http.ResponseWriter, _ int64) bool {
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	}}
	r := s.start(t)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, got))
	require.Len(t, s.ranges, 4)
}

func TestResumableReaderGivesUp(t *testing.T) {
	s := &rangeServer{data: testData(t), respond: func(_ int, w http.ResponseWriter, _ int64) bool {
		w.WriteHeader(http.StatusBadGateway)
		return true
	}}
	r := s.start(t)

	_, err := io.ReadAll(r)
	require.ErrorContains(t, err, fmt.Sprintf("after %d attempts", maxResumeAttempts))
	require.ErrorContains(t, err, "status code: 502")
	require.Len(t, s.ranges, 1+maxResumeAttempts)
}

func TestResumableReaderCancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &rangeServer{data: testData(t)}
	r := startReader(ctx, t, s)
	r.backoff = time.Hour
	r.maxBackoff = time.Hour

	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := io.ReadAll(r)
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, s.ranges, 1)
}

func TestResumableReaderRejectsMismatchedResume(t *testing.T) {
	data := testData(t)
	for _, tc := range []struct {
		name    string
		respond func(w http.ResponseWriter, start int64)
		want    string
	}{{
		name:    "range ignored or resource changed",
		respond: func(w http.ResponseWriter, _ int64) { w.WriteHeader(http.StatusOK); _, _ = w.Write(data) },
		want:    "server sent the whole resource",
	}, {
		name: "wrong start",
		respond: func(w http.ResponseWriter, start int64) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start-100, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(data[start-100:])
		},
		want: "got Content-Range",
	}, {
		name: "wrong total",
		respond: func(w http.ResponseWriter, start int64) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data), len(data)+1))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(data[start:])
		},
		want: "got Content-Range",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &rangeServer{data: data, respond: func(_ int, w http.ResponseWriter, start int64) bool {
				tc.respond(w, start)
				return true
			}}
			r := s.start(t)

			_, err := io.ReadAll(r)
			require.ErrorContains(t, err, tc.want)
			require.Len(t, s.ranges, 2)
		})
	}
}

func TestResumableReaderReresolvesRedirect(t *testing.T) {
	data := testData(t)
	s := &rangeServer{data: data, respond: func(n int, w http.ResponseWriter, _ int64) bool {
		if n == 1 {
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		return false
	}}
	var redirects int
	mux := http.NewServeMux()
	mux.Handle("/data", s)
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		redirects++
		http.Redirect(w, req, "/data", http.StatusFound)
	})
	r := startReader(context.Background(), t, mux)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, got))
	require.Len(t, s.ranges, 3)
	require.Equal(t, 2, redirects)
}

// partialTransport answers the first request with a body that returns its
// first half together with an error, and later requests with the range asked
// for.
type partialTransport struct {
	data   []byte
	ranges []string
}

type dataAndErrReader struct{ data []byte }

func (d *dataAndErrReader) Read(p []byte) (int, error) {
	n := copy(p, d.data)
	d.data = d.data[n:]
	return n, errors.New("connection lost")
}

func (pt *partialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	pt.ranges = append(pt.ranges, req.Header.Get("Range"))
	resp := &http.Response{Header: http.Header{}, Request: req}
	if len(pt.ranges) == 1 {
		resp.StatusCode = http.StatusOK
		resp.Header.Set("Content-Length", strconv.Itoa(len(pt.data)))
		resp.Body = io.NopCloser(&dataAndErrReader{data: pt.data[:len(pt.data)/2]})
		return resp, nil
	}
	start, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(req.Header.Get("Range"), "bytes="), "-"))
	if err != nil {
		return nil, err
	}
	resp.StatusCode = http.StatusPartialContent
	resp.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(pt.data)-1, len(pt.data)))
	resp.Body = io.NopCloser(bytes.NewReader(pt.data[start:]))
	return resp, nil
}

func TestResumableReaderDataWithError(t *testing.T) {
	data := testData(t)
	pt := &partialTransport{data: data}
	r, err := newResumableReader(context.Background(), "http://example.invalid/", pt)
	require.NoError(t, err)
	r.backoff = time.Millisecond

	got := make([]byte, len(data))
	_, err = io.ReadFull(r, got)
	require.NoError(t, err)
	require.True(t, bytes.Equal(data, got))
	require.Equal(t, []string{"", fmt.Sprintf("bytes=%d-", len(data)/2)}, pt.ranges)
}
