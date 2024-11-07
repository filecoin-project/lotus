package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
)

var IndexCmd = &cli.Command{
	Name:  "index",
	Usage: "Commands related to managing the chainindex",
	Subcommands: []*cli.Command{
		validateBackfillChainIndexCmd,
	},
}

var validateBackfillChainIndexCmd = &cli.Command{
	Name:  "validate-backfill",
	Usage: "Validates and optionally backfills the chainindex for a range of epochs",
	Description: `
lotus index validate-backfill --from <start_epoch> --to <end_epoch> [--backfill] [--log-good] [--quiet]

The command validates the chain index entries for each epoch in the specified range, checking for missing or
inconsistent entries (i.e. the indexed data does not match the actual chain state). If '--backfill' is enabled
(which it is by default), it will attempt to backfill any missing entries using the 'ChainValidateIndex' API.

Error conditions:
  - If 'from' or 'to' are invalid (<=0 or 'to' > 'from'), an error is returned.
  - If the 'ChainValidateIndex' API returns an error for an epoch, indicating an inconsistency between the index
    and chain state, an error message is logged for that epoch.

Logging:
  - Progress is logged every 2880 epochs (1 day worth of epochs) processed during the validation process.
  - If '--log-good' is enabled, details are also logged for each epoch that has no detected problems. This includes:
    - Null rounds with no messages/events.
    - Epochs with a valid indexed entry.
  - If --quiet is enabled, only errors are logged, unless --log-good is also enabled, in which case good tipsets
    are also logged.

Example usage:

To validate and backfill the chain index for the last 5760 epochs (2 days) and log details for all epochs:

lotus index validate-backfill --from 1000000 --to 994240 --log-good

This command is useful for backfilling the chain index over a range of historical epochs during the migration to
the new ChainIndexer. It can also be run periodically to validate the index's integrity using system schedulers
like cron.

If there are any errors during the validation process, the command will exit with a non-zero status and log the
number of failed RPC calls. Otherwise, it will exit with a zero status.
	`,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:     "from",
			Usage:    "from specifies the starting tipset epoch for validation (inclusive)",
			Required: true,
		},
		&cli.IntFlag{
			Name:     "to",
			Usage:    "to specifies the ending tipset epoch for validation (inclusive)",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "backfill",
			Usage: "backfill determines whether to backfill missing index entries during validation (default: true)",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "log-good",
			Usage: "log tipsets that have no detected problems",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "quiet",
			Usage: "suppress output except for errors (or good tipsets if log-good is enabled)",
		},
	},
	Action: func(cctx *cli.Context) error {
		srv, err := GetFullNodeServices(cctx)
		if err != nil {
			return xerrors.Errorf("failed to get full node services: %w", err)
		}
		defer func() {
			if closeErr := srv.Close(); closeErr != nil {
				log.Errorf("error closing services: %w", closeErr)
			}
		}()

		api := srv.FullNodeAPI()
		ctx := ReqContext(cctx)

		fromEpoch := cctx.Int("from")
		if fromEpoch <= 0 {
			return xerrors.Errorf("invalid from epoch: %d, must be greater than 0", fromEpoch)
		}

		toEpoch := cctx.Int("to")
		if toEpoch <= 0 {
			return xerrors.Errorf("invalid to epoch: %d, must be greater than 0", toEpoch)
		}
		if toEpoch > fromEpoch {
			return xerrors.Errorf("to epoch (%d) must be less than or equal to from epoch (%d)", toEpoch, fromEpoch)
		}

		head, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get chain head: %w", err)
		}
		if head.Height() <= abi.ChainEpoch(fromEpoch) {
			return xerrors.Errorf("from epoch (%d) must be less than chain head (%d)", fromEpoch, head.Height())
		}

		backfill := cctx.Bool("backfill")

		// Results Tracking
		logGood := cctx.Bool("log-good")

		quiet := cctx.Bool("quiet")

		failedRPCs := 0
		successfulBackfills := 0
		successfulValidations := 0
		successfulNullRounds := 0

		startTime := time.Now()
		if !quiet {
			_, _ = fmt.Fprintf(cctx.App.Writer, "%s starting chainindex validation; from epoch: %d; to epoch: %d; backfill: %t; log-good: %t\n", currentTimeString(),
				fromEpoch, toEpoch, backfill, logGood)
		}
		totalEpochs := fromEpoch - toEpoch + 1
		haltHeight := -1

		for epoch := fromEpoch; epoch >= toEpoch; epoch-- {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if (fromEpoch-epoch+1)%2880 == 0 || epoch == toEpoch {
				progress := float64(fromEpoch-epoch+1) / float64(totalEpochs) * 100
				elapsed := time.Since(startTime)
				if !quiet {
					_, _ = fmt.Fprintf(cctx.App.ErrWriter, "%s -------- Chain index validation progress: %.2f%%; Time elapsed: %s\n",
						currentTimeString(), progress, elapsed)
				}
			}

			indexValidateResp, err := api.ChainValidateIndex(ctx, abi.ChainEpoch(epoch), backfill)
			if err != nil {
				if strings.Contains(err.Error(), "chain store does not contain data") {
					haltHeight = epoch
					break
				}

				_, _ = fmt.Fprintf(cctx.App.Writer, "%s ✗ Epoch %d; failure: %s\n", currentTimeString(), epoch, err)
				failedRPCs++
				continue
			}

			if indexValidateResp.Backfilled {
				successfulBackfills++
			} else if indexValidateResp.IsNullRound {
				successfulNullRounds++
			} else {
				successfulValidations++
			}

			if !logGood {
				continue
			}

			if indexValidateResp.IsNullRound {
				_, _ = fmt.Fprintf(cctx.App.Writer, "%s ✓ Epoch %d; null round\n", currentTimeString(), epoch)
			} else {
				jsonData, err := json.Marshal(indexValidateResp)
				if err != nil {
					return fmt.Errorf("failed to marshal results to JSON: %w", err)
				}

				_, _ = fmt.Fprintf(cctx.App.Writer, "%s ✓ Epoch %d (%s)\n", currentTimeString(), epoch, string(jsonData))
			}
		}

		if !quiet {
			_, _ = fmt.Fprintf(cctx.App.Writer, "\n%s Chain index validation summary:\n", currentTimeString())
			_, _ = fmt.Fprintf(cctx.App.Writer, "Total failed RPC calls: %d\n", failedRPCs)
			_, _ = fmt.Fprintf(cctx.App.Writer, "Total successful backfills: %d\n", successfulBackfills)
			_, _ = fmt.Fprintf(cctx.App.Writer, "Total successful validations without backfilling: %d\n", successfulValidations)
			_, _ = fmt.Fprintf(cctx.App.Writer, "Total successful Null round validations: %d\n", successfulNullRounds)
		}

		if haltHeight >= 0 {
			return fmt.Errorf("chain index validation and backfilled halted at height %d as chain state does contain data for that height", haltHeight)
		} else if failedRPCs > 0 {
			return fmt.Errorf("chain index validation failed with %d RPC errors", failedRPCs)
		}

		return nil
	},
}

func currentTimeString() string {
	currentTime := time.Now().Format("2006-01-02 15:04:05.000")
	return currentTime
}
