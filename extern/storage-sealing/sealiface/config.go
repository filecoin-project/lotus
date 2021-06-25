package sealiface

import "time"

// this has to be in a separate package to not make lotus API depend on filecoin-ffi

type Config struct {
	// 0 = no limit
	MaxWaitDealsSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectorsForDeals uint64

	WaitDealsDelay time.Duration

	AlwaysKeepUnsealedCopy bool

	FinalizeEarly bool

	BatchPreCommits     bool
	MaxPreCommitBatch   int
	PreCommitBatchWait  time.Duration
	PreCommitBatchSlack time.Duration

	AggregateCommits bool
	MinCommitBatch   int
	MaxCommitBatch   int
	CommitBatchWait  time.Duration
	CommitBatchSlack time.Duration

	TerminateBatchMax  uint64
	TerminateBatchMin  uint64
	TerminateBatchWait time.Duration
}
