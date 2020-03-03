package sealmgr

import (
	"github.com/filecoin-project/go-sectorbuilder"
)

type LocalWorker struct {
	sectorbuilder.Basic
}

var _ Worker = &LocalWorker{}
