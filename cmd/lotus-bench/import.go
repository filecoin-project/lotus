package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger2"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"gopkg.in/urfave/cli.v2"
)

type TipSetExec struct {
	TipSet   types.TipSetKey
	Trace    []*api.InvocResult
	Duration time.Duration
}

var importBenchCmd = &cli.Command{
	Name:  "import",
	Usage: "benchmark chain import and validation",
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
		defer cfi.Close()

		tdir, err := ioutil.TempDir("", "lotus-import-bench")
		if err != nil {
			return err
		}

		bds, err := badger.NewDatastore(tdir, nil)
		if err != nil {
			return err
		}
		bs := blockstore.NewBlockstore(bds)
		ds := datastore.NewMapDatastore()
		cs := store.NewChainStore(bs, ds, vm.Syscalls(ffiwrapper.ProofVerifier))
		stm := stmgr.NewStateManager(cs)

		prof, err := os.Create("import-bench.prof")
		if err != nil {
			return err
		}
		defer prof.Close()

		if err := pprof.StartCPUProfile(prof); err != nil {
			return err
		}

		head, err := cs.Import(cfi)
		if err != nil {
			return err
		}

		if h := cctx.Int64("height"); h != 0 {
			tsh, err := cs.GetTipsetByHeight(context.TODO(), abi.ChainEpoch(h), head)
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
				return xerrors.Errorf("tipset chain had state mismatch at height %d", cur.Height())
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
		defer ibj.Close()

		if err := json.NewEncoder(ibj).Encode(out); err != nil {
			return err
		}

		return nil

	},
}
