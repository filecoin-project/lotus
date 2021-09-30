package sealiface

import (
	"time"

	"github.com/filecoin-project/go-state-types/abi"
)

// this has to be in a separate package to not make lotus API depend on filecoin-ffi

type Config struct {
	// 0 = no limit
	MaxWaitDealsSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectors uint64

	// includes failed, 0 = no limit
	MaxSealingSectorsForDeals uint64

	WaitDealsDelay time.Duration

	CommittedCapacitySectorLifetime time.Duration

	StartEpochSealingBuffer abi.ChainEpoch

	AlwaysKeepUnsealedCopy bool

	FinalizeEarly bool

	CollateralFromMinerBalance bool
	AvailableBalanceBuffer     abi.TokenAmount
	DisableCollateralFallback  bool

	BatchPreCommits     bool
	MaxPreCommitBatch   int
	PreCommitBatchWait  time.Duration
	PreCommitBatchSlack time.Duration

	AggregateCommits bool
	MinCommitBatch   int
	MaxCommitBatch   int
	CommitBatchWait  time.Duration
	CommitBatchSlack time.Duration

	AggregateAboveBaseFee      abi.TokenAmount
	BatchPreCommitAboveBaseFee abi.TokenAmount

	TerminateBatchMax  uint64
	TerminateBatchMin  uint64
	TerminateBatchWait time.Duration
}
