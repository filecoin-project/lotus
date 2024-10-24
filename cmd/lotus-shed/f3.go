package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var f3Cmd = &cli.Command{
	Name:        "f3",
	Description: "f3 related commands",
	Subcommands: []*cli.Command{
		f3ClearStateCmd,
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
			fmt.Fprintf(cctx.App.Writer, "deleting: %q\n", r.Key)
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
			fmt.Fprintln(cctx.App.Writer, "NOTE: dry run complete, re-run with --really-do-it to actually delete F3 state")
		}

		return nil
	},
}
