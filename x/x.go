package x

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"hlm-ipfs/x/logger"

	"github.com/filecoin-project/go-jsonrpc/x"
	"github.com/filecoin-project/lotus/x/conf"
	"github.com/filecoin-project/lotus/x/meta"
	"github.com/filecoin-project/lotus/x/micro"
	"github.com/filecoin-project/lotus/x/store"
)

type Kind int

const (
	KindWorker Kind = iota
	KindMiner
)

func Init(repo string) (err error) {
	repo = filepath.Join(repo, "x")
	if err = conf.Init(repo); err != nil {
		return err
	}
	return store.Init(repo)
}

// Start miner-id or worker-id
func Start(kind Kind, id, url string) error {
	conf.X.ID = id
	conf.X.Url = url

	var (
		srvName     string
		fnSetNodeID func(context.Context) context.Context
		fnGetNodeID func(ctx context.Context) (string, bool)
	)
	switch kind {
	case KindWorker:
		srvName = micro.WorkerName
		fnSetNodeID = meta.SetWorkerID
		fnGetNodeID = meta.GetMinerID
	case KindMiner:
		srvName = micro.MinerName
		fnSetNodeID = meta.SetMinerID
		fnGetNodeID = meta.GetWorkerID
	default:
		return errors.New("node-kind invalid")
	}
	x.UseClientContext(func(ctx context.Context) context.Context {
		return fnSetNodeID(ctx)
	})
	x.UseServerTracing(func(ctx context.Context, method string) func() {
		now := time.Now()
		id, _ := fnGetNodeID(ctx)
		return func() {
			logger.Infow("json-rpc invoke", "id", id, "method", method, "elapsed", time.Since(now).Milliseconds())
		}
	})
	return micro.Start(srvName, id, url)
}

func Stop() error {
	return micro.Stop()
}
