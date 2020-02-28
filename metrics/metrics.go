package metrics

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// Global Tags
var (
	Version, _   = tag.NewKey("version")
	Commit, _    = tag.NewKey("commit")
	RPCMethod, _ = tag.NewKey("method")
)

// Measures
var (
	LotusInfo        = stats.Int64("info", "Arbitrary counter to tag lotus info to", stats.UnitDimensionless)
	ChainHeight      = stats.Int64("chain/height", "Current Height of the chain", stats.UnitDimensionless)
	ChainNodeHeight  = stats.Int64("chain/node_height", "Current Height of the node", stats.UnitDimensionless)
	PeerCount        = stats.Int64("peer/count", "Current number of FIL peers", stats.UnitDimensionless)
	RPCInvalidMethod = stats.Int64("rpc/invalid_method", "Total number of invalid RPC methods called", stats.UnitDimensionless)
	RPCRequestError  = stats.Int64("rpc/request_error", "Total number of request errors handled", stats.UnitDimensionless)
	RPCResponseError = stats.Int64("rpc/response_error", "Total number of responses errors handled", stats.UnitDimensionless)
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
		Measure:     ChainHeight,
		Aggregation: view.LastValue(),
	},
	&view.View{
		Measure:     ChainNodeHeight,
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
	&view.View{
		Measure:     PeerCount,
		Aggregation: view.LastValue(),
	},
}
