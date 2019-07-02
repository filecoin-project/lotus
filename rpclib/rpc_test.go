package rpclib

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
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
	NewClient(testServ.URL, "SimpleServerHandler", &client)

	// Add(int) error

	if err := client.Add(2); err != nil {
		t.Fatal(err)
	}

	if serverHandler.n != 2 {
		t.Error("expected 2")
	}

	err := client.Add(-3546)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "test" {
		t.Fatal("wrong error", err)
	}

	// AddGet(int) int

	n := client.AddGet(3)
	if n != 5 {
		t.Error("wrong n")
	}

	if serverHandler.n != 5 {
		t.Error("expected 5")
	}

	// StringMatch

	o, err := client.StringMatch(TestType{S: "0"}, 0)
	if err != nil {
		t.Error(err)
	}
	if o.S != "0" || o.I != 0 {
		t.Error("wrong result")
	}

	_, err = client.StringMatch(TestType{S: "5"}, 5)
	if err == nil || err.Error() != ":(" {
		t.Error("wrong err")
	}

	o, err = client.StringMatch(TestType{S: "8", I: 8}, 8)
	if err != nil {
		t.Error(err)
	}
	if o.S != "8" || o.I != 8 {
		t.Error("wrong result")
	}

	// Invalid client handlers

	var noret struct {
		Add func(int)
	}
	NewClient(testServ.URL, "SimpleServerHandler", &noret)

	// this one should actually work
	noret.Add(4)
	if serverHandler.n != 9 {
		t.Error("expected 9")
	}

	var noparam struct {
		Add func()
	}
	NewClient(testServ.URL, "SimpleServerHandler", &noparam)

	// shouldn't panic
	noparam.Add()

	var erronly struct {
		AddGet func() (int, error)
	}
	NewClient(testServ.URL, "SimpleServerHandler", &erronly)

	_, err = erronly.AddGet()
	if err == nil || err.Error() != "RPC error (-32602): wrong param count" {
		t.Error("wrong error:", err)
	}

	var wrongtype struct {
		Add func(string) error
	}
	NewClient(testServ.URL, "SimpleServerHandler", &wrongtype)

	err = wrongtype.Add("not an int")
	if err == nil || err.Error() != "RPC error (-32700): json: cannot unmarshal string into Go value of type int" {
		t.Error("wrong error:", err)
	}

	var notfound struct {
		NotThere func(string) error
	}
	NewClient(testServ.URL, "SimpleServerHandler", &notfound)

	err = notfound.NotThere("hello?")
	if err == nil || err.Error() != "RPC error (-32601): method 'SimpleServerHandler.NotThere' not found" {
		t.Error("wrong error:", err)
	}
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
	NewClient(testServ.URL, "CtxHandler", &client)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Test(ctx)
	serverHandler.lk.Lock()

	if !serverHandler.cancelled {
		t.Error("expected cancellation on the server side")
	}

	serverHandler.cancelled = false

	serverHandler.lk.Unlock()

	var noCtxClient struct {
		Test func()
	}
	NewClient(testServ.URL, "CtxHandler", &noCtxClient)

	noCtxClient.Test()

	serverHandler.lk.Lock()

	if serverHandler.cancelled || serverHandler.i != 2 {
		t.Error("wrong serverHandler state")
	}

	serverHandler.lk.Unlock()
}
