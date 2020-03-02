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
	MessageFrom, _  = tag.NewKey("message_from")
	MessageTo, _    = tag.NewKey("message_to")
	MessageNonce, _ = tag.NewKey("message_nonce")
)

// Measures
var (
	LotusInfo             = stats.Int64("info", "Arbitrary counter to tag lotus info to", stats.UnitDimensionless)
	ChainNodeHeight       = stats.Int64("chain/node_height", "Current Height of the node", stats.UnitDimensionless)
	ChainNodeWorkerHeight = stats.Int64("chain/node_worker_height", "Current Height of workers on the node", stats.UnitDimensionless)
	MessageAddFailure     = stats.Int64("message/add_faliure", "Counter for messages that failed to be added", stats.UnitDimensionless)
	MessageDecodeFailure  = stats.Int64("message/decode_faliure", "Counter for messages that failed to be decoded", stats.UnitDimensionless)
	PeerCount             = stats.Int64("peer/count", "Current number of FIL peers", stats.UnitDimensionless)
	RPCInvalidMethod      = stats.Int64("rpc/invalid_method", "Total number of invalid RPC methods called", stats.UnitDimensionless)
	RPCRequestError       = stats.Int64("rpc/request_error", "Total number of request errors handled", stats.UnitDimensionless)
	RPCResponseError      = stats.Int64("rpc/response_error", "Total number of responses errors handled", stats.UnitDimensionless)
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
		Measure:     MessageAddFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{MessageFrom, MessageTo, MessageNonce},
	},
	&view.View{
		Measure:     MessageDecodeFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{PeerID},
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
