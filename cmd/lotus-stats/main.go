package main

import (
	"context"
	"flag"
	"os"

	"github.com/filecoin-project/lotus/tools/stats"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("stats")

const (
	influxAddrEnvVar = "INFLUX_ADDR"
	influxUserEnvVar = "INFLUX_USER"
	influxPassEnvVar = "INFLUX_PASS"
)

func main() {
	var repo string = "~/.lotus"
	var database string = "lotus"
	var reset bool = false
	var nosync bool = false
	var height int64 = 0
	var headlag int = 3

	flag.StringVar(&repo, "repo", repo, "lotus repo path")
	flag.StringVar(&database, "database", database, "influx database")
	flag.Int64Var(&height, "height", height, "block height to start syncing from (0 will resume)")
	flag.IntVar(&headlag, "head-lag", headlag, "number of head events to hold to protect against small reorgs")
	flag.BoolVar(&reset, "reset", reset, "truncate database before starting stats gathering")
	flag.BoolVar(&nosync, "nosync", nosync, "skip waiting for sync")

	flag.Parse()

	ctx := context.Background()

	influx, err := stats.InfluxClient(os.Getenv(influxAddrEnvVar), os.Getenv(influxUserEnvVar), os.Getenv(influxPassEnvVar))
	if err != nil {
		log.Fatal(err)
	}

	if reset {
		if err := stats.ResetDatabase(influx, database); err != nil {
			log.Fatal(err)
		}
	}

	if !reset && height == 0 {
		h, err := stats.GetLastRecordedHeight(influx, database)
		if err != nil {
			log.Info(err)
		}

		height = h
	}

	api, closer, err := stats.GetFullNodeAPI(ctx, repo)
	if err != nil {
		log.Fatal(err)
	}
	defer closer()

	if !nosync {
		if err := stats.WaitForSyncComplete(ctx, api); err != nil {
			log.Fatal(err)
		}
	}

	stats.Collect(ctx, api, influx, database, height, headlag)
}
