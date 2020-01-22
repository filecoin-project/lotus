package jsonrpc

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type SimpleServerHandler struct {
	n int
}

type TestType struct {
	S string
	I int
}

type TestOut struct {
	TestType
	Ok bool
}

func (h *SimpleServerHandler) Add(in int) error {
	if in == -3546 {
		return errors.New("test")
	}

	h.n += in

	return nil
}

func (h *SimpleServerHandler) AddGet(in int) int {
	h.n += in
	return h.n
}

func (h *SimpleServerHandler) StringMatch(t TestType, i2 int64) (out TestOut, err error) {
	if strconv.FormatInt(i2, 10) == t.S {
		out.Ok = true
	}
	if i2 != int64(t.I) {
		return TestOut{}, errors.New(":(")
	}
	out.I = t.I
	out.S = t.S
	return
}

func TestRPC(t *testing.T) {
	// setup server

	serverHandler := &SimpleServerHandler{}

	rpcServer := NewServer()
	rpcServer.Register("SimpleServerHandler", serverHandler)

	// httptest stuff
	testServ := httptest.NewServer(rpcServer)
	defer testServ.Close()
	// setup client

	var client struct {
		Add         func(int) error
		AddGet      func(int) int
		StringMatch func(t TestType, i2 int64) (out TestOut, err error)
	}
	closer, err := NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &client, nil)
	require.NoError(t, err)
	defer closer()

	// Add(int) error

	require.NoError(t, client.Add(2))
	require.Equal(t, 2, serverHandler.n)

	err = client.Add(-3546)
	require.EqualError(t, err, "test")

	// AddGet(int) int

	n := client.AddGet(3)
	require.Equal(t, 5, n)
	require.Equal(t, 5, serverHandler.n)

	// StringMatch

	o, err := client.StringMatch(TestType{S: "0"}, 0)
	require.NoError(t, err)
	require.Equal(t, "0", o.S)
	require.Equal(t, 0, o.I)

	_, err = client.StringMatch(TestType{S: "5"}, 5)
	require.EqualError(t, err, ":(")

	o, err = client.StringMatch(TestType{S: "8", I: 8}, 8)
	require.NoError(t, err)
	require.Equal(t, "8", o.S)
	require.Equal(t, 8, o.I)

	// Invalid client handlers

	var noret struct {
		Add func(int)
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &noret, nil)
	require.NoError(t, err)

	// this one should actually work
	noret.Add(4)
	require.Equal(t, 9, serverHandler.n)
	closer()

	var noparam struct {
		Add func()
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &noparam, nil)
	require.NoError(t, err)

	// shouldn't panic
	noparam.Add()
	closer()

	var erronly struct {
		AddGet func() (int, error)
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &erronly, nil)
	require.NoError(t, err)

	_, err = erronly.AddGet()
	if err == nil || err.Error() != "RPC error (-32602): wrong param count" {
		t.Error("wrong error:", err)
	}
	closer()

	var wrongtype struct {
		Add func(string) error
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &wrongtype, nil)
	require.NoError(t, err)

	err = wrongtype.Add("not an int")
	if err == nil || !strings.Contains(err.Error(), "RPC error (-32700):") || !strings.Contains(err.Error(), "json: cannot unmarshal string into Go value of type int") {
		t.Error("wrong error:", err)
	}
	closer()

	var notfound struct {
		NotThere func(string) error
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &notfound, nil)
	require.NoError(t, err)

	err = notfound.NotThere("hello?")
	if err == nil || err.Error() != "RPC error (-32601): method 'SimpleServerHandler.NotThere' not found" {
		t.Error("wrong error:", err)
	}
	closer()
}

type CtxHandler struct {
	lk sync.Mutex

	cancelled bool
	i         int
}

func (h *CtxHandler) Test(ctx context.Context) {
	h.lk.Lock()
	defer h.lk.Unlock()
	timeout := time.After(300 * time.Millisecond)
	h.i++

	select {
	case <-timeout:
	case <-ctx.Done():
		h.cancelled = true
	}
}

func TestCtx(t *testing.T) {
	// setup server

	serverHandler := &CtxHandler{}

	rpcServer := NewServer()
	rpcServer.Register("CtxHandler", serverHandler)

	// httptest stuff
	testServ := httptest.NewServer(rpcServer)
	defer testServ.Close()

	// setup client

	var client struct {
		Test func(ctx context.Context)
	}
	closer, err := NewClient("ws://"+testServ.Listener.Addr().String(), "CtxHandler", &client, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Test(ctx)
	serverHandler.lk.Lock()

	if !serverHandler.cancelled {
		t.Error("expected cancellation on the server side")
	}

	serverHandler.cancelled = false

	serverHandler.lk.Unlock()
	closer()

	var noCtxClient struct {
		Test func()
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "CtxHandler", &noCtxClient, nil)
	if err != nil {
		t.Fatal(err)
	}

	noCtxClient.Test()

	serverHandler.lk.Lock()

	if serverHandler.cancelled || serverHandler.i != 2 {
		t.Error("wrong serverHandler state")
	}

	serverHandler.lk.Unlock()
	closer()
}

type UnUnmarshalable int

func (*UnUnmarshalable) UnmarshalJSON([]byte) error {
	return errors.New("nope")
}

type UnUnmarshalableHandler struct{}

func (*UnUnmarshalableHandler) GetUnUnmarshalableStuff() (UnUnmarshalable, error) {
	return UnUnmarshalable(5), nil
}

func TestUnmarshalableResult(t *testing.T) {
	var client struct {
		GetUnUnmarshalableStuff func() (UnUnmarshalable, error)
	}

	rpcServer := NewServer()
	rpcServer.Register("Handler", &UnUnmarshalableHandler{})

	testServ := httptest.NewServer(rpcServer)
	defer testServ.Close()

	closer, err := NewClient("ws://"+testServ.Listener.Addr().String(), "Handler", &client, nil)
	require.NoError(t, err)
	defer closer()

	_, err = client.GetUnUnmarshalableStuff()
	require.EqualError(t, err, "RPC client error: unmarshaling result: nope")
}

type ChanHandler struct {
	wait chan struct{}
}

func (h *ChanHandler) Sub(ctx context.Context, i int, eq int) (<-chan int, error) {
	out := make(chan int)

	go func() {
		defer close(out)
		var n int

		for {
			select {
			case <-ctx.Done():
				fmt.Println("ctxdone1")
				return
			case <-h.wait:
			}

			n += i

			if n == eq {
				fmt.Println("eq")
				return
			}

			select {
			case <-ctx.Done():
				fmt.Println("ctxdone2")
				return
			case out <- n:
			}
		}
	}()

	return out, nil
}

func TestChan(t *testing.T) {
	var client struct {
		Sub func(context.Context, int, int) (<-chan int, error)
	}

	serverHandler := &ChanHandler{
		wait: make(chan struct{}, 5),
	}

	rpcServer := NewServer()
	rpcServer.Register("ChanHandler", serverHandler)

	testServ := httptest.NewServer(rpcServer)
	defer testServ.Close()

	closer, err := NewClient("ws://"+testServ.Listener.Addr().String(), "ChanHandler", &client, nil)
	require.NoError(t, err)

	defer closer()

	serverHandler.wait <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())

	// sub

	sub, err := client.Sub(ctx, 2, -1)
	require.NoError(t, err)

	// recv one

	require.Equal(t, 2, <-sub)

	// recv many (order)

	serverHandler.wait <- struct{}{}
	serverHandler.wait <- struct{}{}
	serverHandler.wait <- struct{}{}

	require.Equal(t, 4, <-sub)
	require.Equal(t, 6, <-sub)
	require.Equal(t, 8, <-sub)

	// close (through ctx)
	cancel()

	_, ok := <-sub
	require.Equal(t, false, ok)

	// sub (again)

	serverHandler.wait <- struct{}{}

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	sub, err = client.Sub(ctx, 3, 6)
	require.NoError(t, err)

	require.Equal(t, 3, <-sub)

	// close (remote)
	serverHandler.wait <- struct{}{}
	_, ok = <-sub
	require.Equal(t, false, ok)
}

func TestControlChanDeadlock(t *testing.T) {
	for r := 0; r < 20; r++ {
		testControlChanDeadlock(t)
	}
}

func testControlChanDeadlock(t *testing.T) {
	var client struct {
		Sub func(context.Context, int, int) (<-chan int, error)
	}

	n := 5000

	serverHandler := &ChanHandler{
		wait: make(chan struct{}, n),
	}

	rpcServer := NewServer()
	rpcServer.Register("ChanHandler", serverHandler)

	testServ := httptest.NewServer(rpcServer)
	defer testServ.Close()

	closer, err := NewClient("ws://"+testServ.Listener.Addr().String(), "ChanHandler", &client, nil)
	require.NoError(t, err)

	defer closer()

	for i := 0; i < n; i++ {
		serverHandler.wait <- struct{}{}
	}

	ctx, _ := context.WithCancel(context.Background())

	sub, err := client.Sub(ctx, 1, -1)
	require.NoError(t, err)

	go func() {
		for i := 0; i < n; i++ {
			require.Equal(t, i+1, <-sub)
		}
	}()

	_, err = client.Sub(ctx, 2, -1)
	require.NoError(t, err)
}
