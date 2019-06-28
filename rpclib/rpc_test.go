package rpclib

import (
	"errors"
	"net/http/httptest"
	"strconv"
	"testing"
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
	rpcServer.Register(serverHandler)

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
		t.Fatal("wrong error")
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
		Add func() error
	}
	NewClient(testServ.URL, "SimpleServerHandler", &erronly)

	err = erronly.Add()
	if err == nil || err.Error() != "RPC client error: non 200 response code" {
		t.Error("wrong error")
	}

	var wrongtype struct {
		Add func(string) error
	}
	NewClient(testServ.URL, "SimpleServerHandler", &wrongtype)

	err = wrongtype.Add("not an int")
	if err == nil || err.Error() != "RPC client error: non 200 response code" {
		t.Error("wrong error")
	}
}
