package node

import (
	"github.com/filecoin-project/go-lotus/api"
)

type StorageMinerAPI struct {
	CommonAPI
}

var _ api.StorageMiner = &StorageMinerAPI{}
