package sealing

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

// Epochs
const SealRandomnessLookback = miner.ChainFinality

// Epochs
func SealRandomnessLookbackLimit(spt abi.RegisteredSealProof) abi.ChainEpoch {
	return miner.MaxSealDuration[spt]
}

// Epochs
const InteractivePoRepConfidence = 6
