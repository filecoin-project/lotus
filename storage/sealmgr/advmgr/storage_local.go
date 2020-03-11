package advmgr

import (
	"github.com/filecoin-project/lotus/node/config"
)

type LocalStorage interface {
	GetStorage() (config.StorageConfig, error)
	SetStorage(func(*config.StorageConfig)) error
}
