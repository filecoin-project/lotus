package stages

// DefaultPipeline returns the default stage pipeline. This pipeline.
//
// 1. Funds a "funding" actor, if necessary.
// 2. Submits any ready window posts.
// 3. Submits any ready prove commits.
// 4. Submits pre-commits with the remaining gas.
func DefaultPipeline() ([]Stage, error) {
	// TODO: make this configurable. E.g., through DI?
	// Ideally, we'd also be able to change priority, limit throughput (by limiting gas in the
	// block builder, etc.
	funding, err := NewFundingStage()
	if err != nil {
		return nil, err
	}
	wdpost, err := NewWindowPoStStage()
	if err != nil {
		return nil, err
	}
	provecommit, err := NewProveCommitStage(funding)
	if err != nil {
		return nil, err
	}
	precommit, err := NewPreCommitStage(funding, provecommit)
	if err != nil {
		return nil, err
	}

	return []Stage{funding, wdpost, provecommit, precommit}, nil
}
