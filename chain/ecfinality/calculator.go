// Package ecfinality implements the FRC-0089 EC finality calculator.
//
// The calculator computes an upper bound on the probability that a confirmed
// tipset could be reorganized out of the canonical chain by an adversarial
// fork, using observed chain data (block counts per epoch). Under healthy
// network conditions (~5 blocks/epoch), the 2^-30 finality guarantee
// (roughly one-in-a-billion chance of reorg) is typically achieved within
// ~30 epochs (~15 minutes), compared to the static 900-epoch (~7.5 hour)
// EC finality assumption which is based on worst-case network conditions.
//
// Reference: https://github.com/filecoin-project/FIPs/blob/master/FRCs/frc-0089.md
// Python reference: https://github.com/consensus-shipyard/ec-finality-calculator
package ecfinality

import (
	"math"

	skellampmf "github.com/rvagg/go-skellam-pmf"
)

const (
	// BisectLow and BisectHigh define the search range for the bisect algorithm
	// that finds the epoch depth at which the finality guarantee is met. A low
	// bound of 3 avoids evaluating trivially shallow depths; a high bound of
	// 200 accommodates degraded chains that take longer to finalize.
	BisectLow  = 3
	BisectHigh = 200

	// DefaultBlocksPerEpoch is the Filecoin mainnet expected block production rate.
	DefaultBlocksPerEpoch = 5.0

	// DefaultByzantineFraction is the standard Filecoin security assumption for
	// adversarial mining power.
	DefaultByzantineFraction = 0.3

	// DefaultSafetyExponent is the target reorg probability as a power of 2.
	// 2^-30 (~one-in-a-billion) is the standard Filecoin finality guarantee.
	DefaultSafetyExponent = -30
)

// CalcValidatorProb computes the upper-bound probability that a confirmed
// tipset could be reorganized out of the canonical chain. This is a Go port
// of the Python reference implementation from FRC-0089
// (finality_calc_validator.py).
//
// Parameters:
//   - chain: block counts per epoch (index 0 = earliest epoch)
//   - finality: lookback depth for the L distribution (900 on mainnet)
//   - blocksPerEpoch: expected blocks per epoch (5 for mainnet)
//   - byzantineFraction: upper bound on adversarial power fraction (e.g. 0.3)
//   - currentEpoch: index into chain for the current epoch
//   - targetEpoch: index into chain for the epoch being evaluated
func CalcValidatorProb(chain []int, finality int, blocksPerEpoch float64, byzantineFraction float64, currentEpoch int, targetEpoch int) float64 {
	if currentEpoch <= targetEpoch || targetEpoch < 0 || currentEpoch >= len(chain) {
		return 1.0
	}

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

		for i := targetEpoch; i > max(0, currentEpoch-finality); i-- {
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

	// Compute reorg probability upper bound via convolution
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

// FindThresholdDepth performs a bisect search to find the shallowest depth at
// which the reorg probability drops below the given guarantee. Returns -1 if
// the guarantee is not met within the search range.
func FindThresholdDepth(chain []int, finality int, blocksPerEpoch float64, byzantineFraction float64, guarantee float64) int {
	currentEpoch := len(chain) - 1
	low, high := BisectLow, min(BisectHigh, currentEpoch)

	if low >= high {
		return -1
	}

	probLow := CalcValidatorProb(chain, finality, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-low)
	if probLow < guarantee {
		return low
	}

	probHigh := CalcValidatorProb(chain, finality, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-high)
	if probHigh > guarantee {
		return -1
	}

	for low < high {
		mid := (low + high) / 2
		prob := CalcValidatorProb(chain, finality, blocksPerEpoch, byzantineFraction, currentEpoch, currentEpoch-mid)
		if prob < guarantee {
			high = mid
		} else {
			low = mid + 1
		}
	}
	return low
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
