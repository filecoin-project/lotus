package meta

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc/meta"
	"github.com/filecoin-project/lotus/x/conf"
)

const (
	MetaMinerID  = "META-MinerID"
	MetaWorkerID = "META-WorkerID"
)

func GetMinerID(ctx context.Context) (string, bool) {
	return meta.Get(ctx, MetaMinerID)
}

func SetMinerID(ctx context.Context) context.Context {
	return meta.Set(ctx, MetaMinerID, conf.X.ID)
}

func GetWorkerID(ctx context.Context) (string, bool) {
	return meta.Get(ctx, MetaWorkerID)
}

func SetWorkerID(ctx context.Context) context.Context {
	return meta.Set(ctx, MetaWorkerID, conf.X.ID)
}
