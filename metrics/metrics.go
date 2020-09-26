package metrics

import (
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	rpcmetrics "github.com/filecoin-project/go-jsonrpc/metrics"
)

// Distribution
var defaultMillisecondsDistribution = view.Distribution(0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)

// Global Tags
var (
	Version, _      = tag.NewKey("version")
	Commit, _       = tag.NewKey("commit")
	PeerID, _       = tag.NewKey("peer_id")
	FailureType, _  = tag.NewKey("failure_type")
	Local, _        = tag.NewKey("local")
	MessageFrom, _  = tag.NewKey("message_from")
	MessageTo, _    = tag.NewKey("message_to")
	MessageNonce, _ = tag.NewKey("message_nonce")
	ReceivedFrom, _ = tag.NewKey("received_from")
)

// Measures
var (
	LotusInfo                           = stats.Int64("info", "Arbitrary counter to tag lotus info to", stats.UnitDimensionless)
	ChainNodeHeight                     = stats.Int64("chain/node_height", "Current Height of the node", stats.UnitDimensionless)
	ChainNodeHeightExpected             = stats.Int64("chain/node_height_expected", "Expected Height of the node", stats.UnitDimensionless)
	ChainNodeWorkerHeight               = stats.Int64("chain/node_worker_height", "Current Height of workers on the node", stats.UnitDimensionless)
	MessagePublished                    = stats.Int64("message/published", "Counter for total locally published messages", stats.UnitDimensionless)
	MessageReceived                     = stats.Int64("message/received", "Counter for total received messages", stats.UnitDimensionless)
	MessageValidationFailure            = stats.Int64("message/failure", "Counter for message validation failures", stats.UnitDimensionless)
	MessageValidationSuccess            = stats.Int64("message/success", "Counter for message validation successes", stats.UnitDimensionless)
	BlockPublished                      = stats.Int64("block/published", "Counter for total locally published blocks", stats.UnitDimensionless)
	BlockReceived                       = stats.Int64("block/received", "Counter for total received blocks", stats.UnitDimensionless)
	BlockValidationFailure              = stats.Int64("block/failure", "Counter for block validation failures", stats.UnitDimensionless)
	BlockValidationSuccess              = stats.Int64("block/success", "Counter for block validation successes", stats.UnitDimensionless)
	BlockValidationDurationMilliseconds = stats.Float64("block/validation_ms", "Duration for Block Validation in ms", stats.UnitMilliseconds)
	PeerCount                           = stats.Int64("peer/count", "Current number of FIL peers", stats.UnitDimensionless)
	PubsubPublishMessage                = stats.Int64("pubsub/published", "Counter for total published messages", stats.UnitDimensionless)
	PubsubDeliverMessage                = stats.Int64("pubsub/delivered", "Counter for total delivered messages", stats.UnitDimensionless)
	PubsubRejectMessage                 = stats.Int64("pubsub/rejected", "Counter for total rejected messages", stats.UnitDimensionless)
	PubsubDuplicateMessage              = stats.Int64("pubsub/duplicate", "Counter for total duplicate messages", stats.UnitDimensionless)
	PubsubRecvRPC                       = stats.Int64("pubsub/recv_rpc", "Counter for total received RPCs", stats.UnitDimensionless)
	PubsubSendRPC                       = stats.Int64("pubsub/send_rpc", "Counter for total sent RPCs", stats.UnitDimensionless)
	PubsubDropRPC                       = stats.Int64("pubsub/drop_rpc", "Counter for total dropped RPCs", stats.UnitDimensionless)
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
)

// DefaultViews is an array of OpenCensus views for metric gathering purposes
var DefaultViews = append([]*view.View{
	InfoView,
	ChainNodeHeightView,
	ChainNodeHeightExpectedView,
	ChainNodeWorkerHeightView,
	BlockReceivedView,
	BlockValidationFailureView,
	BlockValidationSuccessView,
	BlockValidationDurationView,
	MessagePublishedView,
	MessageReceivedView,
	MessageValidationFailureView,
	MessageValidationSuccessView,
	PeerCountView,
	PubsubPublishMessageView,
	PubsubDeliverMessageView,
	PubsubRejectMessageView,
	PubsubDuplicateMessageView,
	PubsubRecvRPCView,
	PubsubSendRPCView,
	PubsubDropRPCView,
},
	rpcmetrics.DefaultViews...)

// SinceInMilliseconds returns the duration of time since the provide time as a float64.
func SinceInMilliseconds(startTime time.Time) float64 {
	return float64(time.Since(startTime).Nanoseconds()) / 1e6
}
