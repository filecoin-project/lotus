package main

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var (
	MpoolAge         = stats.Float64("mpoolage", "Age of messages in the mempool", stats.UnitSeconds)
	MpoolSize        = stats.Int64("mpoolsize", "Number of messages in mempool", stats.UnitDimensionless)
	MpoolInboundRate = stats.Int64("inbound", "Counter for inbound messages", stats.UnitDimensionless)
)

var (
	LeTag, _ = tag.NewKey("le")
)

var (
	AgeView = &view.View{
		Name:        "mpool-age",
		Measure:     MpoolAge,
		TagKeys:     []tag.Key{LeTag},
		Aggregation: view.LastValue(),
	}
	SizeView = &view.View{
		Name:        "mpool-size",
		Measure:     MpoolSize,
		Aggregation: view.LastValue(),
	}
	InboundRate = &view.View{
		Name:        "msg-inbound",
		Measure:     MpoolInboundRate,
		Aggregation: view.Count(),
	}
)

type msgInfo struct {
	msg  *types.SignedMessage
	seen time.Time
}

var mpoolStatsCmd = &cli.Command{
	Name: "mpool-stats",
	Action: func(cctx *cli.Context) error {
		logging.SetLogLevel("rpc", "ERROR")

		if err := view.Register(AgeView); err != nil {
			return err
		}

		expo, err := prometheus.NewExporter(prometheus.Options{
			Namespace: "lotusmpool",
		})
		if err != nil {
			return err
		}

		http.Handle("/debug/metrics", expo)

		go func() {
			if err := http.ListenAndServe(":10555", nil); err != nil {
				panic(err)
			}
		}()

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
					stats.Record(ctx, MpoolInboundRate.M(1))
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
				stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(LeTag, "40")}, MpoolAge.M(st.Perc40.Seconds()))
				stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(LeTag, "50")}, MpoolAge.M(st.Perc50.Seconds()))
				stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(LeTag, "60")}, MpoolAge.M(st.Perc60.Seconds()))
				stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(LeTag, "70")}, MpoolAge.M(st.Perc70.Seconds()))
				stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(LeTag, "80")}, MpoolAge.M(st.Perc80.Seconds()))
				stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(LeTag, "90")}, MpoolAge.M(st.Perc90.Seconds()))
				stats.RecordWithTags(ctx, []tag.Mutator{tag.Upsert(LeTag, "95")}, MpoolAge.M(st.Perc95.Seconds()))

				stats.Record(ctx, MpoolSize.M(int64(len(tracker))))
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
