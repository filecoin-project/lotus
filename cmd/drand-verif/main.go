package main

import (
	"encoding/json"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon/drand"
	"github.com/filecoin-project/lotus/chain/types"
)

var beaconJSON = `
  {
    "Round": 11233542,
    "Data": "lYkAm0e/2eNlEGsRo0L+UHXrRAWwK4Dok0Jphs+2AHeZjjtHmWiG4DXKHN5f2WKJ"
  }
`

func main() {
	var entry types.BeaconEntry

	err := json.Unmarshal([]byte(beaconJSON), &entry)
	if err != nil {
		panic(err)
	}
	const MAINNET_GENESIS_TIME = 1598306400
	shd, err := drand.BeaconScheduleFromDrandSchedule(build.DrandConfigSchedule(), MAINNET_GENESIS_TIME, nil)
	if err != nil {
		panic(err)
	}

	err = shd.BeaconForEpoch(4273425).VerifyEntry(entry, nil)
	if err != nil {
		panic(err)
	}

}
