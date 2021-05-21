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

func PledgeSectors(t *testing.T, ctx context.Context, miner TestMiner, n, existing int, blockNotif <-chan struct{}) { //nolint:golint
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

	for len(toCheck) > 0 {
		for n := range toCheck {
			st, err := miner.SectorsStatus(ctx, n, false)
			require.NoError(t, err)
			if st.State == api.SectorState(sealing.Proving) {
				delete(toCheck, n)
			}
			if strings.Contains(string(st.State), "Fail") {
				t.Fatal("sector in a failed state", st.State)
			}
		}

		build.Clock.Sleep(100 * time.Millisecond)
		fmt.Printf("WaitSeal: %d\n", len(s))
	}
}
