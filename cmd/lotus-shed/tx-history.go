package main

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

var txHistoryCmd = &cli.Command{
	Name:  "tx-history",
	Usage: "Export all historical transactions to CSV (gas_limit, max_priority_fee, max_fee, epoch, base_fee)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "output",
			Value: "tx-history.csv",
			Usage: "output CSV file path",
		},
		&cli.Int64Flag{
			Name:  "from",
			Value: -1,
			Usage: "starting epoch (default: chain head)",
		},
		&cli.Int64Flag{
			Name:  "to",
			Value: 0,
			Usage: "ending epoch (inclusive, default: 0 i.e. walk full history)",
		},
		&cli.StringFlag{
			Name:  "api",
			Value: "/ip4/127.0.0.1/tcp/1234/http",
			Usage: "lotus API multiaddr (token not required for read-only access)",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := lcli.ReqContext(cctx)

		apiInfo := cliutil.ParseApiInfo(cctx.String("api"))
		addr, err := apiInfo.DialArgs("v1")
		if err != nil {
			return fmt.Errorf("parsing api address: %w", err)
		}

		api, closer, err := client.NewFullNodeRPCV1(ctx, addr, apiInfo.AuthHeader())
		if err != nil {
			return fmt.Errorf("connecting to lotus API: %w", err)
		}
		defer closer()

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		startEpoch := head.Height()
		if cctx.IsSet("from") {
			startEpoch = abi.ChainEpoch(cctx.Int64("from"))
		}
		endEpoch := abi.ChainEpoch(cctx.Int64("to"))

		ts := head
		if startEpoch != head.Height() {
			ts, err = api.ChainGetTipSetByHeight(ctx, startEpoch, types.EmptyTSK)
			if err != nil {
				return fmt.Errorf("getting tipset at height %d: %w", startEpoch, err)
			}
		}

		outPath := cctx.String("output")
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close() //nolint:errcheck

		w := csv.NewWriter(f)
		if err := w.Write([]string{"gas_limit", "max_priority_fee_per_gas", "max_fee_per_gas", "epoch", "base_fee"}); err != nil {
			return err
		}

		written := 0
		for ts.Height() > endEpoch {
			epoch := ts.Height()
			baseFee := ts.Blocks()[0].ParentBaseFee

			// Collect messages from all blocks in the tipset, deduplicating by CID.
			seen := make(map[cid.Cid]struct{})
			for _, blk := range ts.Blocks() {
				msgs, err := api.ChainGetBlockMessages(ctx, blk.Cid())
				if err != nil {
					fmt.Fprintf(os.Stderr, "skipping epoch %d (block %s): %s\n", epoch, blk.Cid(), err)
					break
				}
				for i, msg := range msgs.BlsMessages {
					c := msgs.Cids[i]
					if _, ok := seen[c]; ok {
						continue
					}
					seen[c] = struct{}{}
					if err := w.Write([]string{
						fmt.Sprintf("%d", msg.GasLimit),
						msg.GasPremium.String(),
						msg.GasFeeCap.String(),
						fmt.Sprintf("%d", epoch),
						baseFee.String(),
					}); err != nil {
						return err
					}
					written++
				}
				for i, smsg := range msgs.SecpkMessages {
					c := msgs.Cids[len(msgs.BlsMessages)+i]
					if _, ok := seen[c]; ok {
						continue
					}
					seen[c] = struct{}{}
					msg := smsg.Message
					if err := w.Write([]string{
						fmt.Sprintf("%d", msg.GasLimit),
						msg.GasPremium.String(),
						msg.GasFeeCap.String(),
						fmt.Sprintf("%d", epoch),
						baseFee.String(),
					}); err != nil {
						return err
					}
					written++
				}
			}

			if written > 0 && written%100000 == 0 {
				w.Flush()
				fmt.Fprintf(os.Stderr, "epoch %d: %d rows written\n", epoch, written)
			}

			parentTs, err := api.ChainGetTipSet(ctx, ts.Parents())
			if err != nil {
				fmt.Fprintf(os.Stderr, "stopping at epoch %d: could not load parent tipset: %s\n", epoch, err)
				break
			}
			ts = parentTs
			if ts.Height() == 0 {
				break
			}
		}

		w.Flush()
		if err := w.Error(); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "done: %d rows written to %s\n", written, outPath)
		return nil
	},
}
