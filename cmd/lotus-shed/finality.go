package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/ecfinality"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

const (
	// recentHealthWindow is the number of recent epochs used to compute average
	// blocks/epoch for the chain health display.
	recentHealthWindow = 30
)

var finalityCmd = &cli.Command{
	Name:  "finality-calculator",
	Usage: "Calculate the reorg probability of a tipset at various depths",
	Description: `Compute the probability that a confirmed tipset could be reorganized out
of the canonical chain, based on observed block production and the FRC-0089 model
(https://github.com/filecoin-project/FIPs/blob/master/FRCs/frc-0089.md).

Under healthy network conditions (tipsets with ~5 blocks), the 2^-30 finality guarantee
(~one-in-a-billion chance of reorg) is typically achieved within ~30 epochs (~15 minutes),
compared to the static 900-epoch (~7.5 hour) EC finality assumption.

Chain history can be read from a running Lotus node or from a text file where each line
contains the number of blocks in an epoch (most recent epoch last).

By default, displays a summary table with finality at key depths. Use --csv for
machine-readable output of all 900 epochs.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "input",
			Usage: "Read chain history from a file instead of querying a Lotus node. File should contain one integer per line (block count per epoch), most recent epoch last.",
		},
		&cli.Float64Flag{
			Name:  "blocks-per-epoch",
			Value: ecfinality.DefaultBlocksPerEpoch,
			Usage: "Expected number of blocks per epoch (Filecoin mainnet protocol constant is 5). Changing this models a different expected block production rate.",
		},
		&cli.Float64Flag{
			Name:  "byzantine-fraction",
			Value: ecfinality.DefaultByzantineFraction,
			Usage: "Assumed upper bound on adversarial mining power as a fraction (0.0-1.0). The standard Filecoin security assumption is 0.3 (30%). Lower values model a more secure network; higher values model stronger adversaries.",
		},
		&cli.IntFlag{
			Name:  "safety-exponent",
			Value: ecfinality.DefaultSafetyExponent,
			Usage: "Safety target as a power of 2 (e.g. -30 means 2^-30). This is the maximum acceptable probability of a tipset being reorganized. The Filecoin standard is -30, which the 900-epoch static finality was designed to achieve.",
		},
		&cli.BoolFlag{
			Name:  "csv",
			Usage: "Output raw CSV (epoch,depth,reorg_probability) for all epochs back to EC finality, suitable for piping to other tools or plotting.",
		},
	},
	Action: func(cctx *cli.Context) error {
		var chain []int
		var headEpoch int

		if cctx.String("input") == "" {
			api, closer, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()
			ctx := lcli.ReqContext(cctx)

			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}
			headEpoch = int(head.Height())
			readLength := int(policy.ChainFinality) + 5
			chain = append(chain, len(head.Cids()))
			for range readLength {
				head, err = api.ChainGetTipSet(ctx, head.Parents())
				if err != nil {
					return err
				}
				chain = append(chain, len(head.Cids()))
			}
			// API walk produces most-recent-first; reverse to match the
			// expected ordering (index 0 = earliest epoch).
			slices.Reverse(chain)
		} else {
			input := cctx.String("input")
			file, err := os.Open(input)
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				num, err := strconv.Atoi(scanner.Text())
				if err != nil {
					return err
				}
				chain = append(chain, num)
			}
			if err := scanner.Err(); err != nil {
				return err
			}
			headEpoch = len(chain) - 1
		}

		if len(chain) < 2 {
			return fmt.Errorf("chain must contain at least 2 epochs, got %d", len(chain))
		}

		blocksPerEpoch := cctx.Float64("blocks-per-epoch")
		byzantineFraction := cctx.Float64("byzantine-fraction")
		safetyExponent := cctx.Int("safety-exponent")
		guarantee := math.Pow(2, float64(safetyExponent))
		finality := int(policy.ChainFinality)
		epochDurationSecs := float64(buildconstants.BlockDelaySecs)
		currentEpoch := len(chain) - 1 // head of the chain segment we are considering, not actual Filecoin epoch number
		out := cctx.App.Writer

		epochToTime := func(depth int) string {
			secs := float64(depth) * epochDurationSecs
			if secs < 120 {
				return fmt.Sprintf("~%.0fs", secs)
			}
			return fmt.Sprintf("~%.0fm", secs/60)
		}

		// CSV mode: raw output for all epochs
		if cctx.Bool("csv") {
			_, _ = fmt.Fprintln(out, "epoch,depth,reorg_probability")
			for i := range min(finality, currentEpoch) {
				prob := ecfinality.CalcValidatorProb(chain, finality, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-i)
				_, _ = fmt.Fprintf(out, "%d,%d,%e\n", headEpoch-i, i, prob)
			}
			return nil
		}

		// Compute chain health: average blocks per epoch over recent history
		recentBlocks := 0
		recentCount := max(1, min(recentHealthWindow, currentEpoch))
		for i := range recentCount {
			recentBlocks += chain[currentEpoch-i]
		}
		avgBlocks := float64(recentBlocks) / float64(recentCount)
		healthLabel := "healthy"
		if avgBlocks < 3.0 {
			healthLabel = "poor"
		} else if avgBlocks < 4.0 {
			healthLabel = "degraded"
		}

		// Bisect search for the finality threshold depth
		thresholdDepth := ecfinality.FindThresholdDepth(chain, finality, blocksPerEpoch, byzantineFraction, guarantee)

		// Print summary header
		_, _ = fmt.Fprintln(out, "EC Finality Calculator (FRC-0089)")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "  Head epoch:       %d\n", headEpoch)
		_, _ = fmt.Fprintf(out, "  Chain health:     %.1f avg blocks/epoch (last %d), %s\n", avgBlocks, recentCount, healthLabel)
		_, _ = fmt.Fprintf(out, "  Adversary:        %.0f%% Byzantine power\n", byzantineFraction*100)
		_, _ = fmt.Fprintf(out, "  Safety target:    2^%d (%.2e)\n", safetyExponent, guarantee)
		_, _ = fmt.Fprintln(out)

		if thresholdDepth > 0 {
			_, _ = fmt.Fprintf(out, "  Probabilistic finality target (2^%d) reached at depth %d (%s)\n", safetyExponent, thresholdDepth, epochToTime(thresholdDepth))
		} else {
			_, _ = fmt.Fprintf(out, "  Probabilistic finality target (2^%d) NOT reached within %d epochs (chain may be unhealthy)\n", safetyExponent, ecfinality.BisectHigh)
		}
		_, _ = fmt.Fprintln(out)

		// Compute probabilities at key depths for the summary table
		depths := []int{3, 5, 10, 15, 20, 25, 30, 50, 75, 100, 200, 500}
		// Insert the threshold depth if it's not already in the list
		if thresholdDepth > 0 {
			inserted := false
			for i, d := range depths {
				if d == thresholdDepth {
					inserted = true
					break
				}
				if d > thresholdDepth {
					depths = append(depths[:i+1], depths[i:]...)
					depths[i] = thresholdDepth
					inserted = true
					break
				}
			}
			if !inserted {
				depths = append(depths, thresholdDepth)
			}
		}

		tw := tablewriter.New(
			tablewriter.Col("Depth", tablewriter.RightAlign()),
			tablewriter.Col("Epoch", tablewriter.RightAlign()),
			tablewriter.Col("Reorg Probability", tablewriter.RightAlign()),
			tablewriter.Col("Status"),
		)

		for _, depth := range depths {
			if depth > currentEpoch {
				continue
			}
			prob := ecfinality.CalcValidatorProb(chain, finality, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-depth)

			status := "above target"
			if prob < guarantee {
				status = "below target"
			}
			if depth == thresholdDepth {
				status = "<-- threshold"
			}

			probStr := fmt.Sprintf("%.3e", prob)
			if prob >= 1.0 {
				probStr = "1.000"
			}
			depthStr := fmt.Sprintf("%d (%s)", depth, epochToTime(depth))

			tw.Write(map[string]interface{}{
				"Depth":             depthStr,
				"Epoch":             headEpoch - depth,
				"Reorg Probability": probStr,
				"Status":            status,
			})
		}
		if err := tw.Flush(out, tablewriter.WithBorders()); err != nil {
			return err
		}

		return nil
	},
}
