package vm

import (
	"math/big"
)

const (
	gasOveruseNum   = 11
	gasOveruseDenom = 10
)

// ComputeGasOutputs computes amount of gas to be refunded and amount of gas to be burned
// Result is (refund, burn)
func ComputeGasOutputs(gasUsed, gasLimit int64) (int64, int64) {
	if gasUsed == 0 {
		return 0, gasLimit
	}

	// over = gasLimit/gasUsed - 1 - 0.3
	// over = min(over, 1)
	// gasToBurn = (gasLimit - gasUsed) * over

	// so to factor out division from `over`
	// over*gasUsed = min(gasLimit - (13*gasUsed)/10, gasUsed)
	// gasToBurn = ((gasLimit - gasUsed)*over*gasUsed) / gasUsed
	over := gasLimit - (gasOveruseNum*gasUsed)/gasOveruseDenom
	if over < 0 {
		return gasLimit - gasUsed, 0
	}

	// if we want sharper scaling it goes here:
	// over *= 2

	if over > gasUsed {
		over = gasUsed
	}

	// needs bigint, as it overflows in pathological case gasLimit > 2^32 gasUsed = gasLimit / 2
	gasToBurn := big.NewInt(gasLimit - gasUsed)
	gasToBurn = gasToBurn.Mul(gasToBurn, big.NewInt(over))
	gasToBurn = gasToBurn.Div(gasToBurn, big.NewInt(gasUsed))

	return gasLimit - gasUsed - gasToBurn.Int64(), gasToBurn.Int64()
}
