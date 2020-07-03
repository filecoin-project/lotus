package stats

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/specs-actors/actors/abi"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("stats")

func Collect(api api.FullNode) {
	time.Sleep(15 * time.Second)
	fmt.Println("collect stats")

	var database string = "testground"
	var height int64 = 0
	var headlag int = 3

	influxAddr := os.Getenv("INFLUXDB_URL")
	influxUser := ""
	influxPass := ""

	ctx := context.Background()

	influx, err := InfluxClient(influxAddr, influxUser, influxPass)
	if err != nil {
		log.Fatal(err)
	}

	h, err := GetLastRecordedHeight(influx, database)
	if err != nil {
		log.Info(err)
	}

	height = h

	tipsetsCh, err := GetTips(ctx, api, abi.ChainEpoch(height), headlag)
	if err != nil {
		log.Fatal(err)
	}

	wq := NewInfluxWriteQueue(ctx, influx)
	defer wq.Close()

	for tipset := range tipsetsCh {
		log.Infow("Collect stats", "height", tipset.Height())
		pl := NewPointList()
		height := tipset.Height()

		if err := RecordTipsetPoints(ctx, api, pl, tipset); err != nil {
			log.Warnw("Failed to record tipset", "height", height, "error", err)
			continue
		}

		if err := RecordTipsetMessagesPoints(ctx, api, pl, tipset); err != nil {
			log.Warnw("Failed to record messages", "height", height, "error", err)
			continue
		}

		if err := RecordTipsetStatePoints(ctx, api, pl, tipset); err != nil {
			log.Warnw("Failed to record state", "height", height, "error", err)
			continue
		}

		// Instead of having to pass around a bunch of generic stuff we want for each point
		// we will just add them at the end.

		tsTimestamp := time.Unix(int64(tipset.MinTimestamp()), int64(0))

		nb, err := InfluxNewBatch()
		if err != nil {
			log.Fatal(err)
		}

		for _, pt := range pl.Points() {
			pt.SetTime(tsTimestamp)

			nb.AddPoint(NewPointFrom(pt))
		}

		nb.SetDatabase(database)

		log.Infow("Adding points", "count", len(nb.Points()), "height", tipset.Height())

		wq.AddBatch(nb)
	}
}
