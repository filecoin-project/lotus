package vm

const (
	gasOveruseNum   = 3
	gasOveruseDenom = 10
)

// ComputeGasOutputs computes amount of gas to be refunded and amount of gas to be burned
// Result is (refund, burn)
func ComputeGasOutputs(gasUsed, gasLimit int64) (int64, int64) {
	allowedGasOverUsed := gasUsed + (gasUsed*gasOveruseNum)/gasOveruseDenom
	gasToBurn := gasLimit - allowedGasOverUsed
	if gasToBurn < 0 {
		gasToBurn = 0
	}

	return gasLimit - gasUsed - gasToBurn, gasToBurn
}
