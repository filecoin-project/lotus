package types

// FinalityStatus describes how the node is currently determining finality,
// combining probabilistic EC finality (based on observed chain health) with
// F3 fast finality when available.
type FinalityStatus struct {
	// ECFinalityThresholdDepth is the shallowest epoch depth at which the
	// probability of a chain reorganization drops below 2^-30 (~one in a
	// billion). A value of -1 indicates the threshold was not met within the
	// search range, which suggests degraded chain health.
	ECFinalityThresholdDepth int `json:"ecFinalityThresholdDepth"`

	// ECFinalizedTipSet is the most recent tipset where the reorg probability
	// is below 2^-30, based on observed block production. Nil if the
	// threshold is not met.
	ECFinalizedTipSet *TipSet `json:"ecFinalizedTipSet"`

	// F3FinalizedTipSet is the tipset finalized by F3 (Fast Finality), if F3
	// is running and has issued a certificate. Nil if F3 is not available.
	F3FinalizedTipSet *TipSet `json:"f3FinalizedTipSet"`

	// FinalizedTipSet is the overall finalized tipset used by the node,
	// taking the most recent of F3 and EC calculator results.
	FinalizedTipSet *TipSet `json:"finalizedTipSet"`

	// Head is the current chain head used for the computation.
	Head *TipSet `json:"head"`
}
