package build

type ConsensusType int

const (
	FilecoinEC ConsensusType = 0
	Mir                      = 1
)

func IsMirConsensus() bool {
	return Consensus == Mir
}
