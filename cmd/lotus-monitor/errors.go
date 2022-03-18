package main

import (
	"github.com/urfave/cli/v2"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	errCount     = stats.Int64("errors/count", "error count", "errors")
	errCountView = &view.View{
		Name:        "errors/count",
		Measure:     errCount,
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{},
	}
)

func init() {
	if err := view.Register(errCountView); err != nil {
		log.Fatalf("cannot register error recorder view: %w", err)
	}
}

func errorRecorder(cctx *cli.Context, errs chan error) {
	for range errs {
		stats.Record(cctx.Context, errCount.M(1))
	}
}
