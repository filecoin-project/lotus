package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"

	skellampmf "github.com/rvagg/go-skellam-pmf"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

const (
	// recentHealthWindow is the number of recent epochs used to compute average
	// blocks/epoch for the chain health display.
	recentHealthWindow = 30

	// bisectLow and bisectHigh define the search range for the bisect algorithm
	// that finds the epoch depth at which the finality guarantee is met. A low
	// bound of 3 avoids evaluating trivially shallow depths; a high bound of
	// 200 accommodates degraded chains that take longer to finalize.
	bisectLow  = 3
	bisectHigh = 200
)

var finalityCmd = &cli.Command{
	Name:  "finality-calculator",
	Usage: "Calculate the EC finality probability of a tipset",
	Description: `Compute the probability that a previous blockchain tipset gets replaced,
based on FRC-0089 (https://github.com/filecoin-project/FIPs/blob/master/FRCs/frc-0089.md).

Under healthy network conditions (tipsets with ~5 blocks), the 2^-30 error guarantee
is typically achieved within ~30 epochs (~15 minutes), compared to the static 900-epoch
(~7.5 hour) EC finality assumption.

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
			Value: 5.0,
			Usage: "Expected number of blocks per epoch (Filecoin mainnet protocol constant is 5). Changing this models a different expected block production rate.",
		},
		&cli.Float64Flag{
			Name:  "byzantine-fraction",
			Value: 0.3,
			Usage: "Assumed upper bound on adversarial mining power as a fraction (0.0-1.0). The standard Filecoin security assumption is 0.3 (30%). Lower values model a more secure network; higher values model stronger adversaries.",
		},
		&cli.IntFlag{
			Name:  "safety-exponent",
			Value: -30,
			Usage: "Safety target as a power of 2 (e.g. -30 means 2^-30). This is the maximum acceptable probability of a tipset being replaced. The Filecoin standard is -30, which the 900-epoch static finality was designed to achieve.",
		},
		&cli.BoolFlag{
			Name:  "csv",
			Usage: "Output raw CSV (epoch,depth,error_probability) for all epochs back to EC finality, suitable for piping to other tools or plotting.",
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
			_, _ = fmt.Fprintln(out, "epoch,depth,error_probability")
			for i := range min(int(policy.ChainFinality), currentEpoch) {
				prob := FinalityCalcValidator(chain, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-i)
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
		thresholdDepth := -1
		low, high := bisectLow, min(bisectHigh, currentEpoch)

		if low >= high {
			// Chain too short for bisect search
			thresholdDepth = -1
		} else if probLow := FinalityCalcValidator(chain, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-low); probLow < guarantee {
			thresholdDepth = low
		} else {
			probHigh := FinalityCalcValidator(chain, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-high)
			if probHigh > guarantee {
				thresholdDepth = -1
			} else {
				for low < high {
					mid := (low + high) / 2
					prob := FinalityCalcValidator(chain, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-mid)
					if prob < guarantee {
						high = mid
					} else {
						low = mid + 1
					}
				}
				thresholdDepth = low
			}
		}
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
			_, _ = fmt.Fprintf(out, "  Probabilistic finality target (2^%d) NOT reached within %d epochs (chain may be unhealthy)\n", safetyExponent, bisectHigh)
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
			tablewriter.Col("Error Probability", tablewriter.RightAlign()),
			tablewriter.Col("Status"),
		)

		for _, depth := range depths {
			if depth > currentEpoch {
				continue
			}
			prob := FinalityCalcValidator(chain, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-depth)

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
				"Error Probability": probStr,
				"Status":            status,
			})
		}
		if err := tw.Flush(out, tablewriter.WithBorders()); err != nil {
			return err
		}

		return nil
	},
}

// FinalityCalcValidator computes the probability that a previous blockchain tipset gets
// replaced. This is a Go port of the Python reference implementation from FRC-0089:
// https://github.com/consensus-shipyard/ec-finality-calculator (finality_calc_validator.py)
//
// Parameters:
//   - chain: block counts per epoch (index 0 = earliest epoch)
//   - blocksPerEpoch: expected blocks per epoch (5 for mainnet)
//   - byzantineFraction: upper bound on adversarial power fraction (e.g. 0.3)
//   - currentEpoch: index into chain for the current epoch
//   - targetEpoch: index into chain for the epoch being evaluated
func FinalityCalcValidator(chain []int, blocksPerEpoch float64, byzantineFraction float64, currentEpoch int, targetEpoch int) float64 {
	const negligibleThreshold = 1e-25

	maxKL := 400
	maxKB := (currentEpoch - targetEpoch) * int(blocksPerEpoch)
	maxKM := 400
	maxIM := 100

	rateMaliciousBlocks := blocksPerEpoch * byzantineFraction
	rateHonestBlocks := blocksPerEpoch - rateMaliciousBlocks

	// Compute L: adversarial lead distribution at target epoch
	prL := make([]float64, maxKL+1)

	for k := 0; k <= maxKL; k++ {
		sumExpectedAdversarialBlocksI := 0.0
		sumChainBlocksI := 0

		for i := targetEpoch; i > max(0, currentEpoch-int(policy.ChainFinality)); i-- {
			sumExpectedAdversarialBlocksI += rateMaliciousBlocks
			sumChainBlocksI += chain[i-1]
			prLi := poissonProb(sumExpectedAdversarialBlocksI, float64(k+sumChainBlocksI))
			prL[k] = max(prL[k], prLi)
		}
		if k > 1 && prL[k] < negligibleThreshold && prL[k] < prL[k-1] {
			maxKL = k
			prL = prL[:k+1]
			break
		}
	}

	prL[0] += 1 - sumFloat64(prL)

	// Compute B: adversarial blocks during settlement period
	prB := make([]float64, maxKB+1)

	for k := 0; k <= maxKB; k++ {
		prB[k] = poissonProb(float64(currentEpoch-targetEpoch)*rateMaliciousBlocks, float64(k))

		if k > 1 && prB[k] < negligibleThreshold && prB[k] < prB[k-1] {
			maxKB = k
			prB = prB[:k+1]
			break
		}
	}

	// Compute M: adversarial mining advantage in the future (Skellam distribution)
	prHgt0 := 1 - poissonProb(rateHonestBlocks, 0)

	expZ := 0.0
	for k := 0; k < int(4*blocksPerEpoch); k++ {
		pmf := poissonProb(rateMaliciousBlocks, float64(k))
		expZ += ((rateHonestBlocks + float64(k)) / math.Pow(2, float64(k))) * pmf
	}

	ratePublicChain := prHgt0 * expZ

	prM := make([]float64, maxKM+1)
	for k := 0; k <= maxKM; k++ {
		for i := maxIM; i > 0; i-- {
			probMI := skellampmf.SkellamPMF(k, float64(i)*rateMaliciousBlocks, float64(i)*ratePublicChain)

			if probMI < negligibleThreshold && probMI < prM[k] {
				break
			}
			prM[k] = max(prM[k], probMI)
		}

		if k > 1 && prM[k] < negligibleThreshold && prM[k] < prM[k-1] {
			maxKM = k
			prM = prM[:k+1]
			break
		}
	}

	prM[0] += 1 - sumFloat64(prM)

	// Compute error probability upper bound via convolution
	cumsumL := cumsum(prL)
	cumsumB := cumsum(prB)
	cumsumM := cumsum(prM)

	k := sumInt(chain[targetEpoch:currentEpoch])

	sumLgeK := cumsumL[len(cumsumL)-1]
	if k > 0 {
		sumLgeK -= cumsumL[min(k-1, maxKL)]
	}

	doubleSum := 0.0

	for l := range k {
		sumBgeKminL := cumsumB[len(cumsumB)-1]
		if k-l-1 > 0 {
			sumBgeKminL -= cumsumB[min(k-l-1, maxKB)]
		}
		doubleSum += prL[min(l, maxKL)] * sumBgeKminL

		for b := 0; b < k-l; b++ {
			sumMgeKminLminB := cumsumM[len(cumsumM)-1]
			if k-l-b-1 > 0 {
				sumMgeKminLminB -= cumsumM[min(k-l-b-1, maxKM)]
			}
			doubleSum += prL[min(l, maxKL)] * prB[min(b, maxKB)] * sumMgeKminLminB
		}
	}

	prError := sumLgeK + doubleSum

	return min(prError, 1.0)
}

func poissonProb(lambda float64, x float64) float64 {
	return math.Exp(poissonLogProb(lambda, x))
}

func poissonLogProb(lambda float64, x float64) float64 {
	if x < 0 || math.Floor(x) != x {
		return math.Inf(-1)
	}
	if lambda == 0 {
		if x == 0 {
			return 0 // P(X=0 | lambda=0) = 1, log(1) = 0
		}
		return math.Inf(-1)
	}
	lg, _ := math.Lgamma(math.Floor(x) + 1)
	return x*math.Log(lambda) - lambda - lg
}

func sumFloat64(s []float64) float64 {
	var total float64
	for _, v := range s {
		total += v
	}
	return total
}

func sumInt(s []int) int {
	var total int
	for _, v := range s {
		total += v
	}
	return total
}

func cumsum(arr []float64) []float64 {
	result := make([]float64, len(arr))
	var s float64
	for i, v := range arr {
		s += v
		result[i] = s
	}
	return result
}
