package rpcenc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"sync"

	"github.com/google/uuid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/specs-actors/actors/abi"
	sealing "github.com/filecoin-project/storage-fsm"
)

var log = logging.Logger("rpc")

type StreamType string
const (
	Null       StreamType = "null"
	PushStream StreamType = "push"
	// TODO: Data transfer handoff to workers?
)

type ReaderStream struct {
	Type StreamType
	Info string
}

func ReaderParamEncoder(addr string) jsonrpc.Option {
	return jsonrpc.WithParamEncoder(new(io.Reader), func(value reflect.Value) (reflect.Value, error) {
		r := value.Interface().(io.Reader)

		if r, ok := r.(*sealing.NullReader); ok {
			return reflect.ValueOf(ReaderStream{Type: Null, Info: fmt.Sprint(r.N)}), nil
		}

		reqID := uuid.New()
		u, _ := url.Parse(addr)
		u.Path = path.Join(u.Path, reqID.String())

		go func() {
			// TODO: figure out errors here

			resp, err := http.Post(u.String(), "application/octet-stream", r)
			if err != nil {
				log.Errorf("sending reader param: %+v", err)
				return
			}

			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				log.Errorf("sending reader param: non-200 status: ", resp.Status)
				return
			}

		}()

		return reflect.ValueOf(ReaderStream{Type: PushStream, Info: reqID.String()}), nil
	})
}

type waitReadCloser struct {
	io.ReadCloser
	wait chan struct{}
}

func (w *waitReadCloser) Read(p []byte) (int, error) {
	n, err := w.ReadCloser.Read(p)
	if err != nil {
		close(w.wait)
	}
	return n, err
}

func (w *waitReadCloser) Close() error {
	close(w.wait)
	return w.ReadCloser.Close()
}

func ReaderParamDecoder() (http.HandlerFunc, jsonrpc.ServerOption) {
	var readersLk sync.Mutex
	readers := map[uuid.UUID]chan *waitReadCloser{}

	hnd := func(resp http.ResponseWriter, req *http.Request) {
		strId := path.Base(req.URL.Path)
		u, err := uuid.Parse(strId)
		if err != nil {
			http.Error(resp, fmt.Sprintf("parsing reader uuid: %s", err), 400)
		}

		readersLk.Lock()
		ch, found := readers[u]
		if !found {
			ch = make(chan *waitReadCloser)
			readers[u] = ch
		}
		readersLk.Unlock()

		wr := &waitReadCloser{
			ReadCloser: req.Body,
			wait:       make(chan struct{}),
		}

		select {
		case ch <- wr:
		case <-req.Context().Done():
			log.Error("context error in reader stream handler (1): %v", req.Context().Err())
			resp.WriteHeader(500)
			return
		}

		select {
		case <-wr.wait:
		case <-req.Context().Done():
			log.Error("context error in reader stream handler (2): %v", req.Context().Err())
			resp.WriteHeader(500)
			return
		}

		resp.WriteHeader(200)
	}

	dec := jsonrpc.WithParamDecoder(new(io.Reader), func(ctx context.Context, b []byte) (reflect.Value, error) {
		var rs ReaderStream
		if err := json.Unmarshal(b, &rs); err != nil {
			return reflect.Value{}, xerrors.Errorf("unmarshaling reader id: %w", err)
		}

		if rs.Type == Null {
			n, err := strconv.ParseInt(rs.Info, 10, 64)
			if err != nil {
				return reflect.Value{}, xerrors.Errorf("parsing null byte count: %w", err)
			}

			return reflect.ValueOf(sealing.NewNullReader(abi.UnpaddedPieceSize(n))), nil
		}

		u, err := uuid.Parse(rs.Info)
		if err != nil {
			return reflect.Value{}, xerrors.Errorf("parsing reader UUDD: %w", err)
		}

		readersLk.Lock()
		ch, found := readers[u]
		if !found {
			ch = make(chan *waitReadCloser)
			readers[u] = ch
		}
		readersLk.Unlock()

		select {
		case wr := <-ch:
			return reflect.ValueOf(wr), nil
		case <-ctx.Done():
			return reflect.Value{}, ctx.Err()
		}
	})

	return hnd, dec
}
