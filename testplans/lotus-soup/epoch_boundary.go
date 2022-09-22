package main

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/testplans/lotus-soup/testkit"
)

// This test runs a set of miners and let them mine for some time.
// Each miner tracks the different blocks they are mining so we can
// process a posteriori the different chains they are mining.
func epochBoundary(t *testkit.TestEnvironment) error {
	t.RecordMessage("running node with role '%s'", t.Role)

	ctx := context.Background()
	// Dispatch/forward non-client roles to defaults.
	if t.Role != "miner" {
		return testkit.HandleDefaultRole(t)
	}
	// prepare miners to run.
	m, err := testkit.PrepareMiner(t)
	if err != nil {
		return err
	}
	go func() {
		miner := m.FullApi
		ch, _ := miner.ChainNotify(ctx)
		for {
			curr := <-ch
			for _, c := range curr {
				if c.Type != "apply" {
					continue
				}
				// We collect new blocks seen by the node along with its cid.
				// We can process the results a posteriori to determine the number of equivocations.
				ts := c.Val
				t.RecordMessage("New Block: height=%v, cid=%v", ts.Height(), ts.Cids())
			}
		}
	}()
	err = m.RunDefault()
	if err != nil {
		return err
	}

	// time to mine in the experiment.
	// TODO: Make this configurable and optionally make it a number of epochs.
	time.Sleep(120 * time.Second)
	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}
