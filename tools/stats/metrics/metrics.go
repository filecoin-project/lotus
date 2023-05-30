package metrics

import (
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"

	"github.com/filecoin-project/lotus/metrics"
)

var Timer = metrics.Timer
var SinceInMilliseconds = metrics.SinceInMilliseconds

// Distribution
var (
	defaultMillisecondsDistribution = view.Distribution(0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 32, 64, 128, 256, 500, 1000, 2000, 3000, 5000, 10000, 20000, 30000, 40000, 50000, 60000)
)

// Global Tags
var ()

// Measures
var (
	TipsetCollectionHeight              = stats.Int64("tipset_collection/height", "Current Height of the node", stats.UnitDimensionless)
	TipsetCollectionHeightExpected      = stats.Int64("tipset_collection/height_expected", "Current Height of the node", stats.UnitDimensionless)
	TipsetCollectionPoints              = stats.Int64("tipset_collection/points", "Counter for total number of points collected", stats.UnitDimensionless)
	TipsetCollectionDuration            = stats.Float64("tipset_collection/total_ms", "Duration of tipset point collection", stats.UnitMilliseconds)
	TipsetCollectionBlockHeaderDuration = stats.Float64("tipset_collection/block_header_ms", "Duration of block header point collection", stats.UnitMilliseconds)
	TipsetCollectionMessageDuration     = stats.Float64("tipset_collection/message_ms", "Duration of message point collection", stats.UnitMilliseconds)
	TipsetCollectionStaterootDuration   = stats.Float64("tipset_collection/stateroot_ms", "Duration of stateroot point collection", stats.UnitMilliseconds)
	IpldStoreCacheSize                  = stats.Int64("ipld_store/cache_size", "Initialized size of the object read cache", stats.UnitDimensionless)
	IpldStoreCacheLength                = stats.Int64("ipld_store/cache_length", "Current length of object read cache", stats.UnitDimensionless)
	IpldStoreCacheHit                   = stats.Int64("ipld_store/cache_hit", "Counter for total cache hits", stats.UnitDimensionless)
	IpldStoreCacheMiss                  = stats.Int64("ipld_store/cache_miss", "Counter for total cache misses", stats.UnitDimensionless)
	IpldStoreReadDuration               = stats.Float64("ipld_store/read_ms", "Duration of object read request to lotus", stats.UnitMilliseconds)
	IpldStoreGetDuration                = stats.Float64("ipld_store/get_ms", "Duration of object get from store", stats.UnitMilliseconds)
	WriteQueueSize                      = stats.Int64("write_queue/length", "Current length of the write queue", stats.UnitDimensionless)
)

// Views
var (
	TipsetCollectionHeightView = &view.View{
		Measure:     TipsetCollectionHeight,
		Aggregation: view.LastValue(),
	}
	TipsetCollectionHeightExpectedView = &view.View{
		Measure:     TipsetCollectionHeightExpected,
		Aggregation: view.LastValue(),
	}
	TipsetCollectionPointsView = &view.View{
		Measure:     TipsetCollectionPoints,
		Aggregation: view.Sum(),
	}
	TipsetCollectionDurationView = &view.View{
		Measure:     TipsetCollectionDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	TipsetCollectionBlockHeaderDurationView = &view.View{
		Measure:     TipsetCollectionBlockHeaderDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	TipsetCollectionMessageDurationView = &view.View{
		Measure:     TipsetCollectionMessageDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	TipsetCollectionStaterootDurationView = &view.View{
		Measure:     TipsetCollectionStaterootDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	IpldStoreCacheSizeView = &view.View{
		Measure:     IpldStoreCacheSize,
		Aggregation: view.LastValue(),
	}
	IpldStoreCacheLengthView = &view.View{
		Measure:     IpldStoreCacheLength,
		Aggregation: view.LastValue(),
	}
	IpldStoreCacheHitView = &view.View{
		Measure:     IpldStoreCacheHit,
		Aggregation: view.Count(),
	}
	IpldStoreCacheMissView = &view.View{
		Measure:     IpldStoreCacheMiss,
		Aggregation: view.Count(),
	}
	IpldStoreReadDurationView = &view.View{
		Measure:     IpldStoreReadDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
	IpldStoreGetDurationView = &view.View{
		Measure:     IpldStoreGetDuration,
		Aggregation: defaultMillisecondsDistribution,
	}
)

// DefaultViews is an array of OpenCensus views for metric gathering purposes
var DefaultViews = []*view.View{
	TipsetCollectionHeightView,
	TipsetCollectionHeightExpectedView,
	TipsetCollectionPointsView,
	TipsetCollectionDurationView,
	TipsetCollectionBlockHeaderDurationView,
	TipsetCollectionMessageDurationView,
	TipsetCollectionStaterootDurationView,
	IpldStoreCacheSizeView,
	IpldStoreCacheLengthView,
	IpldStoreCacheHitView,
	IpldStoreCacheMissView,
	IpldStoreReadDurationView,
	IpldStoreGetDurationView,
}
