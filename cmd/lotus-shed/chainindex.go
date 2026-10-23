package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/index"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/node/repo"
)

const (
	restoreBatch = 2880
	// restoreQuietStretch is the span of unverified tipsets worth reporting:
	// a day without indexed events is plausible early after FEVM, but is also
	// what event storage being off looks like
	restoreQuietStretch = 2880
)

var chainIndexCmd = &cli.Command{
	Name:  "chainindex",
	Usage: "Offline tools for the chain index database",
	Subcommands: []*cli.Command{
		chainIndexRestoreBloomsCmd,
	},
}

var chainIndexRestoreBloomsCmd = &cli.Command{
	Name:  "restore-blooms",
	Usage: "Write missing tipset blooms from the events already in the chain index",
	Description: `The chain index stores a logs bloom per tipset, and treats a tipset without one as not yet
indexed: eth_getLogs and GetActorEventsRaw over it fail with "not found in index". Indexes built by
older Lotus versions lack blooms for older tipsets. 'lotus index validate-backfill' restores them,
but only after re-verifying each epoch against chain state, which can take weeks on a large
archival node.

This command builds each missing bloom from the events already in the index, without reading the
chain, and replaces all-0xff placeholder blooms the same way.

The index proves a tipset's events are complete if the tipset has no messages, has indexed events,
or predates FEVM. A tipset with messages but no indexed events after FEVM is unverified: it may
have emitted none, or its events may never have been indexed, for example because it was indexed
from a snapshot import or with event storage off. Unverified tipsets are restored too, with an
empty bloom, so queries over them answer rather than fail, and stretches of a day or more of them
are listed with the 'lotus index validate-backfill' command that verifies them against the chain
and repairs any that are wrong. Tipsets whose indexed events are reverted are left alone and listed.

The Lotus daemon must be stopped. Back up chainindex.db, -wal and -shm together first. Opening the
index migrates its schema as the daemon would, which can prevent a downgrade to an older Lotus.
Afterwards, 'lotus index validate-backfill' over a range verifies the result against the chain and
repairs it.`,
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:        "from",
			Usage:       "highest height to restore",
			DefaultText: "highest indexed height minus finality",
		},
		&cli.Int64Flag{
			Name:        "to",
			Usage:       "lowest height to restore",
			DefaultText: "lowest indexed height",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "count the tipsets that would be restored without writing",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}
		exists, err := r.Exists()
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("lotus repo doesn't exist")
		}
		lr, err := r.Lock(repo.FullNode)
		if err != nil {
			return xerrors.Errorf("locking repo, is the daemon running? %w", err)
		}
		defer lr.Close() //nolint:errcheck

		indexPath, err := lr.ChainIndexPath()
		if err != nil {
			return err
		}
		dbPath := filepath.Join(indexPath, index.DefaultDbFilename)
		if _, err := os.Stat(dbPath); err != nil {
			return xerrors.Errorf("chain index not found: %w", err)
		}

		si, err := index.NewSqliteIndexer(dbPath, nil, 0, false, 0)
		if err != nil {
			return xerrors.Errorf("opening chain index: %w", err)
		}
		defer si.Close() //nolint:errcheck

		lowest, highest, ok, err := si.IndexedRange(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("chain index is empty")
		}

		from := highest - policy.ChainFinality
		if cctx.IsSet("from") {
			from = abi.ChainEpoch(cctx.Int64("from"))
			if from > highest-policy.ChainFinality {
				return fmt.Errorf("--from (%d) must be at least %d epochs below the highest indexed height (%d)", from, policy.ChainFinality, highest)
			}
		}
		to := lowest
		if cctx.IsSet("to") {
			to = abi.ChainEpoch(cctx.Int64("to"))
		}
		if from < to {
			_, _ = fmt.Fprintf(cctx.App.Writer, "nothing to do: from (%d) is below to (%d)\n", from, to)
			return nil
		}

		dryRun := cctx.Bool("dry-run")
		verb := "restored"
		if dryRun {
			verb = "to restore"
		}
		summary := func(c *index.RestoreTipsetBloomsCounts) string {
			return fmt.Sprintf("%d %s, %d of them unverified", c.Restored(), verb, c.Unverified)
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "index: %s\nrestoring blooms from height %d down to %d (indexed %d-%d)\n",
			dbPath, from, to, lowest, highest)

		start := time.Now()
		counts, err := si.RestoreTipsetBlooms(ctx, index.RestoreTipsetBloomsOptions{
			From:         from,
			To:           to,
			Batch:        restoreBatch,
			QuietStretch: restoreQuietStretch,
			DryRun:       dryRun,
			Progress: func(lo, hi abi.ChainEpoch, c *index.RestoreTipsetBloomsCounts) {
				pct := float64(from-lo+1) / float64(from-to+1) * 100
				_, _ = fmt.Fprintf(cctx.App.Writer, "%s heights %d-%d done, %.1f%%, %s elapsed: %s\n",
					time.Now().Format(time.DateTime), lo, hi, pct, time.Since(start).Round(time.Second), summary(c))
			},
		})
		if err != nil {
			return err
		}

		w := cctx.App.Writer
		_, _ = fmt.Fprintf(w, "\ndone in %s\n", time.Since(start).Round(time.Second))
		_, _ = fmt.Fprintf(w, "%s:\n", verb)
		_, _ = fmt.Fprintf(w, "  %d with indexed events\n", counts.WithEvents)
		_, _ = fmt.Fprintf(w, "  %d without messages\n", counts.NoMessages)
		_, _ = fmt.Fprintf(w, "  %d without events, before FEVM (height %d)\n", counts.BeforeEvents, buildconstants.UpgradeHyggeHeight)
		_, _ = fmt.Fprintf(w, "  %d without events, unverified\n", counts.Unverified)
		if len(counts.QuietStretches) > 0 {
			_, _ = fmt.Fprintf(w, "\nunverified stretches of %d epochs or more without indexed events, worth verifying\nagainst the chain once the daemon is running:\n", restoreQuietStretch)
			for _, r := range counts.QuietStretches {
				_, _ = fmt.Fprintf(w, "  lotus index validate-backfill --from %d --to %d\n", r.From, r.To)
			}
		}
		if len(counts.RevertedEvents) > 0 {
			_, _ = fmt.Fprintf(w, "\nleft alone, with reverted indexed events:\n")
			for _, h := range counts.RevertedEvents {
				_, _ = fmt.Fprintf(w, "  lotus index validate-backfill --from %d --to %d\n", h, h)
			}
		}
		return nil
	},
}
