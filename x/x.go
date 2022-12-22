package x

import (
	"path/filepath"

	"github.com/filecoin-project/lotus/x/conf"
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

	srvName := micro.MinerName
	if kind == KindWorker {
		srvName = micro.WorkerName
	}
	return micro.Start(srvName, id, url)
}

func Stop() error {
	return micro.Stop()
}
