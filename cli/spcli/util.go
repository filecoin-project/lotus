package spcli

import (
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
)

type ActorAddressGetter func(cctx *cli.Context) (address address.Address, err error)
