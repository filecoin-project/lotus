package metrics

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	rpcmetrics "github.com/filecoin-project/go-jsonrpc/metrics"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
)

// Distributions
var defaultMillisecondsDistribution = view.Distribution(
	0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, // Very short intervals for fast operations
	10, 20, 30, 40, 50, 60, 70, 80, 90, 100, // 10 ms intervals up to 100 ms
	150, 200, 250, 300, 350, 400, 450, 500, // 50 ms intervals from 100 to 500 ms
	600, 700, 800, 900, 1000, // 100 ms intervals from 500 to 1000 ms
	1100, 1200, 1300, 1400, 1500, 1600, 1700, 1800, 1900, 2000, // 100 ms intervals from 1000 to 2000 ms
	3000, 4000, 5000, 6000, 8000, 10000, 13000, 16000, 20000, 25000, 30000, 40000, 50000, 65000, 80000, 100000,
	130_000, 160_000, 200_000, 250_000, 300_000, 400_000, 500_000, 650_000, 800_000, 1000_000, // Larger, less frequent buckets
)

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
	Network, _     = tag.NewKey("network")
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

	// piecereader
	PRReadType, _ = tag.NewKey("pr_type") // seq / rand
	PRReadSize, _ = tag.NewKey("pr_size") // small / big

	// message fetching
	FetchSource, _ = tag.NewKey("fetch_source") // "local" or "network"
)

// Measures
var (
	// common
	LotusInfo          = stats.Int64("info", "Arbitrary counter to tag lotus info to", stats.UnitDimensionless)
	PeerCount          = stats.Int64("peer/count", "Current number of FIL peers", stats.UnitDimensionless)
	APIRequestDuration = stats.Float64("api/request_duration_ms", "Duration of API requests", stats.UnitMilliseconds)

	// chain
	ChainNodeHeight                     = stats.Int64("chain/node_height", "Current Height of the node", stats.UnitDimensionless)
	ChainNodeHeightExpected             = stats.Int64("chain/node_height_expected", "Expected Height of the node", stats.UnitDimensionless)
	ChainNodeWorkerHeight               = stats.Int64("chain/node_worker_height", "Current Height of workers on the node", stats.UnitDimensionless)
	IndexerMessageValidationFailure     = stats.Int64("indexer/failure", "Counter for indexer message validation failures", stats.UnitDimensionless)
	IndexerMessageValidationSuccess     = stats.Int64("indexer/success", "Counter for indexer message validation successes", stats.UnitDimensionless)
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
	MpoolMessageCount                   = stats.Int64("mpool/message_count", "Number of messages in the mpool", stats.UnitDimensionless)
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
	PubsubPruneMessage                  = stats.Int64("pubsub/prune", "Counter for total prune messages", stats.UnitDimensionless)
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
	VMApplyBlocksTotalGas               = stats.Int64("vm/applyblocks_total_gas", "Total gas of messages and cron", stats.UnitDimensionless)
	VMApplyMessagesGas                  = stats.Int64("vm/applyblocks_messages_gas", "Total gas of block messages", stats.UnitDimensionless)
	VMApplyEarlyGas                     = stats.Int64("vm/applyblocks_early_gas", "Total gas of early apply-blocks (null cron, upgrades)", stats.UnitDimensionless)
	VMApplyCronGas                      = stats.Int64("vm/applyblocks_cron_gas", "Total gas of cron", stats.UnitDimensionless)
	VMSends                             = stats.Int64("vm/sends", "Counter for sends processed by the VM", stats.UnitDimensionless)
	VMApplied                           = stats.Int64("vm/applied", "Counter for messages (including internal messages) processed by the VM", stats.UnitDimensionless)
	VMExecutionWaiting                  = stats.Int64("vm/execution_waiting", "Counter for VM executions waiting to be assigned to a lane", stats.UnitDimensionless)
	VMExecutionRunning                  = stats.Int64("vm/execution_running", "Counter for running VM executions", stats.UnitDimensionless)

	// message fetching
	MessageFetchRequested = stats.Int64("message/fetch_requested", "Number of messages requested for fetch", stats.UnitDimensionless)
	MessageFetchLocal     = stats.Int64("message/fetch_local", "Number of messages found locally", stats.UnitDimensionless)
	MessageFetchNetwork   = stats.Int64("message/fetch_network", "Number of messages fetched from network", stats.UnitDimensionless)
	MessageFetchDuration  = stats.Float64("message/fetch_duration_ms", "Duration of message fetch operations", stats.UnitMilliseconds)

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

	DagStorePRInitCount      = stats.Int64("dagstore/pr_init_count", "PieceReader init count", stats.UnitDimensionless)
	DagStorePRBytesRequested = stats.Int64("dagstore/pr_requested_bytes", "PieceReader requested bytes", stats.UnitBytes)

	DagStorePRBytesDiscarded   = stats.Int64("dagstore/pr_discarded_bytes", "PieceReader discarded bytes", stats.UnitBytes)
	DagStorePRDiscardCount     = stats.Int64("dagstore/pr_discard_count", "PieceReader discard count", stats.UnitDimensionless)
	DagStorePRSeekBackCount    = stats.Int64("dagstore/pr_seek_back_count", "PieceReader seek back count", stats.UnitDimensionless)
	DagStorePRSeekForwardCount = stats.Int64("dagstore/pr_seek_forward_count", "PieceReader seek forward count", stats.UnitDimensionless)
	DagStorePRSeekBackBytes    = stats.Int64("dagstore/pr_seek_back_bytes", "PieceReader seek back bytes", stats.UnitBytes)
	DagStorePRSeekForwardBytes = stats.Int64("dagstore/pr_seek_forward_bytes", "PieceReader seek forward bytes", stats.UnitBytes)

	DagStorePRAtHitBytes       = stats.Int64("dagstore/pr_at_hit_bytes", "PieceReader ReadAt bytes from cache", stats.UnitBytes)
	DagStorePRAtHitCount       = stats.Int64("dagstore/pr_at_hit_count", "PieceReader ReadAt from cache hits", stats.UnitDimensionless)
	DagStorePRAtCacheFillCount = stats.Int64("dagstore/pr_at_cache_fill_count", "PieceReader ReadAt full cache fill count", stats.UnitDimensionless)
	DagStorePRAtReadBytes      = stats.Int64("dagstore/pr_at_read_bytes", "PieceReader ReadAt bytes read from source", stats.UnitBytes)    // PRReadSize tag
	DagStorePRAtReadCount      = stats.Int64("dagstore/pr_at_read_count", "PieceReader ReadAt reads from source", stats.UnitDimensionless) // PRReadSize tag

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
	RcmgrBlockProto     = stats.Int64("rcmgr/block_proto", "Number of blocked streams attached to a protocol", stats.UnitDimensionless)
	RcmgrBlockProtoPeer = stats.Int64("rcmgr/block_proto", "Number of blocked streams attached to a protocol for a specific peer", stats.UnitDimensionless)
	RcmgrAllowSvc       = stats.Int64("rcmgr/allow_svc", "Number of allowed streams attached to a service", stats.UnitDimensionless)
	RcmgrBlockSvc       = stats.Int64("rcmgr/block_svc", "Number of blocked streams attached to a service", stats.UnitDimensionless)
	RcmgrBlockSvcPeer   = stats.Int64("rcmgr/block_svc", "Number of blocked streams attached to a service for a specific peer", stats.UnitDimensionless)
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
		TagKeys:     []tag.Key{Version, Commit, NodeType, Network},
	}
	ChainNodeHeightView = &view.View{
		Measure:     ChainNodeHeight,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	ChainNodeHeightExpectedView = &view.View{
		Measure:     ChainNodeHeightExpected,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	ChainNodeWorkerHeightView = &view.View{
		Measure:     ChainNodeWorkerHeight,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	BlockReceivedView = &view.View{
		Measure:     BlockReceived,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	BlockValidationFailureView = &view.View{
		Measure:     BlockValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType, Network},
	}
	BlockValidationSuccessView = &view.View{
		Measure:     BlockValidationSuccess,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	BlockValidationDurationView = &view.View{
		Measure:     BlockValidationDurationMilliseconds,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	BlockDelayView = &view.View{
		Measure: BlockDelay,
		TagKeys: []tag.Key{MinerID, Network},
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
		TagKeys:     []tag.Key{FailureType, Local, Network},
	}
	IndexerMessageValidationSuccessView = &view.View{
		Measure:     IndexerMessageValidationSuccess,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	MessagePublishedView = &view.View{
		Measure:     MessagePublished,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	MessageReceivedView = &view.View{
		Measure:     MessageReceived,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	MessageValidationFailureView = &view.View{
		Measure:     MessageValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType, Local, Network},
	}
	MessageValidationSuccessView = &view.View{
		Measure:     MessageValidationSuccess,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	MessageValidationDurationView = &view.View{
		Measure:     MessageValidationDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{MsgValid, Local, Network},
	}
	MpoolGetNonceDurationView = &view.View{
		Measure:     MpoolGetNonceDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	MpoolGetBalanceDurationView = &view.View{
		Measure:     MpoolGetBalanceDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	MpoolAddTsDurationView = &view.View{
		Measure:     MpoolAddTsDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	MpoolAddDurationView = &view.View{
		Measure:     MpoolAddDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	MpoolPushDurationView = &view.View{
		Measure:     MpoolPushDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	MpoolMessageCountView = &view.View{
		Measure:     MpoolMessageCount,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	PeerCountView = &view.View{
		Measure:     PeerCount,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubPublishMessageView = &view.View{
		Measure:     PubsubPublishMessage,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubDeliverMessageView = &view.View{
		Measure:     PubsubDeliverMessage,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubRejectMessageView = &view.View{
		Measure:     PubsubRejectMessage,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubDuplicateMessageView = &view.View{
		Measure:     PubsubDuplicateMessage,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubPruneMessageView = &view.View{
		Measure:     PubsubPruneMessage,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubRecvRPCView = &view.View{
		Measure:     PubsubRecvRPC,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubSendRPCView = &view.View{
		Measure:     PubsubSendRPC,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	PubsubDropRPCView = &view.View{
		Measure:     PubsubDropRPC,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	APIRequestDurationView = &view.View{
		Measure:     APIRequestDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{APIInterface, Endpoint, Network},
	}
	VMFlushCopyDurationView = &view.View{
		Measure:     VMFlushCopyDuration,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}
	VMFlushCopyCountView = &view.View{
		Measure:     VMFlushCopyCount,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}
	VMApplyBlocksTotalView = &view.View{
		Measure:     VMApplyBlocksTotal,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	VMApplyMessagesView = &view.View{
		Measure:     VMApplyMessages,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	VMApplyEarlyView = &view.View{
		Measure:     VMApplyEarly,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	VMApplyCronView = &view.View{
		Measure:     VMApplyCron,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	VMApplyFlushView = &view.View{
		Measure:     VMApplyFlush,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	VMApplyBlocksTotalGasView = &view.View{
		Measure:     VMApplyBlocksTotalGas,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	VMApplyEarlyGasView = &view.View{
		Measure:     VMApplyEarlyGas,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	VMApplyMessagesGasView = &view.View{
		Measure:     VMApplyMessagesGas,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	VMApplyCronGasView = &view.View{
		Measure:     VMApplyCronGas,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	VMSendsView = &view.View{
		Measure:     VMSends,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	VMAppliedView = &view.View{
		Measure:     VMApplied,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	VMExecutionWaitingView = &view.View{
		Measure:     VMExecutionWaiting,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{ExecutionLane, Network},
	}
	VMExecutionRunningView = &view.View{
		Measure:     VMExecutionRunning,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{ExecutionLane, Network},
	}

	// message fetching
	MessageFetchRequestedView = &view.View{
		Measure:     MessageFetchRequested,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	MessageFetchLocalView = &view.View{
		Measure:     MessageFetchLocal,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	MessageFetchNetworkView = &view.View{
		Measure:     MessageFetchNetwork,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	MessageFetchDurationView = &view.View{
		Measure:     MessageFetchDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{FetchSource, Network},
	}

	// miner
	WorkerCallsStartedView = &view.View{
		Measure:     WorkerCallsStarted,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{TaskType, WorkerHostname, Network},
	}
	WorkerCallsReturnedCountView = &view.View{
		Measure:     WorkerCallsReturnedCount,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{TaskType, WorkerHostname, Network},
	}
	WorkerUntrackedCallsReturnedView = &view.View{
		Measure: WorkerUntrackedCallsReturned,

		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	WorkerCallsReturnedDurationView = &view.View{
		Measure:     WorkerCallsReturnedDuration,
		Aggregation: workMillisecondsDistribution,
		TagKeys:     []tag.Key{TaskType, WorkerHostname, Network},
	}
	SectorStatesView = &view.View{
		Measure:     SectorStates,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{SectorState, Network},
	}
	StorageFSAvailableView = &view.View{
		Measure:     StorageFSAvailable,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageAvailableView = &view.View{
		Measure:     StorageAvailable,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageReservedView = &view.View{
		Measure:     StorageReserved,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageLimitUsedView = &view.View{
		Measure:     StorageLimitUsed,
		Aggregation: view.LastValue(),

		TagKeys: []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageCapacityBytesView = &view.View{
		Measure:     StorageCapacityBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageFSAvailableBytesView = &view.View{
		Measure:     StorageFSAvailableBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageAvailableBytesView = &view.View{
		Measure:     StorageAvailableBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageReservedBytesView = &view.View{
		Measure:     StorageReservedBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageLimitUsedBytesView = &view.View{
		Measure:     StorageLimitUsedBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}
	StorageLimitMaxBytesView = &view.View{
		Measure:     StorageLimitMaxBytes,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{StorageID, PathStorage, PathSeal, Network},
	}

	SchedAssignerCycleDurationView = &view.View{
		Measure:     SchedAssignerCycleDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	SchedAssignerCandidatesDurationView = &view.View{
		Measure:     SchedAssignerCandidatesDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	SchedAssignerWindowSelectionDurationView = &view.View{
		Measure:     SchedAssignerWindowSelectionDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	SchedAssignerSubmitDurationView = &view.View{
		Measure:     SchedAssignerSubmitDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{Network},
	}
	SchedCycleOpenWindowsView = &view.View{
		Measure:     SchedCycleOpenWindows,
		Aggregation: queueSizeDistribution,
		TagKeys:     []tag.Key{Network},
	}
	SchedCycleQueueSizeView = &view.View{
		Measure:     SchedCycleQueueSize,
		Aggregation: queueSizeDistribution,
		TagKeys:     []tag.Key{Network},
	}

	DagStorePRInitCountView = &view.View{
		Measure:     DagStorePRInitCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRBytesRequestedView = &view.View{
		Measure:     DagStorePRBytesRequested,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{PRReadType, Network},
	}
	DagStorePRBytesDiscardedView = &view.View{
		Measure:     DagStorePRBytesDiscarded,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRDiscardCountView = &view.View{
		Measure:     DagStorePRDiscardCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRSeekBackCountView = &view.View{
		Measure:     DagStorePRSeekBackCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRSeekForwardCountView = &view.View{
		Measure:     DagStorePRSeekForwardCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRSeekBackBytesView = &view.View{
		Measure:     DagStorePRSeekBackBytes,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRSeekForwardBytesView = &view.View{
		Measure:     DagStorePRSeekForwardBytes,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}

	DagStorePRAtHitBytesView = &view.View{
		Measure:     DagStorePRAtHitBytes,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRAtHitCountView = &view.View{
		Measure:     DagStorePRAtHitCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRAtCacheFillCountView = &view.View{
		Measure:     DagStorePRAtCacheFillCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	DagStorePRAtReadBytesView = &view.View{
		Measure:     DagStorePRAtReadBytes,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{PRReadSize, Network},
	}
	DagStorePRAtReadCountView = &view.View{
		Measure:     DagStorePRAtReadCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PRReadSize, Network},
	}

	// splitstore
	SplitstoreMissView = &view.View{
		Measure:     SplitstoreMiss,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	SplitstoreCompactionTimeSecondsView = &view.View{
		Measure:     SplitstoreCompactionTimeSeconds,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	SplitstoreCompactionHotView = &view.View{
		Measure:     SplitstoreCompactionHot,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Network},
	}
	SplitstoreCompactionColdView = &view.View{
		Measure:     SplitstoreCompactionCold,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}
	SplitstoreCompactionDeadView = &view.View{
		Measure:     SplitstoreCompactionDead,
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{Network},
	}

	// rcmgr
	RcmgrAllowConnView = &view.View{
		Measure:     RcmgrAllowConn,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Direction, UseFD, Network},
	}
	RcmgrBlockConnView = &view.View{
		Measure:     RcmgrBlockConn,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Direction, UseFD, Network},
	}
	RcmgrAllowStreamView = &view.View{
		Measure:     RcmgrAllowStream,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID, Direction, Network},
	}
	RcmgrBlockStreamView = &view.View{
		Measure:     RcmgrBlockStream,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID, Direction, Network},
	}
	RcmgrAllowPeerView = &view.View{
		Measure:     RcmgrAllowPeer,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID, Network},
	}
	RcmgrBlockPeerView = &view.View{
		Measure:     RcmgrBlockPeer,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{PeerID, Network},
	}
	RcmgrAllowProtoView = &view.View{
		Measure:     RcmgrAllowProto,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{ProtocolID, Network},
	}
	RcmgrBlockProtoView = &view.View{
		Measure:     RcmgrBlockProto,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{ProtocolID, Network},
	}
	RcmgrBlockProtoPeerView = &view.View{
		Measure:     RcmgrBlockProtoPeer,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{ProtocolID, PeerID, Network},
	}
	RcmgrAllowSvcView = &view.View{
		Measure:     RcmgrAllowSvc,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{ServiceID, Network},
	}
	RcmgrBlockSvcView = &view.View{
		Measure:     RcmgrBlockSvc,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{ServiceID, Network},
	}
	RcmgrBlockSvcPeerView = &view.View{
		Measure:     RcmgrBlockSvcPeer,
		Aggregation: view.Count(),

		TagKeys: []tag.Key{ServiceID, PeerID, Network},
	}
	RcmgrAllowMemView = &view.View{
		Measure:     RcmgrAllowMem,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	RcmgrBlockMemView = &view.View{
		Measure:     RcmgrBlockMem,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
	RateLimitedView = &view.View{
		Measure:     RateLimitCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{Network},
	}
)

var views = []*view.View{
	InfoView,
	PeerCountView,
	APIRequestDurationView,

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

// DefaultViews is an array of OpenCensus views for metric gathering purposes
var DefaultViews = func() []*view.View {
	return views
}()

// RegisterViews adds views to the default list without modifying this file.
func RegisterViews(v ...*view.View) {
	views = append(views, v...)
}

func init() {
	RegisterViews(blockstore.DefaultViews...)
	RegisterViews(rpcmetrics.DefaultViews...)
}

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
	MpoolMessageCountView,
	PubsubPublishMessageView,
	PubsubDeliverMessageView,
	PubsubRejectMessageView,
	PubsubDuplicateMessageView,
	PubsubPruneMessageView,
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
	VMApplyBlocksTotalGasView,
	VMApplyEarlyGasView,
	VMApplyMessagesGasView,
	VMApplyCronGasView,
	VMSendsView,
	VMAppliedView,
	VMExecutionWaitingView,
	VMExecutionRunningView,
	MessageFetchRequestedView,
	MessageFetchLocalView,
	MessageFetchNetworkView,
	MessageFetchDurationView,
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
	DagStorePRAtHitBytesView,
	DagStorePRAtHitCountView,
	DagStorePRAtCacheFillCountView,
	DagStorePRAtReadBytesView,
	DagStorePRAtReadCountView,
}, DefaultViews...)

var GatewayNodeViews = append([]*view.View{
	RateLimitedView,
}, ChainNodeViews...)

// SinceInMilliseconds returns the duration of time since the provide time as a float64.
func SinceInMilliseconds(startTime time.Time) float64 {
	return float64(time.Since(startTime).Milliseconds())
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

func AddNetworkTag(ctx context.Context) context.Context {
	ctx, _ = tag.New(ctx, tag.Upsert(Network, buildconstants.NetworkBundle))
	return ctx
}
