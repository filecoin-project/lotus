package metrics

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	rpcmetrics "github.com/filecoin-project/go-jsonrpc/metrics"
)

// Global Tags
var (
	Version, _      = tag.NewKey("version")
	Commit, _       = tag.NewKey("commit")
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
)

var (
	InfoView = &view.View{
		Name:        "info",
		Description: "Lotus node information",
		Measure:     LotusInfo,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Version, Commit},
	}
	ChainNodeHeightView = &view.View{
		Measure:     ChainNodeHeight,
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
	MessageReceivedView = &view.View{
		Measure:     MessageReceived,
		Aggregation: view.Count(),
	}
	MessageValidationFailureView = &view.View{
		Measure:     MessageValidationFailure,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{FailureType},
	}
	MessageValidationSuccessView = &view.View{
		Measure:     MessageValidationSuccess,
		Aggregation: view.Count(),
	}
	PeerCountView = &view.View{
		Measure:     PeerCount,
		Aggregation: view.LastValue(),
	}
)

// DefaultViews is an array of OpenCensus views for metric gathering purposes
var DefaultViews = append([]*view.View{
	InfoView,
	ChainNodeHeightView,
	ChainNodeWorkerHeightView,
	BlockReceivedView,
	BlockValidationFailureView,
	BlockValidationSuccessView,
	MessageReceivedView,
	MessageValidationFailureView,
	MessageValidationSuccessView,
	PeerCountView}, rpcmetrics.DefaultViews...)
