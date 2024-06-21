package v0api

import (
	"github.com/filecoin-project/lotus/api"
)

type Common = api.Common
type Net = api.Net

type CommonStruct = api.CommonStruct
type CommonStub = api.CommonStub
type NetStruct = api.NetStruct
type NetStub = api.NetStub

type StorageMiner = api.StorageMiner
type StorageMinerStruct = api.StorageMinerStruct

type Worker = api.Worker
type WorkerStruct = api.WorkerStruct

type Wallet = api.Wallet

func PermissionedStorMinerAPI(a StorageMiner) StorageMiner {
	return api.PermissionedStorMinerAPI(a)
}

func PermissionedWorkerAPI(a Worker) Worker {
	return api.PermissionedWorkerAPI(a)
}
