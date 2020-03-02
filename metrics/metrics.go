package metrics

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// Global Tags
var (
	Version, _      = tag.NewKey("version")
	Commit, _       = tag.NewKey("commit")
	RPCMethod, _    = tag.NewKey("method")
	PeerID, _       = tag.NewKey("peer_id")
	FailureType, _  = tag.NewKey("failure_type")
	MessageFrom, _  = tag.NewKey("message_from")
	MessageTo, _    = tag.NewKey("message_to")
	MessageNonce, _ = tag.NewKey("message_nonce")
	ReceivedFrom, _ = tag.NewKey("received_from")
)

// Measures
var (
	LotusInfo                = stats.Int64("info", "Arbitrary counter to tag lotus info to", stats.UnitDimensionless)
	ChainNodeHeight          = stats.Int64("chain/node_height", "Current Height of the node", stats.UnitDimensionless)
	ChainNodeWorkerHeight    = stats.Int64("chain/node_worker_height", "Current Height of workers on the node", stats.UnitDimensionless)
	MessageReceived          = stats.Int64("message/received", "Counter for total received messages", stats.UnitDimensionless)
	MessageValidationFailure = stats.Int64("message/failure", "Counter for message validation failures", stats.UnitDimensionless)
	MessageValidationSuccess = stats.Int64("message/success", "Counter for message validation successes", stats.UnitDimensionless)
	BlockReceived            = stats.Int64("block/received", "Counter for total received blocks", stats.UnitDimensionless)
	BlockValidationFailure   = stats.Int64("block/failure", "Counter for block validation failures", stats.UnitDimensionless)
	BlockValidationSuccess   = stats.Int64("block/success", "Counter for block validation successes", stats.UnitDimensionless)
	PeerCount                = stats.Int64("peer/count", "Current number of FIL peers", stats.UnitDimensionless)
	RPCInvalidMethod         = stats.Int64("rpc/invalid_method", "Total number of invalid RPC methods called", stats.UnitDimensionless)
	RPCRequestError          = stats.Int64("rpc/request_error", "Total number of request errors handled", stats.UnitDimensionless)
	RPCResponseError         = stats.Int64("rpc/response_error", "Total number of responses errors handled", stats.UnitDimensionless)
)

// DefaultViews is an array of Consensus views for metric gathering purposes
var DefaultViews = []*view.View{
	&view.View{
		Name:        "info",
		Description: "Lotus node information",
		Measure:     LotusInfo,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Version, Commit},
	},
	&view.View{
		Measure:     ChainNodeHeight,
		Aggregation: view.LastValue(),
	},
	&view.View{
		Measure:     ChainNodeWorkerHeight,
		Aggregation: view.LastValue(),
	},
	&view.View{
		Measure:     BlockReceived,
		Aggregation: view.Count(),
	},
	&view.View{
		Measure:     BlockValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType, PeerID, ReceivedFrom},
	},
	&view.View{
		Measure:     BlockValidationSuccess,
		Aggregation: view.Count(),
	},
	&view.View{
		Measure:     MessageReceived,
		Aggregation: view.Count(),
	},
	&view.View{
		Measure:     MessageValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType, MessageFrom, MessageTo, MessageNonce},
	},
	&view.View{
		Measure:     MessageValidationSuccess,
		Aggregation: view.Count(),
	},
	&view.View{
		Measure:     PeerCount,
		Aggregation: view.LastValue(),
	},
	// All RPC related metrics should at the very least tag the RPCMethod
	&view.View{
		Measure:     RPCInvalidMethod,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{RPCMethod},
	},
	&view.View{
		Measure:     RPCRequestError,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{RPCMethod},
	},
	&view.View{
		Measure:     RPCResponseError,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{RPCMethod},
	},
}
