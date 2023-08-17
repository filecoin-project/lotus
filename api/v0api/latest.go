package v0api

import (
	"github.com/filecoin-project/lotus/api"
)

type (
	Common    = api.Common
	Net       = api.Net
	CommonNet = api.CommonNet
)

type (
	CommonStruct    = api.CommonStruct
	CommonStub      = api.CommonStub
	NetStruct       = api.NetStruct
	NetStub         = api.NetStub
	CommonNetStruct = api.CommonNetStruct
	CommonNetStub   = api.CommonNetStub
)

type (
	StorageMiner       = api.StorageMiner
	StorageMinerStruct = api.StorageMinerStruct
)

type (
	Worker       = api.Worker
	WorkerStruct = api.WorkerStruct
)

type Wallet = api.Wallet

func PermissionedStorMinerAPI(a StorageMiner) StorageMiner {
	return api.PermissionedStorMinerAPI(a)
}

func PermissionedWorkerAPI(a Worker) Worker {
	return api.PermissionedWorkerAPI(a)
}
