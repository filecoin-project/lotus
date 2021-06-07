package kit

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/stretchr/testify/require"
)

func PledgeSectors(t *testing.T, ctx context.Context, miner TestMiner, n, existing int, blockNotif <-chan struct{}) {
	toCheck := StartPledge(t, ctx, miner, n, existing, blockNotif)

	for len(toCheck) > 0 {
		flushSealingBatches(t, ctx, miner)

		states := map[api.SectorState]int{}
		for n := range toCheck {
			st, err := miner.SectorsStatus(ctx, n, false)
			require.NoError(t, err)
			states[st.State]++
			if st.State == api.SectorState(sealing.Proving) {
				delete(toCheck, n)
			}
			if strings.Contains(string(st.State), "Fail") {
				t.Fatal("sector in a failed state", st.State)
			}
		}

		build.Clock.Sleep(100 * time.Millisecond)
		fmt.Printf("WaitSeal: %d %+v\n", len(toCheck), states)
	}
}

func flushSealingBatches(t *testing.T, ctx context.Context, miner TestMiner) {
	pcb, err := miner.SectorPreCommitFlush(ctx)
	require.NoError(t, err)
	if pcb != nil {
		fmt.Printf("PRECOMMIT BATCH: %+v\n", pcb)
	}

	cb, err := miner.SectorCommitFlush(ctx)
	require.NoError(t, err)
	if cb != nil {
		fmt.Printf("COMMIT BATCH: %+v\n", cb)
	}
}

func StartPledge(t *testing.T, ctx context.Context, miner TestMiner, n, existing int, blockNotif <-chan struct{}) map[abi.SectorNumber]struct{} {
	for i := 0; i < n; i++ {
		if i%3 == 0 && blockNotif != nil {
			<-blockNotif
			t.Log("WAIT")
		}
		t.Logf("PLEDGING %d", i)
		_, err := miner.PledgeSector(ctx)
		require.NoError(t, err)
	}

	for {
		s, err := miner.SectorsList(ctx) // Note - the test builder doesn't import genesis sectors into FSM
		require.NoError(t, err)
		fmt.Printf("Sectors: %d\n", len(s))
		if len(s) >= n+existing {
			break
		}

		build.Clock.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("All sectors is fsm\n")

	s, err := miner.SectorsList(ctx)
	require.NoError(t, err)

	toCheck := map[abi.SectorNumber]struct{}{}
	for _, number := range s {
		toCheck[number] = struct{}{}
	}

	return toCheck
}
