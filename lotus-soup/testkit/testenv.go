package testkit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
)

type TestEnvironment struct {
	*runtime.RunEnv
	*run.InitContext
}

// workaround for default params being wrapped in quote chars
func (t *TestEnvironment) StringParam(name string) string {
	return strings.Trim(t.RunEnv.StringParam(name), "\"")
}

func (t *TestEnvironment) DurationParam(name string) time.Duration {
	d, err := time.ParseDuration(t.StringParam(name))
	if err != nil {
		panic(fmt.Errorf("invalid duration value for param '%s': %w", name, err))
	}
	return d
}

func (t *TestEnvironment) WaitUntilAllDone() {
	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, StateDone, t.TestInstanceCount)
}
