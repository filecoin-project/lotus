package vm

import (
	"context"
	"os"
	"strconv"
	"sync"

	"github.com/ipfs/go-cid"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
)

const (
	// DefaultAvailableExecutionLanes is the number of available execution lanes; it is the bound of
	// concurrent active executions.
	// This is the default value in filecoin-ffi
	DefaultAvailableExecutionLanes = 4
	// DefaultPriorityExecutionLanes is the number of reserved execution lanes for priority computations.
	// This is purely userspace, but we believe it is a reasonable default, even with more available
	// lanes.
	DefaultPriorityExecutionLanes = 2
)

// the execution environment; see below for definition, methods, and initialization
var execution *executionEnv

// implementation of vm executor with simple sanity check preventing use after free.
type vmExecutor struct {
	vmi  Interface
	lane ExecutionLane
}

var _ Interface = (*vmExecutor)(nil)

func newVMExecutor(vmi Interface, lane ExecutionLane) Interface {
	return &vmExecutor{vmi: vmi, lane: lane}
}

func (e *vmExecutor) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error) {
	token := execution.getToken(ctx, e.lane)
	defer token.Done()

	return e.vmi.ApplyMessage(ctx, cmsg)
}

func (e *vmExecutor) ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error) {
	token := execution.getToken(ctx, e.lane)
	defer token.Done()

	return e.vmi.ApplyImplicitMessage(ctx, msg)
}

func (e *vmExecutor) Flush(ctx context.Context) (cid.Cid, error) {
	return e.vmi.Flush(ctx)
}

type executionToken struct {
	lane     ExecutionLane
	reserved int
	ctx      context.Context
}

func (token *executionToken) Done() {
	execution.putToken(token)
}

type executionEnv struct {
	mx   *sync.Mutex
	cond *sync.Cond

	// available executors
	available int
	// reserved executors
	reserved int
}

func (e *executionEnv) getToken(ctx context.Context, lane ExecutionLane) *executionToken {
	metricsUp(ctx, metrics.VMExecutionWaiting, lane)
	defer metricsDown(ctx, metrics.VMExecutionWaiting, lane)

	e.mx.Lock()

	reserving := 0
	if lane == ExecutionLaneDefault {
		for e.available <= e.reserved {
			e.cond.Wait()
		}

	} else {
		for e.available == 0 {
			e.cond.Wait()
		}
		if e.reserved > 0 {
			e.reserved--
			reserving = 1
		}
	}

	e.available--
	e.mx.Unlock()

	metricsUp(ctx, metrics.VMExecutionRunning, lane)
	return &executionToken{lane: lane, reserved: reserving, ctx: ctx}
}

func (e *executionEnv) putToken(token *executionToken) {
	e.mx.Lock()

	e.available++
	e.reserved += token.reserved

	// Note: Signal is unsound, because a priority token could wake up a non-priority
	// goroutine and lead to deadlock. So Broadcast it must be.
	e.cond.Broadcast()
	e.mx.Unlock()

	metricsDown(token.ctx, metrics.VMExecutionRunning, token.lane)
}

func metricsUp(ctx context.Context, metric *stats.Int64Measure, lane ExecutionLane) {
	metricsAdjust(ctx, metric, lane, 1)
}

func metricsDown(ctx context.Context, metric *stats.Int64Measure, lane ExecutionLane) {
	metricsAdjust(ctx, metric, lane, -1)
}

var (
	defaultLaneTag  = tag.Upsert(metrics.ExecutionLane, "default")
	priorityLaneTag = tag.Upsert(metrics.ExecutionLane, "priority")
)

func metricsAdjust(ctx context.Context, metric *stats.Int64Measure, lane ExecutionLane, delta int) {
	laneTag := defaultLaneTag
	if lane > ExecutionLaneDefault {
		laneTag = priorityLaneTag
	}

	ctx, _ = tag.New(ctx, laneTag)
	stats.Record(ctx, metric.M(int64(delta)))
}

func init() {
	var err error

	available := DefaultAvailableExecutionLanes
	if concurrency := os.Getenv("LOTUS_FVM_CONCURRENCY"); concurrency != "" {
		available, err = strconv.Atoi(concurrency)
		if err != nil {
			panic(err)
		}
	}

	priority := DefaultPriorityExecutionLanes
	if reserved := os.Getenv("LOTUS_FVM_CONCURRENCY_RESERVED"); reserved != "" {
		priority, err = strconv.Atoi(reserved)
		if err != nil {
			panic(err)
		}
	}

	// some sanity checks
	if available < 2 {
		panic("insufficient execution concurrency")
	}

	if available <= priority {
		panic("insufficient default execution concurrency")
	}

	mx := &sync.Mutex{}
	cond := sync.NewCond(mx)

	execution = &executionEnv{
		mx:        mx,
		cond:      cond,
		available: available,
		reserved:  priority,
	}
}
