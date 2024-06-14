package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/dreading/gospecfunc/bessel"
	"github.com/filecoin-project/lotus/build"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/constraints"
	"gonum.org/v1/gonum/stat/distuv"
)

var finalityCmd = &cli.Command{
	Name:        "finality-calculator",
	Description: "Calculate the finality probability of at a tipset",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
		},
		&cli.StringFlag{
			Name: "input",
		},
	},
	ArgsUsage: "[inputFile]",
	Action: func(cctx *cli.Context) error {
		input := cctx.Args().Get(0)
		file, err := os.Open(input)
		if err != nil {
			return err
		}
		defer file.Close()

		var chain []int
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

		blocksPerEpoch := 5.0            // Expected number of blocks per epoch
		byzantineFraction := 0.3         // Upper bound on the fraction of malicious nodes in the network
		currentEpoch := len(chain) - 1   // Current epoch (end of history)
		targetEpoch := currentEpoch - 30 // Target epoch for which finality is calculated

		finality := FinalityCalcValidator(chain, blocksPerEpoch, byzantineFraction, currentEpoch, targetEpoch)

		fmt.Fprintf(cctx.App.Writer, "Finality probability: %f\n", finality)

		return nil
	},
}

// FinalityCalcValidator computes the probability that a previous blockchain tipset gets replaced.
//
// Based on https://github.com/consensus-shipyard/ec-finality-calculator
func FinalityCalcValidator(chain []int, blocksPerEpoch float64, byzantineFraction float64, currentEpoch int, targetEpoch int) float64 {
	// Threshold at which the probability of an event is considered negligible
	const negligibleThreshold = 1e-25

	maxKL := 400                                                     // Max k for which to calculate Pr(L=k)
	maxKB := int((currentEpoch - targetEpoch) * int(blocksPerEpoch)) // Max k for which to calculate Pr(B=k)
	maxKM := 400                                                     // Max k for which to calculate Pr(M=k)
	maxIM := 100                                                     // Maximum number of epochs for the calculation (after which the pr become negligible)

	rateMaliciousBlocks := blocksPerEpoch * byzantineFraction // upper bound
	rateHonestBlocks := blocksPerEpoch - rateMaliciousBlocks  // lower bound

	// Compute L
	prL := make([]float64, maxKL+1)

	for k := 0; k <= maxKL; k++ {
		sumExpectedAdversarialBlocksI := 0.0
		sumChainBlocksI := 0

		for i := targetEpoch; i > currentEpoch-int(build.Finality); i-- {
			sumExpectedAdversarialBlocksI += rateMaliciousBlocks
			sumChainBlocksI += chain[i-1]
			// Poisson(k=k, lambda=sum(f*e))
			prLi := distuv.Poisson{Lambda: sumExpectedAdversarialBlocksI}.Prob(float64(k + sumChainBlocksI))
			prL[k] = math.Max(prL[k], prLi)

			// Break if prL[k] becomes negligible
			if k > 1 && prL[k] < negligibleThreshold && prL[k] < prL[k-1] {
				maxKL = k
				prL = prL[:k+1]
				break
			}
		}
	}

	// As the adversarial lead is never negative, the missing probability is added to k=0
	prL[0] += 1 - sum(prL)

	// Compute B
	prB := make([]float64, maxKB+1)

	// Calculate Pr(B=k) for each value of k
	for k := 0; k <= maxKB; k++ {
		prB[k] = distuv.Poisson{Lambda: float64(currentEpoch-targetEpoch) * rateMaliciousBlocks}.Prob(float64(k))

		// Break if prB[k] becomes negligible
		if k > 1 && prB[k] < negligibleThreshold && prB[k] < prB[k-1] {
			maxKB = k
			prB = prB[:k+1]
			break
		}
	}

	// Compute M
	prHgt0 := 1 - distuv.Poisson{Lambda: rateHonestBlocks}.Prob(0)

	expZ := 0.0
	for k := 0; k < int(4*blocksPerEpoch); k++ {
		pmf := distuv.Poisson{Lambda: rateMaliciousBlocks}.Prob(float64(k))
		expZ += ((rateHonestBlocks + float64(k)) / math.Pow(2, float64(k))) * pmf
	}

	ratePublicChain := prHgt0 * expZ

	prM := make([]float64, maxKM+1)
	for k := 0; k <= maxKM; k++ {
		for i := maxIM; i > 0; i-- {
			probMI := SkellamPMF(k, float64(i)*rateMaliciousBlocks, float64(i)*ratePublicChain)

			// Break if probMI becomes negligible
			if probMI < negligibleThreshold && probMI < prM[k] {
				break
			}
			prM[k] = math.Max(prM[k], probMI)
		}

		// Break if prM[k] becomes negligible
		if k > 1 && prM[k] < negligibleThreshold && prM[k] < prM[k-1] {
			maxKM = k
			prM = prM[:k+1]
			break
		}
	}

	prM[0] += 1 - sum(prM)

	// Compute error probability upper bound
	cumsumL := cumsum(prL)
	cumsumB := cumsum(prB)
	cumsumM := cumsum(prM)

	k := sum(chain[targetEpoch:currentEpoch])

	sumLgeK := cumsumL[len(cumsumL)-1]
	if k > 0 {
		sumLgeK -= cumsumL[min(k-1, maxKL)]
	}

	doubleSum := 0.0

	for l := 0; l < k; l++ {
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

	return math.Min(prError, 1.0)
}

func sum[T constraints.Integer | constraints.Float](s []T) T {
	var total T
	for _, v := range s {
		total += v
	}
	return total
}

func cumsum(arr []float64) []float64 {
	cumsums := make([]float64, len(arr))
	cumSum := 0.0
	for i, value := range arr {
		cumSum += value
		cumsums[i] = cumSum
	}
	return cumsums
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SkellamPMF calculates the probability mass function (PMF) of a Skellam distribution.
//
// The Skellam distribution is the probability distribution of the difference
// of two independent Poisson random variables.
//
// Arguments:
// * k - The difference of two Poisson random variables.
// * mu1 - The expected value of the first Poisson distribution.
// * mu2 - The expected value of the second Poisson distribution.
//
// Returns:
// * A float64 representing the PMF of the Skellam distribution at k.
func SkellamPMF(k int, mu1 float64, mu2 float64) float64 {
	// Based on https://github.com/jsoares/rusty-skellam/blob/main/src/lib.rs

	// Return NaN if parameters outside range
	if math.IsNaN(mu1) || mu1 <= 0 || math.IsNaN(mu2) || mu2 <= 0 {
		return math.NaN()
	}

	// Parameterise and compute the Modified Bessel function of the first kind
	nu := float64(k)
	z := complex(2.0*math.Sqrt(mu1*mu2), 0)
	besselResult := bessel.I(nu, z)

	// Compute the pmf
	return math.Exp(-(mu1 + mu2)) * math.Pow(mu1/mu2, nu/2.0) * real(besselResult)
}

/*
func main() {
	seed := rand.NewSource(1)
	random := rand.New(seed)
	chain := make([]int, 1000)
	for i := range chain {
		chain[i] = random.Intn(5) + 1
	}

	errorProbability := FinalityCalcValidator(chain, 5.0, 0.3, 1000, 900)
	fmt.Printf("Error probability: %f\n", errorProbability)
}
*/
