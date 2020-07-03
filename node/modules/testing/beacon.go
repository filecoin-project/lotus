package testing

import (
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
)

func RandomBeacon() (beacon.RandomBeacon, error) {
	return beacon.NewMockBeacon(time.Duration(build.BlockDelaySecs) * time.Second), nil
}
