package jsonrpc

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	closer, err := NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &client)
	assert.NoError(t, err)
	defer closer()

	// Add(int) error

	assert.NoError(t, client.Add(2))
	assert.Equal(t, 2, serverHandler.n)

	err = client.Add(-3546)
	assert.EqualError(t, err, "test")

	// AddGet(int) int

	n := client.AddGet(3)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, serverHandler.n)

	// StringMatch

	o, err := client.StringMatch(TestType{S: "0"}, 0)
	assert.NoError(t, err)
	assert.Equal(t, "0", o.S)
	assert.Equal(t, 0, o.I)

	_, err = client.StringMatch(TestType{S: "5"}, 5)
	assert.EqualError(t, err, ":(")

	o, err = client.StringMatch(TestType{S: "8", I: 8}, 8)
	assert.NoError(t, err)
	assert.Equal(t, "8", o.S)
	assert.Equal(t, 8, o.I)

	// Invalid client handlers

	var noret struct {
		Add func(int)
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &noret)
	assert.NoError(t, err)

	// this one should actually work
	noret.Add(4)
	assert.Equal(t, 9, serverHandler.n)
	closer()

	var noparam struct {
		Add func()
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &noparam)
	assert.NoError(t, err)

	// shouldn't panic
	noparam.Add()
	closer()

	var erronly struct {
		AddGet func() (int, error)
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &erronly)
	assert.NoError(t, err)

	_, err = erronly.AddGet()
	if err == nil || err.Error() != "RPC error (-32602): wrong param count" {
		t.Error("wrong error:", err)
	}
	closer()

	var wrongtype struct {
		Add func(string) error
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &wrongtype)
	assert.NoError(t, err)

	err = wrongtype.Add("not an int")
	if err == nil || !strings.Contains(err.Error(), "RPC error (-32700):") || !strings.Contains(err.Error(), "json: cannot unmarshal string into Go value of type int") {
		t.Error("wrong error:", err)
	}
	closer()

	var notfound struct {
		NotThere func(string) error
	}
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "SimpleServerHandler", &notfound)
	assert.NoError(t, err)

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
	closer, err := NewClient("ws://"+testServ.Listener.Addr().String(), "CtxHandler", &client)
	assert.NoError(t, err)

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
	closer, err = NewClient("ws://"+testServ.Listener.Addr().String(), "CtxHandler", &noCtxClient)
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
