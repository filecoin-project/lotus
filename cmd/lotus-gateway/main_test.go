package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestRunRejectsNegativeEventFilterRange(t *testing.T) {
	app := &cli.App{Commands: []*cli.Command{runCmd}}
	err := app.Run([]string{"lotus-gateway", "run", "--event-filter-max-height-range", "-1"})
	require.EqualError(t, err, "event-filter-max-height-range must be non-negative")
}
