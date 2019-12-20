package actors

import (
	"reflect"
	"sort"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type update struct {
	start  uint64
	method interface{}
}

func withUpdates(updates ...update) interface{} {
	sort.Slice(updates, func(i, j int) bool { // so we iterate from newest below
		return updates[i].start > updates[j].start
	})

	// <script type="application/javascript">

	typ := reflect.TypeOf(updates[0].method)

	out := reflect.MakeFunc(typ, func(args []reflect.Value) (results []reflect.Value) {
		vmctx := args[1].Interface().(types.VMContext)

		for _, u := range updates {
			if u.start >= vmctx.BlockHeight() {
				return reflect.ValueOf(u.method).Call(args)
			}
		}

		return reflect.ValueOf(notFound(vmctx)).Call([]reflect.Value{args[1]})
	})

	return out.Interface()

	// </script>
}

func notFound(vmctx types.VMContext) func() ([]byte, ActorError) {
	return func() ([]byte, ActorError) {
		return nil, aerrors.Fatal("no code for method %d at height %d", vmctx.Message().Method, vmctx.BlockHeight())
	}
}
