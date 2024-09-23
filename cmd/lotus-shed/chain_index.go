package main

import (
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

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
		'./lotus-shed chainindex validate-backfill' is a command-line tool that validates the chainindex for a specified range of epochs.
		It validates the chain index entries for each epoch, checks for missing entries, and optionally backfills them.

		Usage:
		lotus-shed chainindex validate-backfill --from 200 --to 100 --backfill --log-good

		Flags:
		--from     Starting tipset epoch for validation (inclusive) (required)
		--to       Ending tipset epoch for validation (inclusive) (required)
		--backfill Backfill missing index entries (default: true)
		--log-good Log tipsets that have no detected problems (default: false)
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

		backfill := cctx.Bool("backfill")

		// Results Tracking
		logGood := cctx.Bool("log-good")

		_, _ = fmt.Fprintf(cctx.App.Writer, "starting chainindex validation; from epoch: %d; to epoch: %d; backfill: %t; log-good: %t\n", fromEpoch, toEpoch,
			backfill, logGood)

		totalEpochs := fromEpoch - toEpoch + 1
		for epoch := fromEpoch; epoch >= toEpoch; epoch-- {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if (fromEpoch-epoch+1)%2880 == 0 {
				progress := float64(fromEpoch-epoch+1) / float64(totalEpochs) * 100
				log.Infof("-------- chain index validation progress: %.2f%%\n", progress)
			}

			indexValidateResp, err := api.ChainValidateIndex(ctx, abi.ChainEpoch(epoch), backfill)
			if err != nil {
				_, _ = fmt.Fprintf(cctx.App.Writer, "✗ Epoch %d; failure: %s\n", epoch, err)
				continue
			}

			if !logGood {
				continue
			}

			// is it a null round ?
			if indexValidateResp.IsNullRound {
				_, _ = fmt.Fprintf(cctx.App.Writer, "✓ Epoch %d; null round\n", epoch)
			} else {
				jsonData, err := json.Marshal(indexValidateResp)
				if err != nil {
					return fmt.Errorf("failed to marshal results to JSON: %w", err)
				}

				_, _ = fmt.Fprintf(cctx.App.Writer, "✓ Epoch %d (%s)\n", epoch, string(jsonData))
			}
		}

		return nil
	},
}
