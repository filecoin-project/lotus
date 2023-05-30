package api

import (
	"errors"
	"reflect"

	"github.com/filecoin-project/go-jsonrpc"
)

const (
	EOutOfGas = iota + jsonrpc.FirstUserCode
	EActorNotFound
)

type ErrOutOfGas struct{}

func (e *ErrOutOfGas) Error() string {
	return "call ran out of gas"
}

type ErrActorNotFound struct{}

func (e *ErrActorNotFound) Error() string {
	return "actor not found"
}

var RPCErrors = jsonrpc.NewErrors()

func ErrorIsIn(err error, errorTypes []error) bool {
	for _, etype := range errorTypes {
		tmp := reflect.New(reflect.PointerTo(reflect.ValueOf(etype).Elem().Type())).Interface()
		if errors.As(err, tmp) {
			return true
		}
	}
	return false
}

func init() {
	RPCErrors.Register(EOutOfGas, new(*ErrOutOfGas))
	RPCErrors.Register(EActorNotFound, new(*ErrActorNotFound))
}
