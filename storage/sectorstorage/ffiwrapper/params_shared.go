package ffiwrapper

// /////
// Proofs

// 1 / n
const SectorChallengeRatioDiv = 25

const MaxFallbackPostChallengeCount = 10

// extracted from lotus/chain/types/blockheader
func ElectionPostChallengeCount(sectors uint64, faults uint64) uint64 {
	if sectors-faults == 0 {
		return 0
	}
	// ceil(sectors / SectorChallengeRatioDiv)
	return (sectors-faults-1)/SectorChallengeRatioDiv + 1
}
