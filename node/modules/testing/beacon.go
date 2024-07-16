package testing

import (
	"time"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/beacon"
)

func RandomBeacon() (beacon.Schedule, error) {
	return beacon.Schedule{
		{Start: 0,
			Beacon: beacon.NewMockBeacon(time.Duration(buildconstants.BlockDelaySecs) * time.Second),
		}}, nil
}
