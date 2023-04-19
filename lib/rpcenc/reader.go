package rpcenc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/httpreader"
	"github.com/filecoin-project/lotus/storage/pipeline/lib/nullreader"
)

var log = logging.Logger("rpcenc")

var Timeout = 30 * time.Second

type StreamType string

const (
	Null       StreamType = "null"
	PushStream StreamType = "push"
	HTTP       StreamType = "http"
	// TODO: Data transfer handoff to workers?
)

type ReaderStream struct {
	Type StreamType
	Info string
}

var client = func() *http.Client {
	c := *http.DefaultClient
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &c
}()

/*

	Example rpc function:
		Push(context.Context, io.Reader) error

	Request flow:
	1. Client invokes a method with an io.Reader param
	2. go-jsonrpc invokes `ReaderParamEncoder` for the client-provided io.Reader
	3. `ReaderParamEncoder` transforms the reader into a `ReaderStream` which can
	   be serialized as JSON, and sent as jsonrpc request parameter
		3.1. If the reader is of type `*sealing.NullReader`, the resulting object
		     is `ReaderStream{ Type: "null", Info: "[base 10 number of bytes]" }`
		3.2. If the reader is of type `*RpcReader`, and it wasn't read from, we
		     notify that RpcReader to go a different push endpoint, and return
             a `ReaderStream` object like in 3.4.
		3.3. In remaining cases we start a goroutine which:
			3.3.1. Makes a HEAD request to the server push endpoint
			3.3.2. If the HEAD request is redirected, it follows the redirect
			3.3.3. If the request succeeds, it starts a POST request to the
			       endpoint to which the last HEAD request was sent with the
			       reader set as request body.
		3.4. We return a `ReaderStream` indicating the uuid of push request, ex:
		     `ReaderStream{ Type: "push", Info: "[UUID string]" }`
	4. If the reader wasn't a NullReader, the server will receive a HEAD (or
	   POST in case of older clients) request to the push endpoint.
		4.1. The server gets or registers an `*RpcReader` in the `readers` map.
		4.2. It waits for a request to a matching push endpoint to be opened
		4.3. After the request is opened, it returns the `*RpcReader` to
		     go-jsonrpc, which will pass it as the io.Reader parameter to the
		     rpc method implementation
		4.4. If the first request made to the push endpoint was a POST, the
		     returned `*RpcReader` acts as a simple reader reading the POST
		     request body
		4.5. If the first request made to the push endpoint was a HEAD
		4.5.1. On the first call to Read or Close the server responds with
		       a 200 OK header, the client starts a POST request to the same
		       push URL, and the reader starts passing through the POST request
		       body
		4.5.2. If the reader is passed to another (now client) RPC method as a
		       reader parameter, the server for the first request responds to the
		       HEAD request with http 302 Found, instructing the first client to
		       go to the push endpoint of the second RPC server
	5. If the reader was a NullReader (ReaderStream.Type=="null"), we instantiate
	   it, and provide to the method implementation

*/

func ReaderParamEncoder(addr string) jsonrpc.Option {
	// Client side parameter encoder. Runs on the rpc client side. io.Reader -> ReaderStream{}
	return jsonrpc.WithParamEncoder(new(io.Reader), func(value reflect.Value) (reflect.Value, error) {
		r := value.Interface().(io.Reader)

		if r, ok := r.(*nullreader.NullReader); ok {
			return reflect.ValueOf(ReaderStream{Type: Null, Info: fmt.Sprint(r.N)}), nil
		}
		if r, ok := r.(*httpreader.HttpReader); ok && r.URL != "" {
			return reflect.ValueOf(ReaderStream{Type: HTTP, Info: r.URL}), nil
		}

		reqID := uuid.New()
		u, err := url.Parse(addr)
		if err != nil {
			return reflect.Value{}, xerrors.Errorf("parsing push address: %w", err)
		}
		u.Path = path.Join(u.Path, reqID.String())

		rpcReader, redir := r.(*RpcReader)
		if redir {
			// if we have an rpc stream, redirect instead of proxying all the data
			redir = rpcReader.redirect(u.String())
		}

		if !redir {
			go func() {
				// TODO: figure out errors here
				for {
					req, err := http.NewRequest("HEAD", u.String(), nil)
					if err != nil {
						log.Errorf("sending HEAD request for the reder param: %+v", err)
						return
					}
					req.Header.Set("Content-Type", "application/octet-stream")
					resp, err := client.Do(req)
					if err != nil {
						log.Errorf("sending reader param: %+v", err)
						return
					}
					// todo do we need to close the body for a head request?

					if resp.StatusCode == http.StatusFound {
						nextStr := resp.Header.Get("Location")
						u, err = url.Parse(nextStr)
						if err != nil {
							log.Errorf("sending HEAD request for the reder param, parsing next url (%s): %+v", nextStr, err)
							return
						}

						continue
					}

					if resp.StatusCode == http.StatusNoContent { // reader closed before reading anything
						// todo just return??
						return
					}

					if resp.StatusCode != http.StatusOK {
						b, _ := io.ReadAll(resp.Body)
						log.Errorf("sending reader param (%s): non-200 status: %s, msg: '%s'", u.String(), resp.Status, string(b))
						return
					}

					break
				}

				// now actually send the data
				req, err := http.NewRequest("POST", u.String(), r)
				if err != nil {
					log.Errorf("sending reader param: %+v", err)
					return
				}
				req.Header.Set("Content-Type", "application/octet-stream")
				resp, err := client.Do(req)
				if err != nil {
					log.Errorf("sending reader param: %+v", err)
					return
				}

				defer resp.Body.Close() //nolint

				if resp.StatusCode != http.StatusOK {
					b, _ := io.ReadAll(resp.Body)
					log.Errorf("sending reader param (%s): non-200 status: %s, msg: '%s'", u.String(), resp.Status, string(b))
					return
				}
			}()
		}

		return reflect.ValueOf(ReaderStream{Type: PushStream, Info: reqID.String()}), nil
	})
}

type resType int

const (
	resStart    resType = iota // send on first read after HEAD
	resRedirect                // send on redirect before first read after HEAD
	resError
	// done/closed = close res channel
)

type readRes struct {
	rt   resType
	meta string
}

// RpcReader watches the ReadCloser and closes the res channel when
// either: (1) the ReaderCloser fails on Read (including with a benign error
// like EOF), or (2) when Close is called.
//
// Use it be notified of terminal states, in situations where a Read failure (or
// EOF) is considered a terminal state too (besides Close).
type RpcReader struct {
	postBody     io.ReadCloser   // nil on initial head request
	next         chan *RpcReader // on head will get us the postBody after sending resStart
	mustRedirect bool
	eof          bool

	res       chan readRes
	beginOnce *sync.Once
	closeOnce *sync.Once
}

var ErrHasBody = errors.New("RPCReader has body, either already read from or from a client with no redirect support")
var ErrMustRedirect = errors.New("reader can't be read directly; marked as MustRedirect")

// MustRedirect marks the reader as required to be redirected. Will make local
// calls Read fail. MUST be called before this reader is used in any goroutine.
// If the reader can't be redirected will return ErrHasBody
func (w *RpcReader) MustRedirect() error {
	if w.postBody != nil {
		w.closeOnce.Do(func() {
			w.res <- readRes{
				rt: resError,
			}
			close(w.res)
		})

		return ErrHasBody
	}

	w.mustRedirect = true
	return nil
}

func (w *RpcReader) beginPost() {
	if w.mustRedirect {
		w.res <- readRes{
			rt: resError,
		}
		close(w.res)
		return
	}

	if w.postBody == nil {
		w.res <- readRes{
			rt: resStart,
		}

		nr := <-w.next

		w.postBody = nr.postBody
		w.res = nr.res
		w.beginOnce = nr.beginOnce
		w.closeOnce = nr.closeOnce
	}
}

func (w *RpcReader) Read(p []byte) (int, error) {
	w.beginOnce.Do(func() {
		w.beginPost()
	})

	if w.eof {
		return 0, io.EOF
	}

	if w.mustRedirect {
		return 0, ErrMustRedirect
	}

	if w.postBody == nil {
		return 0, xerrors.Errorf("reader already closed, redirected or cancelled")
	}

	n, err := w.postBody.Read(p)
	if err != nil {
		if err == io.EOF {
			w.eof = true
		}
		w.closeOnce.Do(func() {
			close(w.res)
		})
	}
	return n, err
}

func (w *RpcReader) Close() error {
	w.beginOnce.Do(func() {})
	w.closeOnce.Do(func() {
		close(w.res)
	})
	if w.postBody == nil {
		return nil
	}
	return w.postBody.Close()
}

func (w *RpcReader) redirect(to string) bool {
	if w.postBody != nil {
		return false
	}

	done := false

	w.beginOnce.Do(func() {
		w.closeOnce.Do(func() {
			w.res <- readRes{
				rt:   resRedirect,
				meta: to,
			}

			done = true
			close(w.res)
		})
	})

	return done
}

func ReaderParamDecoder() (http.HandlerFunc, jsonrpc.ServerOption) {
	var readersLk sync.Mutex
	readers := map[uuid.UUID]chan *RpcReader{}

	// runs on the rpc server side, called by the client before making the jsonrpc request
	hnd := func(resp http.ResponseWriter, req *http.Request) {
		strId := path.Base(req.URL.Path)
		u, err := uuid.Parse(strId)
		if err != nil {
			http.Error(resp, fmt.Sprintf("parsing reader uuid: %s", err), 400)
			return
		}

		readersLk.Lock()
		ch, found := readers[u]
		if !found {
			ch = make(chan *RpcReader)
			readers[u] = ch
		}
		readersLk.Unlock()

		wr := &RpcReader{
			res:       make(chan readRes),
			next:      ch,
			beginOnce: &sync.Once{},
			closeOnce: &sync.Once{},
		}

		switch req.Method {
		case http.MethodHead:
			// leave body nil
		case http.MethodPost:
			wr.postBody = req.Body
		default:
			http.Error(resp, "unsupported method", http.StatusMethodNotAllowed)
		}

		tctx, cancel := context.WithTimeout(req.Context(), Timeout)
		defer cancel()

		select {
		case ch <- wr:
		case <-tctx.Done():
			close(ch)
			log.Errorf("context error in reader stream handler (1): %v", tctx.Err())
			resp.WriteHeader(500)
			return
		}

		select {
		case res, ok := <-wr.res:
			if !ok {
				if req.Method == http.MethodHead {
					resp.WriteHeader(http.StatusNoContent)
				} else {
					resp.WriteHeader(http.StatusOK)
				}
				return
			}
			// TODO should we check if we failed the Read, and if so
			//  return an HTTP 500? i.e. turn res into a chan error?

			switch res.rt {
			case resRedirect:
				http.Redirect(resp, req, res.meta, http.StatusFound)
			case resStart: // responding to HEAD, request POST with reader data
				resp.WriteHeader(http.StatusOK)
			case resError:
				resp.WriteHeader(500)
			default:
				log.Errorf("unknown res.rt")
				resp.WriteHeader(500)
			}

			return
		case <-req.Context().Done():
			log.Errorf("context error in reader stream handler (2): %v", req.Context().Err())

			closed := make(chan struct{})
			// start a draining goroutine
			go func() {
				for {
					select {
					case r, ok := <-wr.res:
						if !ok {
							return
						}
						log.Errorw("discarding read res", "type", r.rt, "meta", r.meta)
					case <-closed:
						return
					}
				}
			}()

			wr.beginOnce.Do(func() {})
			wr.closeOnce.Do(func() {
				close(wr.res)
			})
			close(closed)

			resp.WriteHeader(500)
			return
		}
	}

	// Server side reader decoder. runs on the rpc server side, invoked when decoding client request parameters. json(ReaderStream{}) -> io.Reader
	dec := jsonrpc.WithParamDecoder(new(io.Reader), func(ctx context.Context, b []byte) (reflect.Value, error) {
		var rs ReaderStream
		if err := json.Unmarshal(b, &rs); err != nil {
			return reflect.Value{}, xerrors.Errorf("unmarshaling reader id: %w", err)
		}

		switch rs.Type {
		case Null:
			n, err := strconv.ParseInt(rs.Info, 10, 64)
			if err != nil {
				return reflect.Value{}, xerrors.Errorf("parsing null byte count: %w", err)
			}

			return reflect.ValueOf(nullreader.NewNullReader(abi.UnpaddedPieceSize(n))), nil
		case HTTP:
			return reflect.ValueOf(&httpreader.HttpReader{URL: rs.Info}), nil
		}

		u, err := uuid.Parse(rs.Info)
		if err != nil {
			return reflect.Value{}, xerrors.Errorf("parsing reader UUDD: %w", err)
		}

		readersLk.Lock()
		ch, found := readers[u]
		if !found {
			ch = make(chan *RpcReader)
			readers[u] = ch
		}
		readersLk.Unlock()

		ctx, cancel := context.WithTimeout(ctx, Timeout)
		defer cancel()

		select {
		case wr, ok := <-ch:
			if !ok {
				return reflect.Value{}, xerrors.Errorf("handler timed out")
			}

			return reflect.ValueOf(wr), nil
		case <-ctx.Done():
			return reflect.Value{}, ctx.Err()
		}
	})

	return hnd, dec
}
