package proxy

import (
	"context"
	"fmt"
	"reflect"

	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/metrics"
)

func MetricedAPI[T, P any](a T) *P {
	var out P
	proxy(a, &out)
	return &out
}

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

func MetricedFullV2API(a v2api.FullNode) v2api.FullNode {
	var out v2api.FullNodeStruct
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

func MetricedGatewayV2API(a v2api.Gateway) v2api.Gateway {
	var out v2api.GatewayStruct
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

var log = logging.Logger("api_proxy")

func LoggingAPI[T, P any](a T) *P {
	var out P
	logProxy(a, &out)
	return &out
}

func logProxy(in interface{}, outstr interface{}) {
	outs := api.GetInternalStructs(outstr)
	for _, out := range outs {
		rint := reflect.ValueOf(out).Elem()
		ra := reflect.ValueOf(in)

		for f := 0; f < rint.NumField(); f++ {
			field := rint.Type().Field(f)
			fn := ra.MethodByName(field.Name)

			rint.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
				var wargs []interface{}
				wargs = append(wargs, "method", field.Name)

				for i := 1; i < len(args); i++ {
					wargs = append(wargs, fmt.Sprintf("arg%d", i), args[i].Interface())
				}

				res := fn.Call(args)
				for i, r := range res {
					wargs = append(wargs, fmt.Sprintf("ret%d", i), r.Interface())
				}

				log.Debugw("APICALL", wargs...)
				return res
			}))
		}
	}
}
