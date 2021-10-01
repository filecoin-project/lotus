package proxy

import (
	"context"
	"reflect"

	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/metrics"
)

func MetricedStorMinerAPI(a api.StorageMiner) api.StorageMiner {
	var out api.StorageMinerStruct
	proxy(a, &out)
	return &out
}

func MetricedFullAPI(a api.FullNode) api.FullNode {
	var out api.FullNodeStruct
	proxy(a, &out)
	return &out
}

func MetricedWorkerAPI(a api.Worker) api.Worker {
	var out api.WorkerStruct
	proxy(a, &out)
	return &out
}

func MetricedWalletAPI(a api.Wallet) api.Wallet {
	var out api.WalletStruct
	proxy(a, &out)
	return &out
}

func MetricedGatewayAPI(a api.Gateway) api.Gateway {
	var out api.GatewayStruct
	proxy(a, &out)
	return &out
}

func proxy(in interface{}, outstr interface{}) {
	outs := api.GetInternalStructs(outstr)
	for _, out := range outs {
		rint := reflect.ValueOf(out).Elem()
		ra := reflect.ValueOf(in)

		for f := 0; f < rint.NumField(); f++ {
			field := rint.Type().Field(f)
			fn := ra.MethodByName(field.Name)

			rint.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
				ctx := args[0].Interface().(context.Context)
				// upsert function name into context
				ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, field.Name))
				stop := metrics.Timer(ctx, metrics.APIRequestDuration)
				defer stop()
				// pass tagged ctx back into function call
				args[0] = reflect.ValueOf(ctx)
				return fn.Call(args)
			}))
		}
	}
}
