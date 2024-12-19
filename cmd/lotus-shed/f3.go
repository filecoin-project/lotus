package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-f3/gpbft"

	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/node/repo"
)

var f3Cmd = &cli.Command{
	Name:        "f3",
	Description: "f3 related commands",
	Subcommands: []*cli.Command{
		f3ClearStateCmd,
		f3GenExplicitPower,
	},
}

func loadF3IDList(path string) ([]gpbft.ActorID, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close() //nolint:errcheck

	var ids []gpbft.ActorID
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		id, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ID: %w", err)
		}

		ids = append(ids, gpbft.ActorID(id))
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return ids, nil
}

var f3GenExplicitPower = &cli.Command{
	Name:        "gen-explicit-power",
	Description: "generates an explicit power table",

	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:  "good-list",
			Usage: "new line delimited file with known good IDs to be included",
		},
		&cli.PathFlag{
			Name:  "bad-list",
			Usage: "new line delimited file with known bad IDs to be excluded",
		},
		&cli.IntFlag{
			Name:  "n",
			Usage: "generate n entries, exclusive with ratio",
		},
		&cli.Float64Flag{
			Name:  "ratio",
			Usage: "generate given ratio of full power table, exclusive with N",
		},
		&cli.Int64Flag{
			Name:  "seed",
			Usage: "seed for randomization, -1 will use current nano time",
			Value: -1,
		},
		&cli.Uint64Flag{
			Name:  "iteration",
			Usage: "the iteration of randomization, random entries will be exclusive across iterations",
			Value: 0,
		},
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids) or @epoch",
		},
	},

	Action: func(cctx *cli.Context) error {
		ctx := cliutil.ReqContext(cctx)
		api, closer, err := cliutil.GetFullNodeAPIV1(cctx)
		if err != nil {
			return fmt.Errorf("getting api: %w", err)
		}
		defer closer()

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return fmt.Errorf("getting chain head: %w", err)
		}
		if cctx.IsSet("n") && cctx.IsSet("ratio") {
			return fmt.Errorf("n and ratio options are exclusive")
		}

		allPowerEntries, err := api.F3GetECPowerTable(ctx, ts.Key())
		if err != nil {
			return fmt.Errorf("getting power entries: %w", err)
		}

		powerMap := map[gpbft.ActorID]gpbft.PowerEntry{}
		for _, pe := range allPowerEntries {
			powerMap[pe.ID] = pe
		}
		var goodList []gpbft.ActorID
		if goodPath := cctx.Path("good-list"); goodPath != "" {
			goodList, err = loadF3IDList(goodPath)
			if err != nil {
				return fmt.Errorf("loading good list: %w", err)
			}
		}

		var badList []gpbft.ActorID
		if badPath := cctx.Path("bad-list"); badPath != "" {
			badList, err = loadF3IDList(badPath)
			if err != nil {
				return fmt.Errorf("loading bad list: %w", err)
			}
		}
		total := len(powerMap)
		for _, id := range badList {
			delete(powerMap, id)
		}

		var result gpbft.PowerEntries
		add := func(id gpbft.ActorID) {
			result = append(result, powerMap[id])
			delete(powerMap, id)
		}

		for _, id := range goodList {
			if _, ok := powerMap[id]; ok {
				add(id)
			}
		}

		seed := cctx.Int64("seed")
		if seed == -1 {
			seed = time.Now().UnixNano()
		}
		rng := rand.New(rand.NewSource(seed))

		endSize := cctx.Int("n")
		if cctx.IsSet("ratio") {
			endSize = int(float64(total) * cctx.Float64("ratio"))
		}
		if toAdd := endSize - len(result); toAdd > 0 {
			var powerList gpbft.PowerEntries
			for _, pe := range powerMap {
				powerList = append(powerList, pe)
			}
			sort.Sort(powerList)
			rng.Shuffle(len(powerList), powerList.Swap)

			iteration := cctx.Int("iteration")
			startIdx := min(toAdd*iteration, len(powerList))
			endIdx := min(toAdd*(iteration+1), len(powerList))
			result = append(result, powerList[startIdx:endIdx]...)
		}

		if len(result) > endSize {
			result = result[:endSize]
		}
		sort.Sort(result)
		res, err := json.MarshalIndent(result, "  ", "  ")
		if err != nil {
			return fmt.Errorf("marshalling to json: %w", err)
		}
		_, err = cctx.App.Writer.Write(res)
		if err != nil {
			return fmt.Errorf("writing result: %w", err)
		}

		return nil
	},
}

var f3ClearStateCmd = &cli.Command{
	Name:        "clear-state",
	Description: "remove all f3 state",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "really-do-it",
		},
	},
	Action: func(cctx *cli.Context) error {
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return xerrors.Errorf("lotus repo doesn't exist")
		}

		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return err
		}
		defer lr.Close() //nolint:errcheck

		ds, err := lr.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		dryRun := !cctx.Bool("really-do-it")

		q, err := ds.Query(context.Background(), dsq.Query{
			Prefix:   "/f3",
			KeysOnly: true,
		})
		if err != nil {
			return xerrors.Errorf("datastore query: %w", err)
		}
		defer q.Close() //nolint:errcheck

		batch, err := ds.Batch(cctx.Context)
		if err != nil {
			return xerrors.Errorf("failed to create a datastore batch: %w", err)
		}

		for r, ok := q.NextSync(); ok; r, ok = q.NextSync() {
			if r.Error != nil {
				return xerrors.Errorf("failed to read datastore: %w", err)
			}
			_, _ = fmt.Fprintf(cctx.App.Writer, "deleting: %q\n", r.Key)
			if !dryRun {
				if err := batch.Delete(cctx.Context, datastore.NewKey(r.Key)); err != nil {
					return xerrors.Errorf("failed to delete %q: %w", r.Key, err)
				}
			}
		}
		if !dryRun {
			if err := batch.Commit(cctx.Context); err != nil {
				return xerrors.Errorf("failed to flush the batch: %w", err)
			}
		}

		if err := ds.Close(); err != nil {
			return xerrors.Errorf("error when closing datastore: %w", err)
		}

		if dryRun {
			_, _ = fmt.Fprintln(cctx.App.Writer, "NOTE: dry run complete, re-run with --really-do-it to actually delete F3 state")
		}

		return nil
	},
}
