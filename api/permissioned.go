package api

import (
	"context"
	"fmt"
	"reflect"
)

type permKey int

var permCtxKey permKey

const (
	PermRead  = "read" // default
	PermWrite = "write"
	// todo: more perms once needed (network / sign / call/invoke / miner / etc)
)

var defaultPerms = []string{PermRead}

func WithPerm(ctx context.Context, perms []string) context.Context {
	return context.WithValue(ctx, permCtxKey, perms)
}

func Permissioned(a API) API {
	var out Struct

	rint := reflect.ValueOf(&out.Internal).Elem()
	ra := reflect.ValueOf(a)

	for f := 0; f < rint.NumField(); f++ {
		field := rint.Type().Field(f)
		requiredPerm := field.Tag.Get("perm")
		if requiredPerm == "" {
			requiredPerm = PermRead
		}

		fn := ra.MethodByName(field.Name)

		rint.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
			ctx := args[0].Interface().(context.Context)
			callerPerms, ok := ctx.Value(permCtxKey).([]string)
			if !ok {
				callerPerms = defaultPerms
			}

			for _, callerPerm := range callerPerms {
				if callerPerm == requiredPerm {
					return fn.Call(args)
				}
			}

			// TODO: return as error
			panic(fmt.Sprintf("unauthorized call to %s", field.Name))
		}))

	}

	return &out
}
