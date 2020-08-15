package builders

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/oni/tvx/lotus"
)

const (
	overuseNum = 11
	overuseDen = 10
)

// CalculateDeduction returns the balance that shall be deducted from the
// sender's account as a result of applying this message.
func CalculateDeduction(am *ApplicableMessage) big.Int {
	m := am.Message

	minerReward := GetMinerReward(m.GasLimit, m.GasPremium) // goes to the miner
	burn := CalculateBurn(m.GasLimit, am.Result.GasUsed)    // vanishes
	deducted := big.Add(minerReward, burn)                  // sum of gas accrued

	if am.Result.ExitCode.IsSuccess() {
		deducted = big.Add(deducted, m.Value) // message value
	}
	return deducted
}

// GetMinerReward returns the amount that the miner gets to keep, which is
func GetMinerReward(gasLimit int64, gasPremium abi.TokenAmount) abi.TokenAmount {
	return big.Mul(big.NewInt(gasLimit), gasPremium)
}

func GetMinerPenalty(gasLimit int64) big.Int {
	return big.Mul(lotus.BaseFee, big.NewInt(gasLimit))
}

// CalculateBurn calcualtes the amount that will be burnt, a function of the
// gas limit and the gas actually used.
func CalculateBurn(gasLimit int64, gasUsed int64) big.Int {
	over := gasLimit - (overuseNum*gasUsed)/overuseDen
	if over < 0 {
		over = 0
	}
	if over > gasUsed {
		over = gasUsed
	}

	overestimateGas := big.NewInt(gasLimit - gasUsed)
	overestimateGas = big.Mul(overestimateGas, big.NewInt(over))
	overestimateGas = big.Div(overestimateGas, big.NewInt(gasUsed))

	totalBurnGas := big.Add(overestimateGas, big.NewInt(gasUsed))
	return big.Mul(lotus.BaseFee, totalBurnGas)
}
