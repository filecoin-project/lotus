package metrics

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	rpcmetrics "github.com/filecoin-project/go-jsonrpc/metrics"

	"github.com/filecoin-project/lotus/blockstore"
)

// Distribution
var defaultMillisecondsDistribution = view.Distribution(0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 3000, 4000, 5000, 7500, 10000, 20000, 50000, 100_000, 250_000, 500_000, 1000_000)
var workMillisecondsDistribution = view.Distribution(
	250, 500, 1000, 2000, 5000, 10_000, 30_000, 60_000, 2*60_000, 5*60_000, 10*60_000, 15*60_000, 30*60_000, // short sealing tasks
	40*60_000, 45*60_000, 50*60_000, 55*60_000, 60*60_000, 65*60_000, 70*60_000, 75*60_000, 80*60_000, 85*60_000, 100*60_000, 120*60_000, // PC2 / C2 range
	130*60_000, 140*60_000, 150*60_000, 160*60_000, 180*60_000, 200*60_000, 220*60_000, 260*60_000, 300*60_000, // PC1 range
	350*60_000, 400*60_000, 600*60_000, 800*60_000, 1000*60_000, 1300*60_000, 1800*60_000, 4000*60_000, 10000*60_000, // intel PC1 range
)

var queueSizeDistribution = view.Distribution(0, 1, 2, 3, 5, 7, 10, 15, 25, 35, 50, 70, 90, 130, 200, 300, 500, 1000, 2000, 5000, 10000)

// Global Tags
var (
	// common
	Version, _     = tag.NewKey("version")
	Commit, _      = tag.NewKey("commit")
	NodeType, _    = tag.NewKey("node_type")
	PeerID, _      = tag.NewKey("peer_id")
	MinerID, _     = tag.NewKey("miner_id")
	FailureType, _ = tag.NewKey("failure_type")

	// chain
	Local, _        = tag.NewKey("local")
	MessageFrom, _  = tag.NewKey("message_from")
	MessageTo, _    = tag.NewKey("message_to")
	MessageNonce, _ = tag.NewKey("message_nonce")
	ReceivedFrom, _ = tag.NewKey("received_from")
	MsgValid, _     = tag.NewKey("message_valid")
	Endpoint, _     = tag.NewKey("endpoint")
	APIInterface, _ = tag.NewKey("api") // to distinguish between gateway api and full node api endpoint calls

	// miner
	TaskType, _       = tag.NewKey("task_type")
	WorkerHostname, _ = tag.NewKey("worker_hostname")
	StorageID, _      = tag.NewKey("storage_id")
	SectorState, _    = tag.NewKey("sector_state")

	PathSeal, _    = tag.NewKey("path_seal")
	PathStorage, _ = tag.NewKey("path_storage")

	// rcmgr
	ServiceID, _  = tag.NewKey("svc")
	ProtocolID, _ = tag.NewKey("proto")
	Direction, _  = tag.NewKey("direction")
	UseFD, _      = tag.NewKey("use_fd")

	// vm execution
	ExecutionLane, _ = tag.NewKey("lane")
)

// Measures
var (
	// common
	LotusInfo          = stats.Int64("info", "Arbitrary counter to tag lotus info to", stats.UnitDimensionless)
	PeerCount          = stats.Int64("peer/count", "Current number of FIL peers", stats.UnitDimensionless)
	APIRequestDuration = stats.Float64("api/request_duration_ms", "Duration of API requests", stats.UnitMilliseconds)

	// graphsync

	GraphsyncReceivingPeersCount              = stats.Int64("graphsync/receiving_peers", "number of peers we are receiving graphsync data from", stats.UnitDimensionless)
	GraphsyncReceivingActiveCount             = stats.Int64("graphsync/receiving_active", "number of active receiving graphsync transfers", stats.UnitDimensionless)
	GraphsyncReceivingCountCount              = stats.Int64("graphsync/receiving_pending", "number of pending receiving graphsync transfers", stats.UnitDimensionless)
	GraphsyncReceivingTotalMemoryAllocated    = stats.Int64("graphsync/receiving_total_allocated", "amount of block memory allocated for receiving graphsync data", stats.UnitBytes)
	GraphsyncReceivingTotalPendingAllocations = stats.Int64("graphsync/receiving_pending_allocations", "amount of block memory on hold being received pending allocation", stats.UnitBytes)
	GraphsyncReceivingPeersPending            = stats.Int64("graphsync/receiving_peers_pending", "number of peers we can't receive more data from cause of pending allocations", stats.UnitDimensionless)

	GraphsyncSendingPeersCount              = stats.Int64("graphsync/sending_peers", "number of peers we are sending graphsync data to", stats.UnitDimensionless)
	GraphsyncSendingActiveCount             = stats.Int64("graphsync/sending_active", "number of active sending graphsync transfers", stats.UnitDimensionless)
	GraphsyncSendingCountCount              = stats.Int64("graphsync/sending_pending", "number of pending sending graphsync transfers", stats.UnitDimensionless)
	GraphsyncSendingTotalMemoryAllocated    = stats.Int64("graphsync/sending_total_allocated", "amount of block memory allocated for sending graphsync data", stats.UnitBytes)
	GraphsyncSendingTotalPendingAllocations = stats.Int64("graphsync/sending_pending_allocations", "amount of block memory on hold from sending pending allocation", stats.UnitBytes)
	GraphsyncSendingPeersPending            = stats.Int64("graphsync/sending_peers_pending", "number of peers we can't send more data to cause of pending allocations", stats.UnitDimensionless)

	// chain
	ChainNodeHeight                     = stats.Int64("chain/node_height", "Current Height of the node", stats.UnitDimensionless)
	ChainNodeHeightExpected             = stats.Int64("chain/node_height_expected", "Expected Height of the node", stats.UnitDimensionless)
	ChainNodeWorkerHeight               = stats.Int64("chain/node_worker_height", "Current Height of workers on the node", stats.UnitDimensionless)
	IndexerMessageValidationFailure     = stats.Int64("indexer/failure", "Counter for indexer message validation failures", stats.UnitDimensionless)
	IndexerMessageValidationSuccess     = stats.Int64("indexer/success", "Counter for indexer message validation successes", stats.UnitDimensionless)
	MessagePending                      = stats.Int64("message/pending", "Current number of pending messages", stats.UnitDimensionless)
	MessagePublished                    = stats.Int64("message/published", "Counter for total locally published messages", stats.UnitDimensionless)
	MessageReceived                     = stats.Int64("message/received", "Counter for total received messages", stats.UnitDimensionless)
	MessageValidationFailure            = stats.Int64("message/failure", "Counter for message validation failures", stats.UnitDimensionless)
	MessageValidationSuccess            = stats.Int64("message/success", "Counter for message validation successes", stats.UnitDimensionless)
	MessageValidationDuration           = stats.Float64("message/validation_ms", "Duration of message validation", stats.UnitMilliseconds)
	MpoolGetNonceDuration               = stats.Float64("mpool/getnonce_ms", "Duration of getStateNonce in mpool", stats.UnitMilliseconds)
	MpoolGetBalanceDuration             = stats.Float64("mpool/getbalance_ms", "Duration of getStateBalance in mpool", stats.UnitMilliseconds)
	MpoolAddTsDuration                  = stats.Float64("mpool/addts_ms", "Duration of addTs in mpool", stats.UnitMilliseconds)
	MpoolAddDuration                    = stats.Float64("mpool/add_ms", "Duration of Add in mpool", stats.UnitMilliseconds)
	MpoolPushDuration                   = stats.Float64("mpool/push_ms", "Duration of Push in mpool", stats.UnitMilliseconds)
	BlockPublished                      = stats.Int64("block/published", "Counter for total locally published blocks", stats.UnitDimensionless)
	BlockReceived                       = stats.Int64("block/received", "Counter for total received blocks", stats.UnitDimensionless)
	BlockValidationFailure              = stats.Int64("block/failure", "Counter for block validation failures", stats.UnitDimensionless)
	BlockValidationSuccess              = stats.Int64("block/success", "Counter for block validation successes", stats.UnitDimensionless)
	BlockValidationDurationMilliseconds = stats.Float64("block/validation_ms", "Duration for Block Validation in ms", stats.UnitMilliseconds)
	BlockDelay                          = stats.Int64("block/delay", "Delay of accepted blocks, where delay is >5s", stats.UnitMilliseconds)
	PubsubPublishMessage                = stats.Int64("pubsub/published", "Counter for total published messages", stats.UnitDimensionless)
	PubsubDeliverMessage                = stats.Int64("pubsub/delivered", "Counter for total delivered messages", stats.UnitDimensionless)
	PubsubRejectMessage                 = stats.Int64("pubsub/rejected", "Counter for total rejected messages", stats.UnitDimensionless)
	PubsubDuplicateMessage              = stats.Int64("pubsub/duplicate", "Counter for total duplicate messages", stats.UnitDimensionless)
	PubsubRecvRPC                       = stats.Int64("pubsub/recv_rpc", "Counter for total received RPCs", stats.UnitDimensionless)
	PubsubSendRPC                       = stats.Int64("pubsub/send_rpc", "Counter for total sent RPCs", stats.UnitDimensionless)
	PubsubDropRPC                       = stats.Int64("pubsub/drop_rpc", "Counter for total dropped RPCs", stats.UnitDimensionless)
	VMFlushCopyDuration                 = stats.Float64("vm/flush_copy_ms", "Time spent in VM Flush Copy", stats.UnitMilliseconds)
	VMFlushCopyCount                    = stats.Int64("vm/flush_copy_count", "Number of copied objects", stats.UnitDimensionless)
	VMApplyBlocksTotal                  = stats.Float64("vm/applyblocks_total_ms", "Time spent applying block state", stats.UnitMilliseconds)
	VMApplyMessages                     = stats.Float64("vm/applyblocks_messages", "Time spent applying block messages", stats.UnitMilliseconds)
	VMApplyEarly                        = stats.Float64("vm/applyblocks_early", "Time spent in early apply-blocks (null cron, upgrades)", stats.UnitMilliseconds)
	VMApplyCron                         = stats.Float64("vm/applyblocks_cron", "Time spent in cron", stats.UnitMilliseconds)
	VMApplyFlush                        = stats.Float64("vm/applyblocks_flush", "Time spent flushing vm state", stats.UnitMilliseconds)
	VMSends                             = stats.Int64("vm/sends", "Counter for sends processed by the VM", stats.UnitDimensionless)
	VMApplied                           = stats.Int64("vm/applied", "Counter for messages (including internal messages) processed by the VM", stats.UnitDimensionless)
	VMExecutionWaiting                  = stats.Int64("vm/execution_waiting", "Counter for VM executions waiting to be assigned to a lane", stats.UnitDimensionless)
	VMExecutionRunning                  = stats.Int64("vm/execution_running", "Counter for running VM executions", stats.UnitDimensionless)

	// miner
	WorkerCallsStarted           = stats.Int64("sealing/worker_calls_started", "Counter of started worker tasks", stats.UnitDimensionless)
	WorkerCallsReturnedCount     = stats.Int64("sealing/worker_calls_returned_count", "Counter of returned worker tasks", stats.UnitDimensionless)
	WorkerCallsReturnedDuration  = stats.Float64("sealing/worker_calls_returned_ms", "Counter of returned worker tasks", stats.UnitMilliseconds)
	WorkerUntrackedCallsReturned = stats.Int64("sealing/worker_untracked_calls_returned", "Counter of returned untracked worker tasks", stats.UnitDimensionless)

	SectorStates = stats.Int64("sealing/states", "Number of sectors in each state", stats.UnitDimensionless)

	StorageFSAvailable      = stats.Float64("storage/path_fs_available_frac", "Fraction of filesystem available storage", stats.UnitDimensionless)
	StorageAvailable        = stats.Float64("storage/path_available_frac", "Fraction of available storage", stats.UnitDimensionless)
	StorageReserved         = stats.Float64("storage/path_reserved_frac", "Fraction of reserved storage", stats.UnitDimensionless)
	StorageLimitUsed        = stats.Float64("storage/path_limit_used_frac", "Fraction of used optional storage limit", stats.UnitDimensionless)
	StorageCapacityBytes    = stats.Int64("storage/path_capacity_bytes", "storage path capacity", stats.UnitBytes)
	StorageFSAvailableBytes = stats.Int64("storage/path_fs_available_bytes", "filesystem available storage bytes", stats.UnitBytes)
	StorageAvailableBytes   = stats.Int64("storage/path_available_bytes", "available storage bytes", stats.UnitBytes)
	StorageReservedBytes    = stats.Int64("storage/path_reserved_bytes", "reserved storage bytes", stats.UnitBytes)
	StorageLimitUsedBytes   = stats.Int64("storage/path_limit_used_bytes", "used optional storage limit bytes", stats.UnitBytes)
	StorageLimitMaxBytes    = stats.Int64("storage/path_limit_max_bytes", "optional storage limit", stats.UnitBytes)

	SchedAssignerCycleDuration           = stats.Float64("sched/assigner_cycle_ms", "Duration of scheduler assigner cycle", stats.UnitMilliseconds)
	SchedAssignerCandidatesDuration      = stats.Float64("sched/assigner_cycle_candidates_ms", "Duration of scheduler assigner candidate matching step", stats.UnitMilliseconds)
	SchedAssignerWindowSelectionDuration = stats.Float64("sched/assigner_cycle_window_select_ms", "Duration of scheduler window selection step", stats.UnitMilliseconds)
	SchedAssignerSubmitDuration          = stats.Float64("sched/assigner_cycle_submit_ms", "Duration of scheduler window submit step", stats.UnitMilliseconds)
	SchedCycleOpenWindows                = stats.Int64("sched/assigner_cycle_open_window", "Number of open windows in scheduling cycles", stats.UnitDimensionless)
	SchedCycleQueueSize                  = stats.Int64("sched/assigner_cycle_task_queue_entry", "Number of task queue entries in scheduling cycles", stats.UnitDimensionless)

	DagStorePRInitCount        = stats.Int64("dagstore/pr_init_count", "PieceReader init count", stats.UnitDimensionless)
	DagStorePRBytesRequested   = stats.Int64("dagstore/pr_requested_bytes", "PieceReader requested bytes", stats.UnitBytes)
	DagStorePRBytesDiscarded   = stats.Int64("dagstore/pr_discarded_bytes", "PieceReader discarded bytes", stats.UnitBytes)
	DagStorePRDiscardCount     = stats.Int64("dagstore/pr_discard_count", "PieceReader discard count", stats.UnitDimensionless)
	DagStorePRSeekBackCount    = stats.Int64("dagstore/pr_seek_back_count", "PieceReader seek back count", stats.UnitDimensionless)
	DagStorePRSeekForwardCount = stats.Int64("dagstore/pr_seek_forward_count", "PieceReader seek forward count", stats.UnitDimensionless)
	DagStorePRSeekBackBytes    = stats.Int64("dagstore/pr_seek_back_bytes", "PieceReader seek back bytes", stats.UnitBytes)
	DagStorePRSeekForwardBytes = stats.Int64("dagstore/pr_seek_forward_bytes", "PieceReader seek forward bytes", stats.UnitBytes)

	// splitstore
	SplitstoreMiss                  = stats.Int64("splitstore/miss", "Number of misses in hotstre access", stats.UnitDimensionless)
	SplitstoreCompactionTimeSeconds = stats.Float64("splitstore/compaction_time", "Compaction time in seconds", stats.UnitSeconds)
	SplitstoreCompactionHot         = stats.Int64("splitstore/hot", "Number of hot blocks in last compaction", stats.UnitDimensionless)
	SplitstoreCompactionCold        = stats.Int64("splitstore/cold", "Number of cold blocks in last compaction", stats.UnitDimensionless)
	SplitstoreCompactionDead        = stats.Int64("splitstore/dead", "Number of dead blocks in last compaction", stats.UnitDimensionless)

	// rcmgr
	RcmgrAllowConn      = stats.Int64("rcmgr/allow_conn", "Number of allowed connections", stats.UnitDimensionless)
	RcmgrBlockConn      = stats.Int64("rcmgr/block_conn", "Number of blocked connections", stats.UnitDimensionless)
	RcmgrAllowStream    = stats.Int64("rcmgr/allow_stream", "Number of allowed streams", stats.UnitDimensionless)
	RcmgrBlockStream    = stats.Int64("rcmgr/block_stream", "Number of blocked streams", stats.UnitDimensionless)
	RcmgrAllowPeer      = stats.Int64("rcmgr/allow_peer", "Number of allowed peer connections", stats.UnitDimensionless)
	RcmgrBlockPeer      = stats.Int64("rcmgr/block_peer", "Number of blocked peer connections", stats.UnitDimensionless)
	RcmgrAllowProto     = stats.Int64("rcmgr/allow_proto", "Number of allowed streams attached to a protocol", stats.UnitDimensionless)
	RcmgrBlockProto     = stats.Int64("rcmgr/block_proto", "Number of blocked blocked streams attached to a protocol", stats.UnitDimensionless)
	RcmgrBlockProtoPeer = stats.Int64("rcmgr/block_proto", "Number of blocked blocked streams attached to a protocol for a specific peer", stats.UnitDimensionless)
	RcmgrAllowSvc       = stats.Int64("rcmgr/allow_svc", "Number of allowed streams attached to a service", stats.UnitDimensionless)
	RcmgrBlockSvc       = stats.Int64("rcmgr/block_svc", "Number of blocked blocked streams attached to a service", stats.UnitDimensionless)
	RcmgrBlockSvcPeer   = stats.Int64("rcmgr/block_svc", "Number of blocked blocked streams attached to a service for a specific peer", stats.UnitDimensionless)
	RcmgrAllowMem       = stats.Int64("rcmgr/allow_mem", "Number of allowed memory reservations", stats.UnitDimensionless)
	RcmgrBlockMem       = stats.Int64("rcmgr/block_mem", "Number of blocked memory reservations", stats.UnitDimensionless)

	// gateway rate limit
	RateLimitCount = stats.Int64("ratelimit/limited", "rate limited connections", stats.UnitDimensionless)
)

var (
	InfoView = &view.View{
		Name:        "info",
		Description: "Lotus node information",
		Measure:     LotusInfo,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Version, Commit, NodeType},
	}
	ChainNodeHeightView = &view.View{
		Measure:     ChainNodeHeight,
		Aggregation: view.LastValue(),
	}
	ChainNodeHeightExpectedView = &view.View{
		Measure:     ChainNodeHeightExpected,
		Aggregation: view.LastValue(),
	}
	ChainNodeWorkerHeightView = &view.View{
		Measure:     ChainNodeWorkerHeight,
		Aggregation: view.LastValue(),
	}
	BlockReceivedView = &view.View{
		Measure:     BlockReceived,
		Aggregation: view.Count(),
	}
	BlockValidationFailureView = &view.View{
		Measure:     BlockValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType},
	}
	BlockValidationSuccessView = &view.View{
		Measure:     BlockValidationSuccess,
		Aggregation: view.Count(),
	}
	BlockValidationDurationView = &view.View{
		Measure:     BlockValidationDurationMilliseconds,
		Aggregation: defaultMillisecondsDistribution,
	}
	BlockDelayView = &view.View{
		Measure: BlockDelay,
		TagKeys: []tag.Key{MinerID},
		Aggregation: func() *view.Aggregation {
			var bounds []float64
			for i := 5; i < 29; i++ { // 5-29s, step 1s
				bounds = append(bounds, float64(i*1000))
			}
			for i := 30; i < 60; i += 2 { // 30-58s, step 2s
				bounds = append(bounds, float64(i*1000))
			}
			for i := 60; i <= 300; i += 10 { // 60-300s, step 10s
				bounds = append(bounds, float64(i*1000))
			}
			bounds = append(bounds, 600*1000) // final cutoff at 10m
			return view.Distribution(bounds...)
		}(),
	}
	IndexerMessageValidationFailureView = &view.View{
		Measure:     IndexerMessageValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType, Local},
	}
	IndexerMessageValidationSuccessView = &view.View{
		Measure:     IndexerMessageValidationSuccess,
		Aggregation: view.Count(),
	}
	MessagePendingView = &view.View{
		Measure:     MessagePending,
		Aggregation: view.LastValue(),
	}
	MessagePublishedView = &view.View{
		Measure:     MessagePublished,
		Aggregation: view.Count(),
	}
	MessageReceivedView = &view.View{
		Measure:     MessageReceived,
		Aggregation: view.Count(),
	}
	MessageValidationFailureView = &view.View{
		Measure:     MessageValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType, Local},
	}
	MessageValidationSuccessView = &view.View{
		Measure:     MessageValidationSuccess,
		Aggregation: view.Count(),
	}
	MessageValidationDurationView = &view.View{
		Measure:     MessageValidationDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{MsgValid, Local},
	}
	MpoolGetNonceDurationView = &view.View{
		Measure:     MpoolGetNonceDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	MpoolGetBalanceDurationView = &view.View{
		Measure:     MpoolGetBalanceDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	MpoolAddTsDurationView = &view.View{
		Measure:     MpoolAddTsDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	MpoolAddDurationView = &view.View{
		Measure:     MpoolAddDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	MpoolPushDurationView = &view.View{
		Measure:     MpoolPushDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	PeerCountView = &view.View{
		Measure:     PeerCount,
		Aggregation: view.LastValue(),
	}
	PubsubPublishMessageView = &view.View{
		Measure:     PubsubPublishMessage,
		Aggregation: view.Count(),
	}
	PubsubDeliverMessageView = &view.View{
		Measure:     PubsubDeliverMessage,
		Aggregation: view.Count(),
	}
	PubsubRejectMessageView = &view.View{
		Measure:     PubsubRejectMessage,
		Aggregation: view.Count(),
	}
	PubsubDuplicateMessageView = &view.View{
		Measure:     PubsubDuplicateMessage,
		Aggregation: view.Count(),
	}
	PubsubRecvRPCView = &view.View{
		Measure:     PubsubRecvRPC,
		Aggregation: view.Count(),
	}
	PubsubSendRPCView = &view.View{
		Measure:     PubsubSendRPC,
		Aggregation: view.Count(),
	}
	PubsubDropRPCView = &view.View{
		Measure:     PubsubDropRPC,
		Aggregation: view.Count(),
	}
	APIRequestDurationView = &view.View{
		Measure:     APIRequestDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{APIInterface, Endpoint},
	}
	VMFlushCopyDurationView = &view.View{
		Measure:     VMFlushCopyDuration,
		Aggregation: view.Sum(),
	}
	VMFlushCopyCountView = &view.View{
		Measure:     VMFlushCopyCount,
		Aggregation: view.Sum(),
	}
	VMApplyBlocksTotalView = &view.View{
		Measure:     VMApplyBlocksTotal,
		Aggregation: defaultMillisecondsDistribution,
	}
	VMApplyMessagesView = &view.View{
		Measure:     VMApplyMessages,
		Aggregation: defaultMillisecondsDistribution,
	}
	VMApplyEarlyView = &view.View{
		Measure:     VMApplyEarly,
		Aggregation: defaultMillisecondsDistribution,
	}
	VMApplyCronView = &view.View{
		Measure:     VMApplyCron,
		Aggregation: defaultMillisecondsDistribution,
	}
	VMApplyFlushView = &view.View{
		Measure:     VMApplyFlush,
		Aggregation: defaultMillisecondsDistribution,
	}
	VMSendsView = &view.View{
		Measure:     VMSends,
		Aggregation: view.LastValue(),
	}
	VMAppliedView = &view.View{
		Measure:     VMApplied,
		Aggregation: view.LastValue(),
	}
	VMExecutionWaitingView = &view.View{
		Measure:     VMExecutionWaiting,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{ExecutionLane},
	}
	VMExecutionRunningView = &view.View{
		Measure:     VMExecutionRunning,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{ExecutionLane},
	}

	// miner
	WorkerCallsStartedView = &view.View{
		Measure:     WorkerCallsStarted,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{TaskType, WorkerHostname},
	}
	WorkerCallsReturnedCountView = &view.View{
		Measure:     WorkerCallsReturnedCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{TaskType, WorkerHostname},
	}
	WorkerUntrackedCallsReturnedView = &view.View{
		Measure:     WorkerUntrackedCallsReturned,
		Aggregation: view.Count(),
	}
	WorkerCallsReturnedDurationView = &view.View{
		Measure:     WorkerCallsReturnedDuration,
		Aggregation: workMillisecondsDistribution,
		TagKeys:     []tag.Key{TaskType, WorkerHostname},
	}
	SectorStatesView = &view.View{
		Measure:     SectorStates,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{SectorState},
	}
	StorageFSAvailableView = &view.View{
		Measure:     StorageFSAvailable,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageAvailableView = &view.View{
		Measure:     StorageAvailable,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageReservedView = &view.View{
		Measure:     StorageReserved,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageLimitUsedView = &view.View{
		Measure:     StorageLimitUsed,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageCapacityBytesView = &view.View{
		Measure:     StorageCapacityBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageFSAvailableBytesView = &view.View{
		Measure:     StorageFSAvailableBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageAvailableBytesView = &view.View{
		Measure:     StorageAvailableBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageReservedBytesView = &view.View{
		Measure:     StorageReservedBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageLimitUsedBytesView = &view.View{
		Measure:     StorageLimitUsedBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}
	StorageLimitMaxBytesView = &view.View{
		Measure:     StorageLimitMaxBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal},
	}

	SchedAssignerCycleDurationView = &view.View{
		Measure:     SchedAssignerCycleDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	SchedAssignerCandidatesDurationView = &view.View{
		Measure:     SchedAssignerCandidatesDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	SchedAssignerWindowSelectionDurationView = &view.View{
		Measure:     SchedAssignerWindowSelectionDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	SchedAssignerSubmitDurationView = &view.View{
		Measure:     SchedAssignerSubmitDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	SchedCycleOpenWindowsView = &view.View{
		Measure:     SchedCycleOpenWindows,
		Aggregation: queueSizeDistribution,
	}
	SchedCycleQueueSizeView = &view.View{
		Measure:     SchedCycleQueueSize,
		Aggregation: queueSizeDistribution,
	}

	DagStorePRInitCountView = &view.View{
		Measure:     DagStorePRInitCount,
		Aggregation: view.Count(),
	}
	DagStorePRBytesRequestedView = &view.View{
		Measure:     DagStorePRBytesRequested,
		Aggregation: view.Sum(),
	}
	DagStorePRBytesDiscardedView = &view.View{
		Measure:     DagStorePRBytesDiscarded,
		Aggregation: view.Sum(),
	}
	DagStorePRDiscardCountView = &view.View{
		Measure:     DagStorePRDiscardCount,
		Aggregation: view.Count(),
	}
	DagStorePRSeekBackCountView = &view.View{
		Measure:     DagStorePRSeekBackCount,
		Aggregation: view.Count(),
	}
	DagStorePRSeekForwardCountView = &view.View{
		Measure:     DagStorePRSeekForwardCount,
		Aggregation: view.Count(),
	}
	DagStorePRSeekBackBytesView = &view.View{
		Measure:     DagStorePRSeekBackBytes,
		Aggregation: view.Sum(),
	}
	DagStorePRSeekForwardBytesView = &view.View{
		Measure:     DagStorePRSeekForwardBytes,
		Aggregation: view.Sum(),
	}

	// splitstore
	SplitstoreMissView = &view.View{
		Measure:     SplitstoreMiss,
		Aggregation: view.Count(),
	}
	SplitstoreCompactionTimeSecondsView = &view.View{
		Measure:     SplitstoreCompactionTimeSeconds,
		Aggregation: view.LastValue(),
	}
	SplitstoreCompactionHotView = &view.View{
		Measure:     SplitstoreCompactionHot,
		Aggregation: view.LastValue(),
	}
	SplitstoreCompactionColdView = &view.View{
		Measure:     SplitstoreCompactionCold,
		Aggregation: view.Sum(),
	}
	SplitstoreCompactionDeadView = &view.View{
		Measure:     SplitstoreCompactionDead,
		Aggregation: view.Sum(),
	}

	// graphsync
	GraphsyncReceivingPeersCountView = &view.View{
		Measure:     GraphsyncReceivingPeersCount,
		Aggregation: view.LastValue(),
	}
	GraphsyncReceivingActiveCountView = &view.View{
		Measure:     GraphsyncReceivingActiveCount,
		Aggregation: view.LastValue(),
	}
	GraphsyncReceivingCountCountView = &view.View{
		Measure:     GraphsyncReceivingCountCount,
		Aggregation: view.LastValue(),
	}
	GraphsyncReceivingTotalMemoryAllocatedView = &view.View{
		Measure:     GraphsyncReceivingTotalMemoryAllocated,
		Aggregation: view.LastValue(),
	}
	GraphsyncReceivingTotalPendingAllocationsView = &view.View{
		Measure:     GraphsyncReceivingTotalPendingAllocations,
		Aggregation: view.LastValue(),
	}
	GraphsyncReceivingPeersPendingView = &view.View{
		Measure:     GraphsyncReceivingPeersPending,
		Aggregation: view.LastValue(),
	}
	GraphsyncSendingPeersCountView = &view.View{
		Measure:     GraphsyncSendingPeersCount,
		Aggregation: view.LastValue(),
	}
	GraphsyncSendingActiveCountView = &view.View{
		Measure:     GraphsyncSendingActiveCount,
		Aggregation: view.LastValue(),
	}
	GraphsyncSendingCountCountView = &view.View{
		Measure:     GraphsyncSendingCountCount,
		Aggregation: view.LastValue(),
	}
	GraphsyncSendingTotalMemoryAllocatedView = &view.View{
		Measure:     GraphsyncSendingTotalMemoryAllocated,
		Aggregation: view.LastValue(),
	}
	GraphsyncSendingTotalPendingAllocationsView = &view.View{
		Measure:     GraphsyncSendingTotalPendingAllocations,
		Aggregation: view.LastValue(),
	}
	GraphsyncSendingPeersPendingView = &view.View{
		Measure:     GraphsyncSendingPeersPending,
		Aggregation: view.LastValue(),
	}

	// rcmgr
	RcmgrAllowConnView = &view.View{
		Measure:     RcmgrAllowConn,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Direction, UseFD},
	}
	RcmgrBlockConnView = &view.View{
		Measure:     RcmgrBlockConn,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Direction, UseFD},
	}
	RcmgrAllowStreamView = &view.View{
		Measure:     RcmgrAllowStream,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID, Direction},
	}
	RcmgrBlockStreamView = &view.View{
		Measure:     RcmgrBlockStream,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID, Direction},
	}
	RcmgrAllowPeerView = &view.View{
		Measure:     RcmgrAllowPeer,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID},
	}
	RcmgrBlockPeerView = &view.View{
		Measure:     RcmgrBlockPeer,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID},
	}
	RcmgrAllowProtoView = &view.View{
		Measure:     RcmgrAllowProto,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{ProtocolID},
	}
	RcmgrBlockProtoView = &view.View{
		Measure:     RcmgrBlockProto,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{ProtocolID},
	}
	RcmgrBlockProtoPeerView = &view.View{
		Measure:     RcmgrBlockProtoPeer,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{ProtocolID, PeerID},
	}
	RcmgrAllowSvcView = &view.View{
		Measure:     RcmgrAllowSvc,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{ServiceID},
	}
	RcmgrBlockSvcView = &view.View{
		Measure:     RcmgrBlockSvc,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{ServiceID},
	}
	RcmgrBlockSvcPeerView = &view.View{
		Measure:     RcmgrBlockSvcPeer,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{ServiceID, PeerID},
	}
	RcmgrAllowMemView = &view.View{
		Measure:     RcmgrAllowMem,
		Aggregation: view.Count(),
	}
	RcmgrBlockMemView = &view.View{
		Measure:     RcmgrBlockMem,
		Aggregation: view.Count(),
	}
	RateLimitedView = &view.View{
		Measure:     RateLimitCount,
		Aggregation: view.Count(),
	}
)

// DefaultViews is an array of OpenCensus views for metric gathering purposes
var DefaultViews = func() []*view.View {
	views := []*view.View{
		InfoView,
		PeerCountView,
		APIRequestDurationView,

		GraphsyncReceivingPeersCountView,
		GraphsyncReceivingActiveCountView,
		GraphsyncReceivingCountCountView,
		GraphsyncReceivingTotalMemoryAllocatedView,
		GraphsyncReceivingTotalPendingAllocationsView,
		GraphsyncReceivingPeersPendingView,
		GraphsyncSendingPeersCountView,
		GraphsyncSendingActiveCountView,
		GraphsyncSendingCountCountView,
		GraphsyncSendingTotalMemoryAllocatedView,
		GraphsyncSendingTotalPendingAllocationsView,
		GraphsyncSendingPeersPendingView,

		RcmgrAllowConnView,
		RcmgrBlockConnView,
		RcmgrAllowStreamView,
		RcmgrBlockStreamView,
		RcmgrAllowPeerView,
		RcmgrBlockPeerView,
		RcmgrAllowProtoView,
		RcmgrBlockProtoView,
		RcmgrBlockProtoPeerView,
		RcmgrAllowSvcView,
		RcmgrBlockSvcView,
		RcmgrBlockSvcPeerView,
		RcmgrAllowMemView,
		RcmgrBlockMemView,
	}
	views = append(views, blockstore.DefaultViews...)
	views = append(views, rpcmetrics.DefaultViews...)
	return views
}()

var ChainNodeViews = append([]*view.View{
	ChainNodeHeightView,
	ChainNodeHeightExpectedView,
	ChainNodeWorkerHeightView,
	BlockReceivedView,
	BlockValidationFailureView,
	BlockValidationSuccessView,
	BlockValidationDurationView,
	BlockDelayView,
	IndexerMessageValidationFailureView,
	IndexerMessageValidationSuccessView,
	MessagePendingView,
	MessagePublishedView,
	MessageReceivedView,
	MessageValidationFailureView,
	MessageValidationSuccessView,
	MessageValidationDurationView,
	MpoolGetNonceDurationView,
	MpoolGetBalanceDurationView,
	MpoolAddTsDurationView,
	MpoolAddDurationView,
	MpoolPushDurationView,
	PubsubPublishMessageView,
	PubsubDeliverMessageView,
	PubsubRejectMessageView,
	PubsubDuplicateMessageView,
	PubsubRecvRPCView,
	PubsubSendRPCView,
	PubsubDropRPCView,
	VMFlushCopyCountView,
	VMFlushCopyDurationView,
	SplitstoreMissView,
	SplitstoreCompactionTimeSecondsView,
	SplitstoreCompactionHotView,
	SplitstoreCompactionColdView,
	SplitstoreCompactionDeadView,
	VMApplyBlocksTotalView,
	VMApplyMessagesView,
	VMApplyEarlyView,
	VMApplyCronView,
	VMApplyFlushView,
	VMSendsView,
	VMAppliedView,
	VMExecutionWaitingView,
	VMExecutionRunningView,
}, DefaultViews...)

var MinerNodeViews = append([]*view.View{
	WorkerCallsStartedView,
	WorkerCallsReturnedCountView,
	WorkerUntrackedCallsReturnedView,
	WorkerCallsReturnedDurationView,

	SectorStatesView,
	StorageFSAvailableView,
	StorageAvailableView,
	StorageReservedView,
	StorageLimitUsedView,
	StorageCapacityBytesView,
	StorageFSAvailableBytesView,
	StorageAvailableBytesView,
	StorageReservedBytesView,
	StorageLimitUsedBytesView,
	StorageLimitMaxBytesView,

	SchedAssignerCycleDurationView,
	SchedAssignerCandidatesDurationView,
	SchedAssignerWindowSelectionDurationView,
	SchedAssignerSubmitDurationView,
	SchedCycleOpenWindowsView,
	SchedCycleQueueSizeView,

	DagStorePRInitCountView,
	DagStorePRBytesRequestedView,
	DagStorePRBytesDiscardedView,
	DagStorePRDiscardCountView,
	DagStorePRSeekBackCountView,
	DagStorePRSeekForwardCountView,
	DagStorePRSeekBackBytesView,
	DagStorePRSeekForwardBytesView,
}, DefaultViews...)

var GatewayNodeViews = append([]*view.View{
	RateLimitedView,
}, ChainNodeViews...)

// SinceInMilliseconds returns the duration of time since the provide time as a float64.
func SinceInMilliseconds(startTime time.Time) float64 {
	return float64(time.Since(startTime).Nanoseconds()) / 1e6
}

// Timer is a function stopwatch, calling it starts the timer,
// calling the returned function will record the duration.
func Timer(ctx context.Context, m *stats.Float64Measure) func() time.Duration {
	start := time.Now()
	return func() time.Duration {
		stats.Record(ctx, m.M(SinceInMilliseconds(start)))
		return time.Since(start)
	}
}
