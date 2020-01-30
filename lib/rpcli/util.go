package rpcli

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
)

func contextDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true

	default:
		return false
	}
}

type idGenerator interface {
	next() uint64
}

type simpleIDGen struct {
	id uint64
}

func (g *simpleIDGen) next() uint64 {
	return atomic.AddUint64(&g.id, 1)
}

// processFuncOut finds value and error Outs in function
func processFuncOut(funcType reflect.Type) (valOut int, errOut int, n int) {
	errOut = -1 // -1 if not found
	valOut = -1
	n = funcType.NumOut()

	switch n {
	case 0:
	case 1:
		if funcType.Out(0) == errorType {
			errOut = 0
		} else {
			valOut = 0
		}
	case 2:
		valOut = 0
		errOut = 1
		if funcType.Out(1) != errorType {
			panic("expected error as second return value")
		}
	default:
		errstr := fmt.Sprintf("too many return values: %s", funcType)
		panic(errstr)
	}

	return
}
