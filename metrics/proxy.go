package metrics

import (
	"context"
	"reflect"

	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
)

func MetricedStorMinerAPI(a api.StorageMiner) api.StorageMiner {
	var out apistruct.StorageMinerStruct
	proxy(a, &out.Internal)
	proxy(a, &out.CommonStruct.Internal)
	return &out
}

func MetricedFullAPI(a api.FullNode) api.FullNode {
	var out apistruct.FullNodeStruct
	proxy(a, &out.Internal)
	proxy(a, &out.CommonStruct.Internal)
	return &out
}

func MetricedWorkerAPI(a api.WorkerAPI) api.WorkerAPI {
	var out apistruct.WorkerStruct
	proxy(a, &out.Internal)
	return &out
}

func MetricedWalletAPI(a api.WalletAPI) api.WalletAPI {
	var out apistruct.WalletStruct
	proxy(a, &out.Internal)
	return &out
}

func MetricedGatewayAPI(a api.GatewayAPI) api.GatewayAPI {
	var out apistruct.GatewayStruct
	proxy(a, &out.Internal)
	return &out
}

func proxy(in interface{}, out interface{}) {
	rint := reflect.ValueOf(out).Elem()
	ra := reflect.ValueOf(in)

	for f := 0; f < rint.NumField(); f++ {
		field := rint.Type().Field(f)
		fn := ra.MethodByName(field.Name)

		rint.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
			ctx := args[0].Interface().(context.Context)
			// upsert function name into context
			ctx, _ = tag.New(ctx, tag.Upsert(Endpoint, field.Name))
			stop := Timer(ctx, APIRequestDuration)
			defer stop()
			// pass tagged ctx back into function call
			args[0] = reflect.ValueOf(ctx)
			return fn.Call(args)
		}))

	}
}
