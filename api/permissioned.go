package api

import (
	"context"
	"reflect"

	"golang.org/x/xerrors"
)

type permKey int

var permCtxKey permKey

type Permission = string

const (
	// When changing these, update docs/API.md too

	PermRead  Permission = "read" // default
	PermWrite Permission = "write"
	PermSign  Permission = "sign"  // Use wallet keys for signing
	PermAdmin Permission = "admin" // Manage permissions
)

var AllPermissions = []Permission{PermRead, PermWrite, PermSign, PermAdmin}
var defaultPerms = []Permission{PermRead}

func WithPerm(ctx context.Context, perms []Permission) context.Context {
	return context.WithValue(ctx, permCtxKey, perms)
}

func PermissionedStorMinerAPI(a StorageMiner) StorageMiner {
	var out StorageMinerStruct
	permissionedAny(a, &out.Internal)
	permissionedAny(a, &out.CommonStruct.Internal)
	return &out
}

func PermissionedFullAPI(a FullNode) FullNode {
	var out FullNodeStruct
	permissionedAny(a, &out.Internal)
	permissionedAny(a, &out.CommonStruct.Internal)
	return &out
}

func HasPerm(ctx context.Context, perm Permission) bool {
	callerPerms, ok := ctx.Value(permCtxKey).([]Permission)
	if !ok {
		callerPerms = defaultPerms
	}

	for _, callerPerm := range callerPerms {
		if callerPerm == perm {
			return true
		}
	}
	return false
}

func permissionedAny(in interface{}, out interface{}) {
	rint := reflect.ValueOf(out).Elem()
	ra := reflect.ValueOf(in)

	for f := 0; f < rint.NumField(); f++ {
		field := rint.Type().Field(f)
		requiredPerm := Permission(field.Tag.Get("perm"))
		if requiredPerm == "" {
			panic("missing 'perm' tag on " + field.Name) // ok
		}

		// Validate perm tag
		ok := false
		for _, perm := range AllPermissions {
			if requiredPerm == perm {
				ok = true
				break
			}
		}
		if !ok {
			panic("unknown 'perm' tag on " + field.Name) // ok
		}

		fn := ra.MethodByName(field.Name)

		rint.Field(f).Set(reflect.MakeFunc(field.Type, func(args []reflect.Value) (results []reflect.Value) {
			ctx := args[0].Interface().(context.Context)
			if HasPerm(ctx, requiredPerm) {
				return fn.Call(args)
			}

			err := xerrors.Errorf("missing permission to invoke '%s' (need '%s')", field.Name, requiredPerm)
			rerr := reflect.ValueOf(&err).Elem()

			if field.Type.NumOut() == 2 {
				return []reflect.Value{
					reflect.Zero(field.Type.Out(0)),
					rerr,
				}
			} else {
				return []reflect.Value{rerr}
			}
		}))

	}
}
