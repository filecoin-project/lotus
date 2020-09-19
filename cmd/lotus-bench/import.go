package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/ipld/go-car"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/statediff"

	bdg "github.com/dgraph-io/badger/v2"
	"github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger2"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
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
		&cli.IntFlag{
			Name:  "batch-seal-verify-threads",
			Usage: "set the parallelism factor for batch seal verification",
			Value: runtime.NumCPU(),
		},
		&cli.StringFlag{
			Name:  "repodir",
			Usage: "set the repo directory for the lotus bench run (defaults to /tmp)",
		},
		&cli.StringFlag{
			Name:  "syscall-cache",
			Usage: "read and write syscall results from datastore",
		},
		&cli.BoolFlag{
			Name:  "export-traces",
			Usage: "should we export execution traces",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "no-import",
			Usage: "should we import the chain? if set to true chain has to be previously imported",
		},
		&cli.BoolFlag{
			Name: "only-gc",
		},
		&cli.BoolFlag{
			Name:  "global-profile",
			Value: true,
		},
		&cli.Int64Flag{
			Name: "start-at",
		},
	},
	Action: func(cctx *cli.Context) error {
		vm.BatchSealVerifyParallelism = cctx.Int("batch-seal-verify-threads")
		if !cctx.Args().Present() {
			fmt.Println("must pass car file of chain to benchmark importing")
			return nil
		}

		cfi, err := os.Open(cctx.Args().First())
		if err != nil {
			return err
		}
		defer cfi.Close() //nolint:errcheck // read only file

		go func() {
			http.ListenAndServe("localhost:6060", nil) //nolint:errcheck
		}()

		var tdir string
		if rdir := cctx.String("repodir"); rdir != "" {
			tdir = rdir
		} else {
			tmp, err := ioutil.TempDir("", "lotus-import-bench")
			if err != nil {
				return err
			}
			tdir = tmp
		}

		bdgOpt := badger.DefaultOptions
		bdgOpt.GcInterval = 0
		bdgOpt.Options = bdg.DefaultOptions("")
		bdgOpt.Options.SyncWrites = false
		bdgOpt.Options.Truncate = true
		bdgOpt.Options.DetectConflicts = false

		bds, err := badger.NewDatastore(tdir, &bdgOpt)
		if err != nil {
			return err
		}

		if cctx.Bool("only-gc") {
			log.Info("calling CollectGarbage on main ds")
			bds.CollectGarbage()
			log.Info("done calling CollectGarbage on main ds")
		}
		bs := blockstore.NewBlockstore(bds)
		cacheOpts := blockstore.DefaultCacheOpts()
		cacheOpts.HasBloomFilterSize = 0

		cbs, err := blockstore.CachedBlockstore(context.TODO(), bs, blockstore.DefaultCacheOpts())
		if err != nil {
			return err
		}
		bs = cbs
		ds := datastore.NewMapDatastore()

		var verifier ffiwrapper.Verifier = ffiwrapper.ProofVerifier
		if cctx.IsSet("syscall-cache") {
			scds, err := badger.NewDatastore(cctx.String("syscall-cache"), &bdgOpt)
			if err != nil {
				return xerrors.Errorf("opening syscall-cache datastore: %w", err)
			}

			if cctx.Bool("only-gc") {
				log.Info("calling CollectGarbage on syscall ds")
				scds.CollectGarbage()
				log.Info("done calling CollectGarbage on syscall ds")
			}
			verifier = &cachingVerifier{
				ds:      scds,
				backend: verifier,
			}
		}
		if cctx.Bool("only-gc") {
			return nil
		}

		cs := store.NewChainStore(bs, ds, vm.Syscalls(verifier))
		stm := stmgr.NewStateManager(cs)

		if cctx.Bool("global-profile") {
			prof, err := os.Create("import-bench.prof")
			if err != nil {
				return err
			}
			defer prof.Close() //nolint:errcheck

			if err := pprof.StartCPUProfile(prof); err != nil {
				return err
			}
		}

		var head *types.TipSet
		if !cctx.Bool("no-import") {
			head, err = cs.Import(cfi)
			if err != nil {
				return err
			}
		} else {
			cr, err := car.NewCarReader(cfi)
			if err != nil {
				return err
			}
			head, err = cs.LoadTipSet(types.NewTipSetKey(cr.Header.Roots...))
			if err != nil {
				return err
			}
		}

		gb, err := cs.GetTipsetByHeight(context.TODO(), 0, head, true)
		if err != nil {
			return err
		}

		err = cs.SetGenesis(gb.Blocks()[0])
		if err != nil {
			return err
		}

		startEpoch := abi.ChainEpoch(1)
		if cctx.IsSet("start-at") {
			startEpoch = abi.ChainEpoch(cctx.Int64("start-at"))
			start, err := cs.GetTipsetByHeight(context.TODO(), abi.ChainEpoch(cctx.Int64("start-at")), head, true)
			if err != nil {
				return err
			}

			err = cs.SetHead(start)
			if err != nil {
				return err
			}
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
		for ts.Height() > startEpoch {
			next, err := cs.LoadTipSet(ts.Parents())
			if err != nil {
				return err
			}

			tschain = append(tschain, next)
			ts = next
		}

		var enc *json.Encoder
		if cctx.Bool("export-traces") {
			ibj, err := os.Create("import-bench.json")
			if err != nil {
				return err
			}
			defer ibj.Close() //nolint:errcheck

			enc = json.NewEncoder(ibj)
		}

		for i := len(tschain) - 1; i >= 1; i-- {
			cur := tschain[i]
			start := time.Now()
			log.Infof("computing state (height: %d, ts=%s)", cur.Height(), cur.Cids())
			st, trace, err := stm.ExecutionTrace(context.TODO(), cur)
			if err != nil {
				return err
			}
			tse := &TipSetExec{
				TipSet:   cur.Key(),
				Trace:    trace,
				Duration: time.Since(start),
			}
			if enc != nil {
				stripCallers(tse.Trace)

				if err := enc.Encode(tse); err != nil {
					return xerrors.Errorf("failed to write out tipsetexec: %w", err)
				}
			}
			if tschain[i-1].ParentState() != st {
				stripCallers(tse.Trace)
				lastTrace := tse.Trace
				d, err := json.MarshalIndent(lastTrace, "", "  ")
				if err != nil {
					panic(err)
				}
				fmt.Println("TRACE")
				fmt.Println(string(d))
				fmt.Println(statediff.Diff(context.Background(), bs, tschain[i-1].ParentState(), st, statediff.ExpandActors))
				return xerrors.Errorf("tipset chain had state mismatch at height %d (%s != %s)", cur.Height(), cur.ParentState(), st)
			}
		}

		pprof.StopCPUProfile()

		return nil

	},
}

func walkExecutionTrace(et *types.ExecutionTrace) {
	for _, gc := range et.GasCharges {
		gc.Callers = nil
	}
	for _, sub := range et.Subcalls {
		walkExecutionTrace(&sub) //nolint:scopelint,gosec
	}
}

func stripCallers(trace []*api.InvocResult) {
	for _, t := range trace {
		walkExecutionTrace(&t.ExecutionTrace)
	}
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
		c, v := countGasCosts(&sub) //nolint
		cgas += c
		vgas += v
	}

	return cgas, vgas
}

type stats struct {
	timeTaken meanVar
	gasRatio  meanVar

	extraCovar *covar
}

type covar struct {
	meanX float64
	meanY float64
	c     float64
	n     float64
	m2x   float64
	m2y   float64
}

func (cov1 *covar) Covariance() float64 {
	return cov1.c / (cov1.n - 1)
}

func (cov1 *covar) VarianceX() float64 {
	return cov1.m2x / (cov1.n - 1)
}

func (cov1 *covar) StddevX() float64 {
	return math.Sqrt(cov1.VarianceX())
}

func (cov1 *covar) VarianceY() float64 {
	return cov1.m2y / (cov1.n - 1)
}

func (cov1 *covar) StddevY() float64 {
	return math.Sqrt(cov1.VarianceY())
}

func (cov1 *covar) AddPoint(x, y float64) {
	cov1.n++

	dx := x - cov1.meanX
	cov1.meanX += dx / cov1.n
	dx2 := x - cov1.meanX
	cov1.m2x += dx * dx2

	dy := y - cov1.meanY
	cov1.meanY += dy / cov1.n
	dy2 := y - cov1.meanY
	cov1.m2y += dy * dy2

	cov1.c += dx * dy
}

func (cov1 *covar) Combine(cov2 *covar) {
	if cov1.n == 0 {
		*cov1 = *cov2
		return
	}
	if cov2.n == 0 {
		return
	}

	if cov1.n == 1 {
		cpy := *cov2
		cpy.AddPoint(cov2.meanX, cov2.meanY)
		*cov1 = cpy
		return
	}
	if cov2.n == 1 {
		cov1.AddPoint(cov2.meanX, cov2.meanY)
	}

	out := covar{}
	out.n = cov1.n + cov2.n

	dx := cov1.meanX - cov2.meanX
	out.meanX = cov1.meanX - dx*cov2.n/out.n
	out.m2x = cov1.m2x + cov2.m2x + dx*dx*cov1.n*cov2.n/out.n

	dy := cov1.meanY - cov2.meanY
	out.meanY = cov1.meanY - dy*cov2.n/out.n
	out.m2y = cov1.m2y + cov2.m2y + dy*dy*cov1.n*cov2.n/out.n

	out.c = cov1.c + cov2.c + dx*dy*cov1.n*cov2.n/out.n
	*cov1 = out
}

func (cov1 *covar) A() float64 {
	return cov1.Covariance() / cov1.VarianceX()
}
func (cov1 *covar) B() float64 {
	return cov1.meanY - cov1.meanX*cov1.A()
}
func (cov1 *covar) Correl() float64 {
	return cov1.Covariance() / cov1.StddevX() / cov1.StddevY()
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

func (v1 *meanVar) Combine(v2 *meanVar) {
	if v1.n == 0 {
		*v1 = *v2
		return
	}
	if v2.n == 0 {
		return
	}
	if v1.n == 1 {
		cpy := *v2
		cpy.AddPoint(v1.mean)
		*v1 = cpy
		return
	}
	if v2.n == 1 {
		v1.AddPoint(v2.mean)
		return
	}

	newCount := v1.n + v2.n
	delta := v2.mean - v1.mean
	meanDelta := delta * v2.n / newCount
	m2 := v1.m2 + v2.m2 + delta*meanDelta*v1.n
	v1.n = newCount
	v1.mean += meanDelta
	v1.m2 = m2
}

func getExtras(ex interface{}) (*string, *float64) {
	if t, ok := ex.(string); ok {
		return &t, nil
	}
	if size, ok := ex.(float64); ok {
		return nil, &size
	}
	if exMap, ok := ex.(map[string]interface{}); ok {
		t, tok := exMap["type"].(string)
		size, sok := exMap["size"].(float64)
		if tok && sok {
			return &t, &size
		}
		if tok {
			return &t, nil
		}
		if sok {
			return nil, &size
		}
		return nil, nil
	}
	return nil, nil
}

func tallyGasCharges(charges map[string]*stats, et types.ExecutionTrace) {
	for i, gc := range et.GasCharges {
		name := gc.Name
		if name == "OnIpldGetEnd" {
			continue
		}
		tt := float64(gc.TimeTaken.Nanoseconds())
		if name == "OnVerifyPost" && tt > 2e9 {
			log.Warnf("Skipping abnormally long OnVerifyPost: %fs", tt/1e9)
			// discard initial very long OnVerifyPost
			continue
		}
		eType, eSize := getExtras(gc.Extra)

		if name == "OnIpldGet" {
			next := &types.GasTrace{}
			if i+1 < len(et.GasCharges) {
				next = et.GasCharges[i+1]
			}
			if next.Name != "OnIpldGetEnd" {
				log.Warn("OnIpldGet without OnIpldGetEnd")
			} else {
				_, size := getExtras(next.Extra)
				eSize = size
			}
		}
		if eType != nil {
			name += "-" + *eType
		}
		compGas := gc.ComputeGas
		if compGas == 0 {
			compGas = gc.VirtualComputeGas
		}
		if compGas == 0 {
			compGas = 1
		}
		s := charges[name]
		if s == nil {
			s = new(stats)
			charges[name] = s
		}

		if eSize != nil {
			if s.extraCovar == nil {
				s.extraCovar = &covar{}
			}
			s.extraCovar.AddPoint(*eSize, tt)
		}

		s.timeTaken.AddPoint(tt)

		ratio := tt / float64(compGas) * GasPerNs
		s.gasRatio.AddPoint(ratio)
	}
	for _, sub := range et.Subcalls {
		tallyGasCharges(charges, sub)
	}
}

var importAnalyzeCmd = &cli.Command{
	Name: "analyze",
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			fmt.Println("must pass bench file to analyze")
			return nil
		}

		go func() {
			http.ListenAndServe("localhost:6060", nil) //nolint:errcheck
		}()

		fi, err := os.Open(cctx.Args().First())
		if err != nil {
			return err
		}
		defer fi.Close() //nolint:errcheck

		const nWorkers = 16
		jsonIn := make(chan []byte, 2*nWorkers)
		type result struct {
			totalTime       time.Duration
			chargeStats     map[string]*stats
			expensiveInvocs []Invocation
		}

		results := make(chan result, nWorkers)

		for i := 0; i < nWorkers; i++ {
			go func() {
				chargeStats := make(map[string]*stats)
				var totalTime time.Duration
				const invocsKeep = 32
				var expensiveInvocs = make([]Invocation, 0, 8*invocsKeep)
				var leastExpensiveInvoc = time.Duration(0)

				for {
					b, ok := <-jsonIn
					if !ok {
						results <- result{
							totalTime:       totalTime,
							chargeStats:     chargeStats,
							expensiveInvocs: expensiveInvocs,
						}
						return
					}

					var tse TipSetExec
					err := json.Unmarshal(b, &tse)
					if err != nil {
						log.Warnf("error unmarshaling tipset: %+v", err)
						continue
					}

					totalTime += tse.Duration
					for _, inv := range tse.Trace {
						if inv.Duration > leastExpensiveInvoc {
							expensiveInvocs = append(expensiveInvocs, Invocation{
								TipSet: tse.TipSet,
								Invoc:  inv,
							})
						}

						tallyGasCharges(chargeStats, inv.ExecutionTrace)
					}
					if len(expensiveInvocs) > 4*invocsKeep {
						sort.Slice(expensiveInvocs, func(i, j int) bool {
							return expensiveInvocs[i].Invoc.Duration > expensiveInvocs[j].Invoc.Duration
						})
						leastExpensiveInvoc = expensiveInvocs[len(expensiveInvocs)-1].Invoc.Duration
						n := 30
						if len(expensiveInvocs) < n {
							n = len(expensiveInvocs)
						}
						expensiveInvocs = expensiveInvocs[:n]
					}
				}
			}()
		}

		var totalTipsets int64
		reader := bufio.NewReader(fi)
		for {
			b, err := reader.ReadBytes('\n')
			if err != nil && err != io.EOF {
				if e, ok := err.(*json.SyntaxError); ok {
					log.Warnf("syntax error at byte offset %d", e.Offset)
				}
				return err
			}
			totalTipsets++
			jsonIn <- b
			fmt.Fprintf(os.Stderr, "\rProcessed %d tipsets", totalTipsets)
			if err == io.EOF {
				break
			}
		}
		close(jsonIn)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Collecting results\n")

		var invocs []Invocation
		var totalTime time.Duration
		var keys []string
		var charges = make(map[string]*stats)
		for i := 0; i < nWorkers; i++ {
			fmt.Fprintf(os.Stderr, "\rProcessing results from worker %d/%d", i+1, nWorkers)
			res := <-results
			invocs = append(invocs, res.expensiveInvocs...)
			for k, v := range res.chargeStats {
				s := charges[k]
				if s == nil {
					s = new(stats)
					charges[k] = s
				}
				s.timeTaken.Combine(&v.timeTaken)
				s.gasRatio.Combine(&v.gasRatio)

				if v.extraCovar != nil {
					if s.extraCovar == nil {
						s.extraCovar = &covar{}
					}
					s.extraCovar.Combine(v.extraCovar)
				}
			}
			totalTime += res.totalTime
		}

		fmt.Fprintf(os.Stderr, "\nCollecting gas keys\n")
		for k := range charges {
			keys = append(keys, k)
		}

		fmt.Println("Gas Price Deltas")
		sort.Strings(keys)
		for _, k := range keys {
			s := charges[k]
			fmt.Printf("%s: incr by %.4f~%.4f; tt %.4f~%.4f\n", k, s.gasRatio.Mean(), s.gasRatio.Stddev(),
				s.timeTaken.Mean(), s.timeTaken.Stddev())
			if s.extraCovar != nil {
				fmt.Printf("\t correll: %.2f, tt = %.2f * extra + %.2f\n", s.extraCovar.Correl(),
					s.extraCovar.A(), s.extraCovar.B())
				fmt.Printf("\t covar: %.2f, extra: %.2f~%.2f, tt2: %.2f~%.2f, count %.0f\n",
					s.extraCovar.Covariance(), s.extraCovar.meanX, s.extraCovar.StddevX(),
					s.extraCovar.meanY, s.extraCovar.StddevY(), s.extraCovar.n)
			}
		}

		sort.Slice(invocs, func(i, j int) bool {
			return invocs[i].Invoc.Duration > invocs[j].Invoc.Duration
		})

		fmt.Println("Total time: ", totalTime)
		fmt.Println("Average time per epoch: ", totalTime/time.Duration(totalTipsets))
		if actorExec, ok := charges["OnActorExec"]; ok {
			timeInActors := actorExec.timeTaken.Mean() * actorExec.timeTaken.n
			fmt.Printf("Avarage time per epoch in actors: %s (%.1f%%)\n", time.Duration(timeInActors)/time.Duration(totalTipsets), timeInActors/float64(totalTime)*100)
		}
		if actorExecDone, ok := charges["OnMethodInvocationDone"]; ok {
			timeInActors := actorExecDone.timeTaken.Mean() * actorExecDone.timeTaken.n
			fmt.Printf("Avarage time per epoch in OnActorExecDone %s (%.1f%%)\n", time.Duration(timeInActors)/time.Duration(totalTipsets), timeInActors/float64(totalTime)*100)
		}

		n := 30
		if len(invocs) < n {
			n = len(invocs)
		}
		fmt.Printf("Top %d most expensive calls:\n", n)
		for i := 0; i < n; i++ {
			inv := invocs[i].Invoc
			fmt.Printf("%s: %s %s %d %s\n", inv.Duration, inv.Msg.From, inv.Msg.To, inv.Msg.Method, invocs[i].TipSet)
		}
		return nil
	},
}
