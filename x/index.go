package x

import (
	"context"

	"github.com/filecoin-project/lotus/storage/paths"
	apix "github.com/filecoin-project/lotus/x/api"
)

func NewSectorIndex() (paths.SectorIndex, error) {
	cli, _, err := apix.GetMinerApi(context.Background())
	return cli, err
}
