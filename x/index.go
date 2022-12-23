package x

import (
	"context"
	"hlm-ipfs/x/infras"

	"github.com/filecoin-project/lotus/storage/paths"
	apix "github.com/filecoin-project/lotus/x/api"
)

func NewSectorIndex() paths.SectorIndex {
	cli, _, err := apix.GetMinerApi(context.Background())
	infras.Throw(err)
	return cli
}
