package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"runtime/pprof"
	"sort"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger2"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/urfave/cli/v2"
)

type TipSetExec struct {
	TipSet   types.TipSetKey
	Trace    []*api.InvocResult
	Duration time.Duration
}

var importBenchCmd = &cli.Command{
	Name:  "import",
	Usage: "benchmark chain import and validation",
	Subcommands: []*cli.Command{
		importAnalyzeCmd,
	},
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "height",
			Usage: "halt validation after given height",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			fmt.Println("must pass car file of chain to benchmark importing")
			return nil
		}

		cfi, err := os.Open(cctx.Args().First())
		if err != nil {
			return err
		}
		defer cfi.Close() //nolint:errcheck // read only file

		tdir, err := ioutil.TempDir("", "lotus-import-bench")
		if err != nil {
			return err
		}

		bds, err := badger.NewDatastore(tdir, nil)
		if err != nil {
			return err
		}
		bs := blockstore.NewBlockstore(bds)
		cbs, err := blockstore.CachedBlockstore(context.TODO(), bs, blockstore.DefaultCacheOpts())
		if err != nil {
			return err
		}
		bs = cbs
		ds := datastore.NewMapDatastore()
		cs := store.NewChainStore(bs, ds, vm.Syscalls(ffiwrapper.ProofVerifier))
		stm := stmgr.NewStateManager(cs)

		prof, err := os.Create("import-bench.prof")
		if err != nil {
			return err
		}
		defer prof.Close() //nolint:errcheck

		if err := pprof.StartCPUProfile(prof); err != nil {
			return err
		}

		head, err := cs.Import(cfi)
		if err != nil {
			return err
		}

		if h := cctx.Int64("height"); h != 0 {
			tsh, err := cs.GetTipsetByHeight(context.TODO(), abi.ChainEpoch(h), head, true)
			if err != nil {
				return err
			}
			head = tsh
		}

		ts := head
		tschain := []*types.TipSet{ts}
		for ts.Height() != 0 {
			next, err := cs.LoadTipSet(ts.Parents())
			if err != nil {
				return err
			}

			tschain = append(tschain, next)
			ts = next
		}

		out := make([]TipSetExec, 0, len(tschain))

		lastState := tschain[len(tschain)-1].ParentState()
		for i := len(tschain) - 2; i >= 0; i-- {
			cur := tschain[i]
			log.Infof("computing state (height: %d, ts=%s)", cur.Height(), cur.Cids())
			if cur.ParentState() != lastState {
				lastTrace := out[len(out)-1].Trace
				d, err := json.MarshalIndent(lastTrace, "", "  ")
				if err != nil {
					panic(err)
				}
				fmt.Println("TRACE")
				fmt.Println(string(d))
				return xerrors.Errorf("tipset chain had state mismatch at height %d (%s != %s)", cur.Height(), cur.ParentState(), lastState)
			}
			start := time.Now()
			st, trace, err := stm.ExecutionTrace(context.TODO(), cur)
			if err != nil {
				return err
			}
			out = append(out, TipSetExec{
				TipSet:   cur.Key(),
				Trace:    trace,
				Duration: time.Since(start),
			})
			lastState = st
		}

		pprof.StopCPUProfile()

		ibj, err := os.Create("import-bench.json")
		if err != nil {
			return err
		}
		defer ibj.Close() //nolint:errcheck

		if err := json.NewEncoder(ibj).Encode(out); err != nil {
			return err
		}

		return nil

	},
}

type Invocation struct {
	TipSet types.TipSetKey
	Invoc  *api.InvocResult
}

const GasPerNs = 10

func countGasCosts(et *types.ExecutionTrace) (int64, int64) {
	var cgas, vgas int64

	for _, gc := range et.GasCharges {
		cgas += gc.ComputeGas
		vgas += gc.VirtualComputeGas
	}

	for _, sub := range et.Subcalls {
		c, v := countGasCosts(&sub)
		cgas += c
		vgas += v
	}

	return cgas, vgas
}

func compStats(vals []float64) (float64, float64) {
	var sum float64

	for _, v := range vals {
		sum += v
	}

	av := sum / float64(len(vals))

	var varsum float64
	for _, v := range vals {
		delta := av - v
		varsum += delta * delta
	}

	return av, math.Sqrt(varsum / float64(len(vals)))
}

var importAnalyzeCmd = &cli.Command{
	Name: "analyze",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			fmt.Println("must pass bench file to analyze")
			return nil
		}

		fi, err := os.Open(cctx.Args().First())
		if err != nil {
			return err
		}

		var results []TipSetExec
		if err := json.NewDecoder(fi).Decode(&results); err != nil {
			return err
		}

		chargeDeltas := make(map[string][]float64)

		var invocs []Invocation
		var totalTime time.Duration
		for i, r := range results {
			_ = i
			totalTime += r.Duration

			for _, inv := range r.Trace {
				invocs = append(invocs, Invocation{
					TipSet: r.TipSet,
					Invoc:  inv,
				})

				cgas, vgas := countGasCosts(&inv.ExecutionTrace)
				fmt.Printf("Invocation: %d %s: %s %d -> %0.2f\n", inv.Msg.Method, inv.Msg.To, inv.Duration, cgas+vgas, float64(GasPerNs*inv.Duration.Nanoseconds())/float64(cgas+vgas))

				for _, gc := range inv.ExecutionTrace.GasCharges {

					compGas := gc.ComputeGas + gc.VirtualComputeGas
					ratio := float64(compGas) / float64(gc.TimeTaken.Nanoseconds())

					chargeDeltas[gc.Name] = append(chargeDeltas[gc.Name], 1/(ratio/GasPerNs))
					//fmt.Printf("%s: %d, %s: %0.2f\n", gc.Name, compGas, gc.TimeTaken, 1/(ratio/GasPerNs))
				}
			}
		}

		var keys []string
		for k := range chargeDeltas {
			keys = append(keys, k)
		}

		fmt.Println("Gas Price Deltas")
		sort.Strings(keys)
		for _, k := range keys {
			vals := chargeDeltas[k]
			av, stdev := compStats(vals)

			fmt.Printf("%s: incr by %f (%f)\n", k, av, stdev)
		}

		sort.Slice(invocs, func(i, j int) bool {
			return invocs[i].Invoc.Duration > invocs[j].Invoc.Duration
		})

		fmt.Println("Total time: ", totalTime)
		fmt.Println("Average time per epoch: ", totalTime/time.Duration(len(results)))

		n := 30
		fmt.Printf("Top %d most expensive calls:\n", n)
		for i := 0; i < n; i++ {
			inv := invocs[i].Invoc
			fmt.Printf("%s: %s %s %d %s\n", inv.Duration, inv.Msg.From, inv.Msg.To, inv.Msg.Method, invocs[i].TipSet)
		}
		return nil
	},
}
