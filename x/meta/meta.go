package meta

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc/meta"
	"github.com/filecoin-project/lotus/x/conf"
)

const (
	MetaHostname = "META-Hostname"
	MetaMinerID  = "META-MinerID"
	MetaWorkerID = "META-WorkerID"
)

func SetHostname(ctx context.Context, name string) context.Context {
	return meta.Set(ctx, MetaHostname, name)
}

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
