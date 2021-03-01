package main

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap/zapcore"
)

var benchCmd = &cli.Command{
	Name:  "bench",
	Usage: "Tools for diagnosing perf issues",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		benchStateCmd,
	},
}

type meanVar struct {
	n    float64
	mean float64
	m2   float64
}

func (v1 *meanVar) AddPoint(value float64) {
	// based on https://en.wikipedia.org/wiki/Algorithms_for_calculating_variance#Welford's_online_algorithm
	v1.n++
	delta := value - v1.mean
	v1.mean += delta / v1.n
	delta2 := value - v1.mean
	v1.m2 += delta * delta2
}

func (v1 *meanVar) Variance() float64 {
	return v1.m2 / (v1.n - 1)
}
func (v1 *meanVar) Mean() float64 {
	return v1.mean
}
func (v1 *meanVar) Stddev() float64 {
	return math.Sqrt(v1.Variance())
}

func (v1 *meanVar) MarshalLogObject(oe zapcore.ObjectEncoder) error {
	oe.AddFloat64("mean", v1.Mean())
	oe.AddFloat64("stddev", v1.Stddev())
	return nil
}

var benchStateCmd = &cli.Command{
	Name: "state-compute",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "tipset-range",
			Value: 1,
		},
		&cli.DurationFlag{
			Name:  "interval",
			Value: 15 * time.Second,
		},
		&cli.IntFlag{
			Name:  "concurrency",
			Value: 1,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		ring := make([]*types.TipSet, cctx.Int("tipset-range"))
		ringIdx := 0

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}
		ring[ringIdx] = head
		for i := range ring {
			// fill the ring
			ring[i] = head
		}
		ringIdx = 0

		concurrency := cctx.Int("concurrency")

		var mvLock sync.Mutex
		mv := &meanVar{}

		t := time.NewTicker(cctx.Duration("interval"))

		for {
			select {
			case <-t.C:
				wg := sync.WaitGroup{}
				wg.Add(concurrency)
				for i := 0; i < concurrency; i++ {
					go func() {
						n := rand.Intn(len(ring))
						ts := ring[n]
						start := time.Now()
						_, err := api.StateCompute(ctx, ts.Height(), nil, ts.Key())
						if err != nil {
							log.Errorf("state compute err: %v", err)
							return
						}
						took := time.Since(start)

						wg.Done()
						mvLock.Lock()
						mv.AddPoint(took.Seconds())
						log.Infow("computing state", "took", took, "ts", ts.Key(), "mv", mv)
						mvLock.Unlock()
					}()
				}
				wg.Wait()

				newHead, err := api.ChainHead(ctx)
				if err != nil {
					return err
				}
				if !newHead.Equals(head) {
					head = newHead
					ring[ringIdx] = head
					ringIdx = (ringIdx + 1) % len(ring)
				}

			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			}
		}

	},
}
