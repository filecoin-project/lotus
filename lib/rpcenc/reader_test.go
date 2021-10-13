package rpcenc

import (
	"context"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
)

type ReaderHandler struct {
	readApi func(ctx context.Context, r io.Reader) ([]byte, error)
}

func (h *ReaderHandler) ReadAllApi(ctx context.Context, r io.Reader, mustRedir bool) ([]byte, error) {
	if mustRedir {
		if err := r.(*RpcReader).MustRedirect(); err != nil {
			return nil, err
		}
	}
	return h.readApi(ctx, r)
}

func (h *ReaderHandler) ReadStartAndApi(ctx context.Context, r io.Reader, mustRedir bool) ([]byte, error) {
	if mustRedir {
		if err := r.(*RpcReader).MustRedirect(); err != nil {
			return nil, err
		}
	}

	n, err := r.Read([]byte{0})
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, xerrors.Errorf("not one")
	}

	return h.readApi(ctx, r)
}

func (h *ReaderHandler) CloseReader(ctx context.Context, r io.Reader) error {
	return r.(io.Closer).Close()
}

func (h *ReaderHandler) ReadAll(ctx context.Context, r io.Reader) ([]byte, error) {
	return ioutil.ReadAll(r)
}

func (h *ReaderHandler) ReadNullLen(ctx context.Context, r io.Reader) (int64, error) {
	return r.(*sealing.NullReader).N, nil
}

func (h *ReaderHandler) ReadUrl(ctx context.Context, u string) (string, error) {
	return u, nil
}

func TestReaderProxy(t *testing.T) {
	var client struct {
		ReadAll func(ctx context.Context, r io.Reader) ([]byte, error)
	}

	serverHandler := &ReaderHandler{}

	readerHandler, readerServerOpt := ReaderParamDecoder()
	rpcServer := jsonrpc.NewServer(readerServerOpt)
	rpcServer.Register("ReaderHandler", serverHandler)

	mux := mux.NewRouter()
	mux.Handle("/rpc/v0", rpcServer)
	mux.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)

	testServ := httptest.NewServer(mux)
	defer testServ.Close()

	re := ReaderParamEncoder("http://" + testServ.Listener.Addr().String() + "/rpc/streams/v0/push")
	closer, err := jsonrpc.NewMergeClient(context.Background(), "ws://"+testServ.Listener.Addr().String()+"/rpc/v0", "ReaderHandler", []interface{}{&client}, nil, re)
	require.NoError(t, err)

	defer closer()

	read, err := client.ReadAll(context.TODO(), strings.NewReader("pooooootato"))
	require.NoError(t, err)
	require.Equal(t, "pooooootato", string(read), "potatoes weren't equal")
}

func TestNullReaderProxy(t *testing.T) {
	var client struct {
		ReadAll     func(ctx context.Context, r io.Reader) ([]byte, error)
		ReadNullLen func(ctx context.Context, r io.Reader) (int64, error)
	}

	serverHandler := &ReaderHandler{}

	readerHandler, readerServerOpt := ReaderParamDecoder()
	rpcServer := jsonrpc.NewServer(readerServerOpt)
	rpcServer.Register("ReaderHandler", serverHandler)

	mux := mux.NewRouter()
	mux.Handle("/rpc/v0", rpcServer)
	mux.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)

	testServ := httptest.NewServer(mux)
	defer testServ.Close()

	re := ReaderParamEncoder("http://" + testServ.Listener.Addr().String() + "/rpc/streams/v0/push")
	closer, err := jsonrpc.NewMergeClient(context.Background(), "ws://"+testServ.Listener.Addr().String()+"/rpc/v0", "ReaderHandler", []interface{}{&client}, nil, re)
	require.NoError(t, err)

	defer closer()

	n, err := client.ReadNullLen(context.TODO(), sealing.NewNullReader(1016))
	require.NoError(t, err)
	require.Equal(t, int64(1016), n)
}

func TestReaderRedirect(t *testing.T) {
	var allClient struct {
		ReadAll func(ctx context.Context, r io.Reader) ([]byte, error)
	}

	{
		allServerHandler := &ReaderHandler{}
		readerHandler, readerServerOpt := ReaderParamDecoder()
		rpcServer := jsonrpc.NewServer(readerServerOpt)
		rpcServer.Register("ReaderHandler", allServerHandler)

		mux := mux.NewRouter()
		mux.Handle("/rpc/v0", rpcServer)
		mux.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)

		testServ := httptest.NewServer(mux)
		defer testServ.Close()

		re := ReaderParamEncoder("http://" + testServ.Listener.Addr().String() + "/rpc/streams/v0/push")
		closer, err := jsonrpc.NewMergeClient(context.Background(), "ws://"+testServ.Listener.Addr().String()+"/rpc/v0", "ReaderHandler", []interface{}{&allClient}, nil, re)
		require.NoError(t, err)

		defer closer()
	}

	var redirClient struct {
		ReadAllApi      func(ctx context.Context, r io.Reader, mustRedir bool) ([]byte, error)
		ReadStartAndApi func(ctx context.Context, r io.Reader, mustRedir bool) ([]byte, error)
		CloseReader     func(ctx context.Context, r io.Reader) error
	}

	{
		allServerHandler := &ReaderHandler{readApi: allClient.ReadAll}
		readerHandler, readerServerOpt := ReaderParamDecoder()
		rpcServer := jsonrpc.NewServer(readerServerOpt)
		rpcServer.Register("ReaderHandler", allServerHandler)

		mux := mux.NewRouter()
		mux.Handle("/rpc/v0", rpcServer)
		mux.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)

		testServ := httptest.NewServer(mux)
		defer testServ.Close()

		re := ReaderParamEncoder("http://" + testServ.Listener.Addr().String() + "/rpc/streams/v0/push")
		closer, err := jsonrpc.NewMergeClient(context.Background(), "ws://"+testServ.Listener.Addr().String()+"/rpc/v0", "ReaderHandler", []interface{}{&redirClient}, nil, re)
		require.NoError(t, err)

		defer closer()
	}

	// redirect
	read, err := redirClient.ReadAllApi(context.TODO(), strings.NewReader("rediracted pooooootato"), true)
	require.NoError(t, err)
	require.Equal(t, "rediracted pooooootato", string(read), "potatoes weren't equal")

	// proxy (because we started reading locally)
	read, err = redirClient.ReadStartAndApi(context.TODO(), strings.NewReader("rediracted pooooootato"), false)
	require.NoError(t, err)
	require.Equal(t, "ediracted pooooootato", string(read), "otatoes weren't equal")

	// check mustredir check; proxy (because we started reading locally)
	read, err = redirClient.ReadStartAndApi(context.TODO(), strings.NewReader("rediracted pooooootato"), true)
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrMustRedirect.Error())
	require.Empty(t, read)

	err = redirClient.CloseReader(context.TODO(), strings.NewReader("rediracted pooooootato"))
	require.NoError(t, err)
}
