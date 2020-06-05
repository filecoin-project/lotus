package sealing

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

// Epochs
const SealRandomnessLookback = miner.ChainFinalityish

// Epochs
func SealRandomnessLookbackLimit(spt abi.RegisteredProof) abi.ChainEpoch {
	return miner.MaxSealDuration[spt]
}

// Epochs
const InteractivePoRepConfidence = 6
