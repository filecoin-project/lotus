package itests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// Regression check for a fix introduced in https://github.com/filecoin-project/lotus/pull/10633
func TestPledgeMaxConcurrentGet(t *testing.T) {
	require.NoError(t, os.Setenv("GET_2K_MAX_CONCURRENT", "1"))
	t.Cleanup(func() {
		require.NoError(t, os.Unsetenv("GET_2K_MAX_CONCURRENT"))
	})

	kit.QuietMiningLogs()

	blockTime := 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, miner, ens := kit.EnsembleMinimal(t, kit.NoStorage()) // no mock proofs
	ens.InterconnectAll().BeginMiningMustPost(blockTime)

	// separate sealed and storage paths so that finalize move needs to happen
	miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
		meta.CanSeal = true
	})
	miner.AddStorage(ctx, t, func(meta *storiface.LocalStorageMeta) {
		meta.CanStore = true
	})

	// NOTE: This test only repros the issue when Fetch tasks take ~10s, there's
	// no great way to do that in a non-horribly-hacky way

	/* The horribly hacky way:

	diff --git a/storage/sealer/sched_worker.go b/storage/sealer/sched_worker.go
	index 35acd755d..76faec859 100644
	--- a/storage/sealer/sched_worker.go
	+++ b/storage/sealer/sched_worker.go
	@@ -513,6 +513,10 @@ func (sw *schedWorker) startProcessingTask(req *WorkerRequest) error {
	                        tw.start()
	                        err = <-werr

	+                       if req.TaskType == sealtasks.TTFetch {
	+                               time.Sleep(10 * time.Second)
	+                       }
	+
	                        select {
	                        case req.ret <- workerResponse{err: err}:
	                        case <-req.Ctx.Done():

	*/

	miner.PledgeSectors(ctx, 3, 0, nil)
}
