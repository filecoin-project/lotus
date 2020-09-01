package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/urfave/cli/v2"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

type msgInfo struct {
	msg  *types.SignedMessage
	seen time.Time
}

var mpoolStatsCmd = &cli.Command{
	Name: "mpool-stats",
	Action: func(cctx *cli.Context) error {
		logging.SetLogLevel("rpc", "ERROR")

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		updates, err := api.MpoolSub(ctx)
		if err != nil {
			return err
		}

		tracker := make(map[cid.Cid]*msgInfo)
		tick := time.Tick(time.Second)
		for {
			select {
			case u := <-updates:
				switch u.Type {
				case lapi.MpoolAdd:
					tracker[u.Message.Cid()] = &msgInfo{
						msg:  u.Message,
						seen: time.Now(),
					}
				case lapi.MpoolRemove:
					mi, ok := tracker[u.Message.Cid()]
					if !ok {
						continue
					}
					fmt.Printf("%s was in the mempool for %s (feecap=%s, prem=%s)\n", u.Message.Cid(), time.Since(mi.seen), u.Message.Message.GasFeeCap, u.Message.Message.GasPremium)
					delete(tracker, u.Message.Cid())
				default:
					return fmt.Errorf("unrecognized mpool update state: %d", u.Type)
				}
			case <-tick:
				var ages []time.Duration
				if len(tracker) == 0 {
					continue
				}
				for _, v := range tracker {
					age := time.Since(v.seen)
					ages = append(ages, age)
				}

				st := ageStats(ages)
				fmt.Printf("%d messages in mempool for average of %s, (%s / %s / %s)\n", st.Count, st.Average, st.Perc50, st.Perc80, st.Perc95)
			}
		}
		return nil
	},
}

type ageStat struct {
	Average time.Duration
	Max     time.Duration
	Perc40  time.Duration
	Perc50  time.Duration
	Perc60  time.Duration
	Perc70  time.Duration
	Perc80  time.Duration
	Perc90  time.Duration
	Perc95  time.Duration
	Count   int
}

func ageStats(ages []time.Duration) *ageStat {
	sort.Slice(ages, func(i, j int) bool {
		return ages[i] < ages[j]
	})

	st := ageStat{
		Count: len(ages),
	}
	var sum time.Duration
	for _, a := range ages {
		sum += a
		if a > st.Max {
			st.Max = a
		}
	}
	st.Average = sum / time.Duration(len(ages))

	p40 := (4 * len(ages)) / 10
	p50 := len(ages) / 2
	p60 := (6 * len(ages)) / 10
	p70 := (7 * len(ages)) / 10
	p80 := (4 * len(ages)) / 5
	p90 := (9 * len(ages)) / 10
	p95 := (19 * len(ages)) / 20

	st.Perc40 = ages[p40]
	st.Perc50 = ages[p50]
	st.Perc60 = ages[p60]
	st.Perc70 = ages[p70]
	st.Perc80 = ages[p80]
	st.Perc90 = ages[p90]
	st.Perc95 = ages[p95]

	return &st
}
