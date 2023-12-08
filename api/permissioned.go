package api

import (
	"github.com/filecoin-project/go-jsonrpc/auth"
)

const (
	// When changing these, update docs/API.md too

	PermRead  auth.Permission = "read" // default
	PermWrite auth.Permission = "write"
	PermSign  auth.Permission = "sign"  // Use wallet keys for signing
	PermAdmin auth.Permission = "admin" // Manage permissions
)

var AllPermissions = []auth.Permission{PermRead, PermWrite, PermSign, PermAdmin}
var DefaultPerms = []auth.Permission{PermRead}

func permissionedProxies(in, out interface{}) {
	outs := GetInternalStructs(out)
	for _, o := range outs {
		auth.PermissionedProxy(AllPermissions, DefaultPerms, in, o)
	}
}

func PermissionedStorMinerAPI(a StorageMiner) StorageMiner {
	var out StorageMinerStruct
	permissionedProxies(a, &out)
	return &out
}

func PermissionedFullAPI(a FullNode) FullNode {
	var out FullNodeStruct
	permissionedProxies(a, &out)
	return &out
}

func PermissionedWorkerAPI(a Worker) Worker {
	var out WorkerStruct
	permissionedProxies(a, &out)
	return &out
}

func PermissionedAPI[T, P any](a T) *P {
	var out P
	permissionedProxies(a, &out)
	return &out
}

func PermissionedWalletAPI(a Wallet) Wallet {
	var out WalletStruct
	permissionedProxies(a, &out)
	return &out
}
