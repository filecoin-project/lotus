package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"

	ocprom "contrib.go.opencensus.io/exporter/prometheus"
	bdg "github.com/dgraph-io/badger/v2"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger2"
	measure "github.com/ipfs/go-ds-measure"
	metricsprometheus "github.com/ipfs/go-metrics-prometheus"
	"github.com/ipld/go-car"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	badgerbs "github.com/filecoin-project/lotus/blockstore/badger"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/verifier"
	verifierffi "github.com/filecoin-project/lotus/chain/verifier/ffi"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/delegated"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/lotus/node/repo"
)

type TipSetExec struct {
	TipSet   types.TipSetKey
	Trace    []*api.InvocResult
	Duration time.Duration
}

var importBenchCmd = &cli.Command{
	Name:  "import",
	Usage: "Benchmark chain import and validation",
	Subcommands: []*cli.Command{
		importAnalyzeCmd,
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "start-tipset",
			Usage: "start validation at the given tipset key; in format cid1,cid2,cid3...",
		},
		&cli.StringFlag{
			Name:  "end-tipset",
			Usage: "halt validation at the given tipset key; in format cid1,cid2,cid3...",
		},
		&cli.StringFlag{
			Name:  "genesis-tipset",
			Usage: "genesis tipset key; in format cid1,cid2,cid3...",
		},
		&cli.Int64Flag{
			Name:  "start-height",
			Usage: "start validation at given height; beware that chain traversal by height is very slow",
		},
		&cli.Int64Flag{
			Name:  "end-height",
			Usage: "halt validation after given height; beware that chain traversal by height is very slow",
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
			Name:  "global-profile",
			Value: true,
		},
		&cli.BoolFlag{
			Name: "only-import",
		},
		&cli.BoolFlag{
			Name: "use-native-badger",
		},
		&cli.StringFlag{
			Name: "car",
			Usage: "path to CAR file; required for import; on validation, either " +
				"a CAR path or the --head flag are required",
		},
		&cli.StringFlag{
			Name: "head",
			Usage: "tipset key of the head, useful when benchmarking validation " +
				"on an existing chain store, where a CAR is not available; " +
				"if both --car and --head are provided, --head takes precedence " +
				"over the CAR root; the format is cid1,cid2,cid3...",
		},
	},
	Action: func(cctx *cli.Context) error {
		metricsprometheus.Inject() //nolint:errcheck
		vm.BatchSealVerifyParallelism = cctx.Int("batch-seal-verify-threads")

		go func() {
			// Prometheus globals are exposed as interfaces, but the prometheus
			// OpenCensus exporter expects a concrete *Registry. The concrete type of
			// the globals are actually *Registry, so we downcast them, staying
			// defensive in case things change under the hood.
			registry, ok := prometheus.DefaultRegisterer.(*prometheus.Registry)
			if !ok {
				log.Warnf("failed to export default prometheus registry; some metrics will be unavailable; unexpected type: %T", prometheus.DefaultRegisterer)
				return
			}
			exporter, err := ocprom.NewExporter(ocprom.Options{
				Registry:  registry,
				Namespace: "lotus",
			})
			if err != nil {
				log.Fatalf("could not create the prometheus stats exporter: %v", err)
			}

			http.Handle("/debug/metrics", exporter)

			_ = http.ListenAndServe("localhost:6060", nil)
		}()

		var tdir string
		if rdir := cctx.String("repodir"); rdir != "" {
			tdir = rdir
		} else {
			tmp, err := os.MkdirTemp("", "lotus-import-bench")
			if err != nil {
				return err
			}
			tdir = tmp
		}

		var (
			ds  datastore.Batching
			bs  blockstore.Blockstore
			err error
		)

		switch {
		case cctx.Bool("use-native-badger"):
			log.Info("using native badger")
			var opts badgerbs.Options
			if opts, err = repo.BadgerBlockstoreOptions(repo.UniversalBlockstore, tdir, false); err != nil {
				return err
			}
			opts.SyncWrites = false
			bs, err = badgerbs.Open(opts)

		default: // legacy badger via datastore.
			log.Info("using legacy badger")
			bdgOpt := badger.DefaultOptions
			bdgOpt.GcInterval = 0
			bdgOpt.Options = bdg.DefaultOptions("")
			bdgOpt.Options.SyncWrites = false
			bdgOpt.Options.Truncate = true
			bdgOpt.Options.DetectConflicts = false

			ds, err = badger.NewDatastore(tdir, &bdgOpt)
		}

		if err != nil {
			return err
		}

		if ds != nil {
			ds = measure.New("dsbench", ds)
			defer ds.Close() //nolint:errcheck
			bs = blockstore.FromDatastore(ds)
		}

		if c, ok := bs.(io.Closer); ok {
			defer c.Close() //nolint:errcheck
		}

		var verifier verifier.Verifier = verifierffi.ProofVerifier
		if cctx.IsSet("syscall-cache") {
			scds, err := badger.NewDatastore(cctx.String("syscall-cache"), &badger.DefaultOptions)
			if err != nil {
				return xerrors.Errorf("opening syscall-cache datastore: %w", err)
			}
			defer scds.Close() //nolint:errcheck

			verifier = &cachingVerifier{
				ds:      scds,
				backend: verifier,
			}
		}
		if cctx.Bool("only-gc") {
			return nil
		}

		metadataDs := datastore.NewMapDatastore()
		cs := store.NewChainStore(bs, bs, metadataDs, filcns.Weight, nil)
		defer cs.Close() //nolint:errcheck

		// TODO: We need to supply the actual beacon after v14
		stm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(verifier), filcns.DefaultUpgradeSchedule(), nil, metadataDs, index.DummyMsgIndex)
		if err != nil {
			return err
		}

		var carFile *os.File
		// open the CAR file if one is provided.
		if path := cctx.String("car"); path != "" {
			var err error
			if carFile, err = os.Open(path); err != nil {
				return xerrors.Errorf("failed to open provided CAR file: %w", err)
			}
		}

		startTime := time.Now()

		// register a gauge that reports how long since the measurable
		// operation began.
		promauto.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "lotus_bench_time_taken_secs",
		}, func() float64 {
			return time.Since(startTime).Seconds()
		})

		defer func() {
			end := time.Now().Format(time.RFC3339)

			resp, err := http.Get("http://localhost:6060/debug/metrics")
			if err != nil {
				log.Warnf("failed to scape prometheus: %s", err)
			}

			metricsfi, err := os.Create("bench.metrics")
			if err != nil {
				log.Warnf("failed to write prometheus data: %s", err)
			}

			_, _ = io.Copy(metricsfi, resp.Body) //nolint:errcheck
			_ = metricsfi.Close()                //nolint:errcheck

			writeProfile := func(name string) {
				if file, err := os.Create(fmt.Sprintf("%s.%s.%s.pprof", name, startTime.Format(time.RFC3339), end)); err == nil {
					if err := pprof.Lookup(name).WriteTo(file, 0); err != nil {
						log.Warnf("failed to write %s pprof: %s", name, err)
					}
					_ = file.Close()
				} else {
					log.Warnf("failed to create %s pprof file: %s", name, err)
				}
			}

			writeProfile("heap")
			writeProfile("allocs")
		}()

		var head *types.TipSet
		// --- IMPORT ---
		if !cctx.Bool("no-import") {
			if cctx.Bool("global-profile") {
				prof, err := os.Create("bench.import.pprof")
				if err != nil {
					return err
				}
				defer prof.Close() //nolint:errcheck

				if err := pprof.StartCPUProfile(prof); err != nil {
					return err
				}
			}

			// import is NOT suppressed; do it.
			if carFile == nil { // a CAR is compulsory for the import.
				return fmt.Errorf("no CAR file provided for import")
			}

			head, _, err = cs.Import(cctx.Context, carFile)
			if err != nil {
				return err
			}

			pprof.StopCPUProfile()
		}

		if cctx.Bool("only-import") {
			return nil
		}

		// --- VALIDATION ---
		//
		// we are now preparing for the validation benchmark.
		// a HEAD needs to be set; --head takes precedence over the root
		// of the CAR, if both are provided.
		if h := cctx.String("head"); h != "" {
			cids, err := lcli.ParseTipSetString(h)
			if err != nil {
				return xerrors.Errorf("failed to parse head tipset key: %w", err)
			}

			head, err = cs.LoadTipSet(cctx.Context, types.NewTipSetKey(cids...))
			if err != nil {
				return err
			}
		} else if carFile != nil && head == nil {
			cr, err := car.NewCarReader(carFile)
			if err != nil {
				return err
			}
			head, err = cs.LoadTipSet(cctx.Context, types.NewTipSetKey(cr.Header.Roots...))
			if err != nil {
				return err
			}
		} else if h == "" && carFile == nil {
			return xerrors.Errorf("neither --car nor --head flags supplied")
		}

		log.Infof("chain head is tipset: %s", head.Key())

		var genesis *types.TipSet
		log.Infof("getting genesis block")
		if tsk := cctx.String("genesis-tipset"); tsk != "" {
			var cids []cid.Cid
			if cids, err = lcli.ParseTipSetString(tsk); err != nil {
				return xerrors.Errorf("failed to parse genesis tipset key: %w", err)
			}
			genesis, err = cs.LoadTipSet(cctx.Context, types.NewTipSetKey(cids...))
		} else {
			log.Warnf("getting genesis by height; this will be slow; pass in the genesis tipset through --genesis-tipset")
			// fallback to the slow path of walking the chain.
			genesis, err = cs.GetTipsetByHeight(context.TODO(), 0, head, true)
		}

		if err != nil {
			return err
		}

		if err = cs.SetGenesis(cctx.Context, genesis.Blocks()[0]); err != nil {
			return err
		}

		// Resolve the end tipset, falling back to head if not provided.
		end := head
		if tsk := cctx.String("end-tipset"); tsk != "" {
			var cids []cid.Cid
			if cids, err = lcli.ParseTipSetString(tsk); err != nil {
				return xerrors.Errorf("failed to end genesis tipset key: %w", err)
			}
			end, err = cs.LoadTipSet(cctx.Context, types.NewTipSetKey(cids...))
		} else if h := cctx.Int64("end-height"); h != 0 {
			log.Infof("getting end tipset at height %d...", h)
			end, err = cs.GetTipsetByHeight(cctx.Context, abi.ChainEpoch(h), head, true)
		}

		if err != nil {
			return err
		}

		// Resolve the start tipset, if provided; otherwise, fallback to
		// height 1 for a start point.
		var (
			startEpoch = abi.ChainEpoch(1)
			start      *types.TipSet
		)

		if tsk := cctx.String("start-tipset"); tsk != "" {
			var cids []cid.Cid
			if cids, err = lcli.ParseTipSetString(tsk); err != nil {
				return xerrors.Errorf("failed to start genesis tipset key: %w", err)
			}
			start, err = cs.LoadTipSet(cctx.Context, types.NewTipSetKey(cids...))
		} else if h := cctx.Int64("start-height"); h != 0 {
			log.Infof("getting start tipset at height %d...", h)
			// lookback from the end tipset (which falls back to head if not supplied).
			start, err = cs.GetTipsetByHeight(context.TODO(), abi.ChainEpoch(h), end, true)
		}

		if err != nil {
			return err
		}

		if start != nil {
			startEpoch = start.Height()
			if err := cs.ForceHeadSilent(cctx.Context, start); err != nil {
				// if err := cs.SetHead(start); err != nil {
				return err
			}
		}

		inverseChain := append(make([]*types.TipSet, 0, end.Height()), end)
		for ts := end; ts.Height() > startEpoch; {
			if h := ts.Height(); h%100 == 0 {
				log.Infof("walking back the chain; loaded tipset at height %d...", h)
			}
			next, err := cs.LoadTipSet(cctx.Context, ts.Parents())
			if err != nil {
				return err
			}

			inverseChain = append(inverseChain, next)
			ts = next
		}

		var enc *json.Encoder
		if cctx.Bool("export-traces") {
			ibj, err := os.Create("bench.json")
			if err != nil {
				return err
			}
			defer ibj.Close() //nolint:errcheck

			enc = json.NewEncoder(ibj)
		}

		if cctx.Bool("global-profile") {
			prof, err := os.Create("bench.validation.pprof")
			if err != nil {
				return err
			}
			defer prof.Close() //nolint:errcheck

			if err := pprof.StartCPUProfile(prof); err != nil {
				return err
			}
		}

		for i := len(inverseChain) - 1; i >= 1; i-- {
			cur := inverseChain[i]
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
				if err := enc.Encode(tse); err != nil {
					return xerrors.Errorf("failed to write out tipsetexec: %w", err)
				}
			}
			if inverseChain[i-1].ParentState() != st {
				lastTrace := tse.Trace
				d, err := json.MarshalIndent(lastTrace, "", "  ")
				if err != nil {
					panic(err)
				}
				fmt.Println("TRACE")
				fmt.Println(string(d))
				//fmt.Println(statediff.Diff(context.Background(), bs, tschain[i-1].ParentState(), st, statediff.ExpandActors))
				return xerrors.Errorf("tipset chain had state mismatch at height %d (%s != %s)", cur.Height(), cur.ParentState(), st)
			}
		}

		pprof.StopCPUProfile()

		return nil
	},
}

type Invocation struct {
	TipSet types.TipSetKey
	Invoc  *api.InvocResult
}

const GasPerNs = 10

type stats struct {
	timeTaken meanVar
	gasRatio  meanVar
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

func tallyGasCharges(charges map[string]*stats, et types.ExecutionTrace) {
	for _, gc := range et.GasCharges {
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
		compGas := gc.ComputeGas
		s := charges[name]
		if s == nil {
			s = new(stats)
			charges[name] = s
		}

		s.timeTaken.AddPoint(tt)

		if compGas == 0 {
			compGas = 1
		}
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
			_ = http.ListenAndServe("localhost:6060", nil)
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
