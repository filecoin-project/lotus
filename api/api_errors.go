package api

import (
	"github.com/filecoin-project/go-jsonrpc"
)

const (
	EOutOfGas = iota + jsonrpc.FirstUserCode
	EActorNotFound
)

type ErrOutOfGas struct{}

func (e ErrOutOfGas) Error() string {
	return "call ran out of gas"
}

type ErrActorNotFound struct{}

func (e ErrActorNotFound) Error() string {
	return "actor not found"
}

var RPCErrors = jsonrpc.NewErrors()

func init() {
	RPCErrors.Register(EOutOfGas, new(ErrOutOfGas))
	RPCErrors.Register(EActorNotFound, new(ErrActorNotFound))
}
