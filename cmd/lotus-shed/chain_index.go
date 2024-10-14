package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var chainIndexCmds = &cli.Command{
	Name:            "chainindex",
	Usage:           "Commands related to managing the chainindex",
	HideHelpCommand: true,
	Subcommands: []*cli.Command{
		withCategory("chainindex", validateBackfillChainIndexCmd),
	},
}

var validateBackfillChainIndexCmd = &cli.Command{
	Name:  "validate-backfill",
	Usage: "Validates and optionally backfills the chainindex for a range of epochs",
	Description: `
lotus-shed chainindex validate-backfill --from <start_epoch> --to <end_epoch> [--backfill] [--log-good]

The command validates the chain index entries for each epoch in the specified range, checking for missing or
inconsistent entries (i.e. the indexed data does not match the actual chain state). If '--backfill' is enabled
(which it is by default), it will attempt to backfill any missing entries using the 'ChainValidateIndex' API.

Parameters:
  - '--from' (required): The starting epoch (inclusive) for the validation range. Must be greater than 0.
  - '--to' (required): The ending epoch (inclusive) for the validation range. Must be greater than 0 and less
                        than or equal to 'from'.
  - '--backfill' (optional, default: true): Whether to backfill missing index entries during validation.
  - '--log-good' (optional, default: false): Whether to log details for tipsets that have no detected problems.

Error conditions:
  - If 'from' or 'to' are invalid (<=0 or 'to' > 'from'), an error is returned.
  - If the 'ChainValidateIndex' API returns an error for an epoch, indicating an inconsistency between the index
    and chain state, an error message is logged for that epoch.

Logging:
  - Progress is logged every 2880 epochs (1 day worth of epochs) processed during the validation process.
  - If '--log-good' is enabled, details are also logged for each epoch that has no detected problems. This includes:
    - Null rounds with no messages/events.
    - Epochs with a valid indexed entry.

Example usage:

To validate and backfill the chain index for the last 5760 epochs (2 days) and log details for all epochs:

lotus-shed chainindex validate-backfill --from 1000000 --to 994240 --log-good

This command is useful for backfilling the chain index over a range of historical epochs during the migration to
the new ChainIndexer. It can also be run periodically to validate the index's integrity using system schedulers
like cron.
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
	},
	Action: func(cctx *cli.Context) error {
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return xerrors.Errorf("failed to get full node services: %w", err)
		}
		defer func() {
			if closeErr := srv.Close(); closeErr != nil {
				log.Errorf("error closing services: %w", closeErr)
			}
		}()

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

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

		failedRPCs := 0
		successfulBackfills := 0
		successfulValidations := 0
		successfulNullRounds := 0

		startTime := time.Now()
		_, _ = fmt.Fprintf(cctx.App.Writer, "%s starting chainindex validation; from epoch: %d; to epoch: %d; backfill: %t; log-good: %t\n", currentTimeString(),
			fromEpoch, toEpoch, backfill, logGood)

		totalEpochs := fromEpoch - toEpoch + 1
		for epoch := fromEpoch; epoch >= toEpoch; epoch-- {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if (fromEpoch-epoch+1)%2880 == 0 || epoch == toEpoch {
				progress := float64(fromEpoch-epoch+1) / float64(totalEpochs) * 100
				elapsed := time.Since(startTime)
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "%s -------- Chain index validation progress: %.2f%%; Time elapsed: %s\n",
					currentTimeString(), progress, elapsed)
			}

			indexValidateResp, err := api.ChainValidateIndex(ctx, abi.ChainEpoch(epoch), backfill)
			if err != nil {
				tsKeyCid, err := tipsetKeyCid(ctx, abi.ChainEpoch(epoch), api)
				if err != nil {
					return fmt.Errorf("failed to get tipset key cid for epoch %d: %w", epoch, err)
				}
				_, _ = fmt.Fprintf(cctx.App.Writer, "%s ✗ Epoch %d (%s); failure: %s\n", currentTimeString(), epoch, tsKeyCid, err)
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
				tsKeyCid, err := tipsetKeyCid(ctx, abi.ChainEpoch(epoch), api)
				if err != nil {
					return fmt.Errorf("failed to get tipset key cid for epoch %d: %w", epoch, err)
				}
				_, _ = fmt.Fprintf(cctx.App.Writer, "%s ✓ Epoch %d (%s); null round\n", currentTimeString(), epoch,
					tsKeyCid)
			} else {
				jsonData, err := json.Marshal(indexValidateResp)
				if err != nil {
					return fmt.Errorf("failed to marshal results to JSON: %w", err)
				}

				_, _ = fmt.Fprintf(cctx.App.Writer, "%s ✓ Epoch %d (%s)\n", currentTimeString(), epoch, string(jsonData))
			}
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "\n%s Chain index validation summary:\n", currentTimeString())
		_, _ = fmt.Fprintf(cctx.App.Writer, "Total failed RPC calls: %d\n", failedRPCs)
		_, _ = fmt.Fprintf(cctx.App.Writer, "Total successful backfills: %d\n", successfulBackfills)
		_, _ = fmt.Fprintf(cctx.App.Writer, "Total successful validations without backfilling: %d\n", successfulValidations)
		_, _ = fmt.Fprintf(cctx.App.Writer, "Total successful Null round validations: %d\n", successfulNullRounds)

		return nil
	},
}

func tipsetKeyCid(ctx context.Context, epoch abi.ChainEpoch, a api.FullNode) (cid.Cid, error) {
	ts, err := a.ChainGetTipSetByHeight(ctx, epoch, types.EmptyTSK)
	if err != nil {
		return cid.Undef, fmt.Errorf("failed to get tipset for epoch %d: %w", epoch, err)
	}
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return cid.Undef, fmt.Errorf("failed to get tipset key cid for epoch %d: %w", epoch, err)
	}
	return tsKeyCid, nil
}

func currentTimeString() string {
	currentTime := time.Now().Format("2006-01-02 15:04:05.000")
	return currentTime
}
