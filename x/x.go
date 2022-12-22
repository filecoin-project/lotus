package x

import (
	"context"
	"errors"
	"path/filepath"

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
		srvName  string
		fnNodeID func(context.Context) context.Context
	)
	switch kind {
	case KindWorker:
		srvName = micro.WorkerName
		fnNodeID = meta.SetWorkerID
	case KindMiner:
		srvName = micro.MinerName
		fnNodeID = meta.SetMinerID
	default:
		return errors.New("node-kind invalid")
	}
	x.UseClientContext(func(ctx context.Context) context.Context {
		return fnNodeID(ctx)
	})
	return micro.Start(srvName, id, url)
}

func Stop() error {
	return micro.Stop()
}
