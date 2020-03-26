package ffiwrapper

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("ffiwrapper")

type SectorBuilder struct {
	sealProofType abi.RegisteredProof
	postProofType abi.RegisteredProof
	ssize         abi.SectorSize // a function of sealProofType and postProofType

	sectors  SectorProvider
	stopping chan struct{}
}

func fallbackPostChallengeCount(sectors uint64, faults uint64) uint64 {
	challengeCount := ElectionPostChallengeCount(sectors, faults)
	if challengeCount > MaxFallbackPostChallengeCount {
		return MaxFallbackPostChallengeCount
	}
	return challengeCount
}

func (sb *SectorBuilder) Stop() {
	close(sb.stopping)
}

func (sb *SectorBuilder) SectorSize() abi.SectorSize {
	return sb.ssize
}

func (sb *SectorBuilder) SealProofType() abi.RegisteredProof {
	return sb.sealProofType
}

func (sb *SectorBuilder) PoStProofType() abi.RegisteredProof {
	return sb.postProofType
}
