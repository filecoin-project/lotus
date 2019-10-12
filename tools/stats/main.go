package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/filecoin-project/go-lotus/chain/address"
)

const (
	INFLUX_ADDR = "INFLUX_ADDR"
	INFLUX_USER = "INFLUX_USER"
	INFLUX_PASS = "INFLUX_PASS"
)

func main() {
	var repo string = "~/.lotus"
	var database string = "lotus"
	var reset bool = false
	var height int64 = 0

	flag.StringVar(&repo, "repo", repo, "lotus repo path")
	flag.StringVar(&database, "database", database, "influx database")
	flag.Int64Var(&height, "height", height, "block height to start syncing from (0 will resume)")
	flag.BoolVar(&reset, "reset", reset, "truncate database before starting stats gathering")

	flag.Parse()

	influxAddr := os.Getenv(INFLUX_ADDR)
	influxUser := os.Getenv(INFLUX_USER)
	influxPass := os.Getenv(INFLUX_PASS)

	ctx := context.Background()

	influx, err := InfluxClient(influxAddr, influxUser, influxPass)
	if err != nil {
		log.Fatal(err)
	}

	if reset {
		if err := ResetDatabase(influx, database); err != nil {
			log.Fatal(err)
		}
	}

	if !reset && height == 0 {
		h, err := GetLastRecordedHeight(influx, database)
		if err != nil {
			log.Print(err)
		}

		height = h
	}

	api, closer, err := GetFullNodeAPI(repo)
	if err != nil {
		log.Fatal(err)
	}
	defer closer()

	tipsetsCh, err := GetTips(ctx, api, uint64(height))
	if err != nil {
		log.Fatal(err)
	}

	for tipset := range tipsetsCh {
		pl := NewPointList()

		RecordTipsetPoints(pl, tipset)

		for _, blockheader := range tipset.Blocks() {
			RecordBlockHeaderPoints(pl, blockheader)

			blkmsgs, err := api.ChainGetBlockMessages(ctx, blockheader.Cid())
			if err != nil {
				log.Fatal(err)
			}

			msgs := blkmsgs.BlsMessages
			for _, msg := range blkmsgs.SecpkMessages {
				msgs = append(msgs, msg.VMMessage())
			}

			RecordBlockMessagesPoints(pl, msgs)
		}

		power, err := api.StateMinerPower(ctx, address.Address{}, tipset)
		if err != nil {
			log.Fatal(err)
		}

		p := NewPoint("chain.power", power.TotalPower.Int64())
		pl.AddPoint(p)

		// Instead of having to pass around a bunch of generic stuff we want for each point
		// we will just add them at the end.

		tsHeight := fmt.Sprintf("%d", tipset.Height())
		tsTimestamp := time.Unix(int64(tipset.MinTimestamp()), int64(0))

		nb, err := InfluxNewBatch()
		if err != nil {
			log.Fatal(err)
		}

		for _, pt := range pl.Points() {
			pt.AddTag("height", tsHeight)
			pt.SetTime(tsTimestamp)

			nb.AddPoint(NewPointFrom(pt))
		}

		nb.SetDatabase(database)

		log.Printf("Writing %d points for height %d", len(nb.Points()), tipset.Height())

		if err := influx.Write(nb); err != nil {
			log.Fatal(err)
		}
	}
}
