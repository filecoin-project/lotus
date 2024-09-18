package main

import (
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

type IndexValidationJSON struct {
	TipsetKey        types.TipSetKey `json:"tipset_key"`
	IndexHeight      uint64          `json:"index_height"`
	IndexMsgCount    uint64          `json:"index_msg_count"`
	IndexEventsCount uint64          `json:"index_events_count"`
}

var validateChainIndexCmd = &cli.Command{
	Name:  "validate-chainindex",
	Usage: "Validates the chainindex for a range of epochs",
	Description: `
		validate-chainindex is a command-line tool that validates the chainindex for a specified range of epochs.
		It fetches the chain index entry for each epoch, checks for missing entries, and optionally backfills them.
		The command provides options to specify the range of epochs, control backfilling, and handle validation errors.

		Usage:
		lotus-shed validate-chainindex --from 200 --to 100 --backfill --output

		Flags:
		--from     Starting tipset epoch for validation (required)
		--to       Ending tipset epoch for validation (required)
		--backfill Backfill missing index entries (default: true)
		--output   Output backfilling results in JSON format (default: false)
	`,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:     "from",
			Usage:    "from specifies the starting tipset epoch for validation. If set to 0, validation starts from the tipset just before the current head",
			Required: true,
		},
		&cli.IntFlag{
			Name:     "to",
			Usage:    "to specifies the ending tipset epoch for validation",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "backfill",
			Usage: "backfill determines whether to backfill missing index entries during validation (default: true)",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "output",
			Usage: "output specifies whether to output the backfilling results in JSON format (default: true)",
			Value: true,
		},
	},
	Action: func(cctx *cli.Context) error {
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return xerrors.Errorf("failed to get full node services: %w", err)
		}
		defer func() {
			if closeErr := srv.Close(); closeErr != nil {
				log.Errorf("error closing services: %v", closeErr)
			}
		}()

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		fromEpoch := cctx.Int("from")
		if fromEpoch == 0 {
			curTs, err := api.ChainHead(ctx)
			if err != nil {
				return xerrors.Errorf("failed to get chain head: %w", err)
			}
			fromEpoch = int(curTs.Height()) - 1
		}

		if fromEpoch <= 0 {
			return xerrors.Errorf("invalid from epoch: %d, must be greater than 0", fromEpoch)
		}

		toEpoch := cctx.Int("to")
		if toEpoch > fromEpoch {
			return xerrors.Errorf("to epoch (%d) must be less than or equal to from epoch (%d)", toEpoch, fromEpoch)
		}

		if toEpoch <= 0 {
			return xerrors.Errorf("invalid to epoch: %d, must be greater than 0", toEpoch)
		}

		backfill := cctx.Bool("backfill")
		output := cctx.Bool("output")

		// Results Tracking
		var results []IndexValidationJSON
		var backfilledEpochs []int

		action := "chainindex validation"
		if backfill {
			action = "chainindex backfill and validation"
		}

		_, _ = fmt.Fprintf(cctx.App.Writer, "starting %s; from epoch: %d; to epoch: %d\n", action, fromEpoch, toEpoch)

		totalEpochs := fromEpoch - toEpoch + 1
		for epoch := fromEpoch; epoch >= toEpoch; epoch-- {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if (fromEpoch-epoch+1)%2880 == 0 {
				progress := float64(fromEpoch-epoch+1) / float64(totalEpochs) * 100
				_, _ = fmt.Fprintf(cctx.App.Writer, "chain index validation progress: %.2f%%\n", progress)
			}

			indexValidateResp, err := api.ChainValidateIndex(ctx, abi.ChainEpoch(epoch), backfill)
			if err != nil {
				return xerrors.Errorf("failed to validate index for epoch %d: %w", epoch, err)
			}

			if indexValidateResp == nil {
				results = append(results, IndexValidationJSON{
					TipsetKey:        types.EmptyTSK,
					IndexHeight:      uint64(epoch),
					IndexMsgCount:    0,
					IndexEventsCount: 0,
				})
				// this was a null round -> continue
				continue
			}

			if backfill {
				backfilledEpochs = append(backfilledEpochs, epoch)
			}

			if output {
				results = append(results, IndexValidationJSON{
					TipsetKey:        indexValidateResp.TipSetKey,
					IndexHeight:      indexValidateResp.Height,
					IndexMsgCount:    indexValidateResp.IndexedMessagesCount,
					IndexEventsCount: indexValidateResp.IndexedEventsCount,
				})
			}
		}

		// Output JSON Results
		if output {
			if err := outputResults(cctx, results); err != nil {
				return xerrors.Errorf("failed to output results: %w", err)
			}
		}

		// Log Summary
		_, _ = fmt.Fprintf(cctx.App.Writer, "validation of chain index from epoch %d to %d completed successfully; backfilled %d missing epochs\n", fromEpoch, toEpoch, len(backfilledEpochs))

		return nil
	},
}

// outputResults marshals the results into JSON and outputs them.
func outputResults(cctx *cli.Context, results []IndexValidationJSON) error {
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results to JSON: %w", err)
	}
	_, _ = fmt.Fprintf(cctx.App.Writer, "chain validation was successful for the following epochs: %s\n", string(jsonData))
	return nil
}
