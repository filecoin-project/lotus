package harmonydb

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	"github.com/filecoin-project/lotus/metrics"
)

var (
	dbTag, _         = tag.NewKey("db_name")
	pre              = "harmonydb_base_"
	waitsBuckets     = []float64{0, 10, 20, 30, 50, 80, 130, 210, 340, 550, 890}
	whichHostBuckets = []float64{0, 1, 2, 3, 4, 5}
)

// DBMeasures groups all db metrics.
var DBMeasures = struct {
	Hits            *stats.Int64Measure
	TotalWait       *stats.Int64Measure
	Waits           prometheus.Histogram
	OpenConnections *stats.Int64Measure
	Errors          *stats.Int64Measure
	WhichHost       prometheus.Histogram
}{
	Hits:      stats.Int64(pre+"hits", "Total number of uses.", stats.UnitDimensionless),
	TotalWait: stats.Int64(pre+"total_wait", "Total delay. A numerator over hits to get average wait.", stats.UnitMilliseconds),
	Waits: prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    pre + "waits",
		Buckets: waitsBuckets,
		Help:    "The histogram of waits for query completions.",
	}),
	OpenConnections: stats.Int64(pre+"open_connections", "Total connection count.", stats.UnitDimensionless),
	Errors:          stats.Int64(pre+"errors", "Total error count.", stats.UnitDimensionless),
	WhichHost: prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    pre + "which_host",
		Buckets: whichHostBuckets,
		Help:    "The index of the hostname being used",
	}),
}

// CacheViews groups all cache-related default views.
func init() {
	metrics.RegisterViews(
		&view.View{
			Measure:     DBMeasures.Hits,
			Aggregation: view.Sum(),
			TagKeys:     []tag.Key{dbTag},
		},
		&view.View{
			Measure:     DBMeasures.TotalWait,
			Aggregation: view.Sum(),
			TagKeys:     []tag.Key{dbTag},
		},
		&view.View{
			Measure:     DBMeasures.OpenConnections,
			Aggregation: view.LastValue(),
			TagKeys:     []tag.Key{dbTag},
		},
		&view.View{
			Measure:     DBMeasures.Errors,
			Aggregation: view.Sum(),
			TagKeys:     []tag.Key{dbTag},
		},
	)
	err := prometheus.Register(DBMeasures.Waits)
	if err != nil {
		panic(err)
	}

	err = prometheus.Register(DBMeasures.WhichHost)
	if err != nil {
		panic(err)
	}
}
