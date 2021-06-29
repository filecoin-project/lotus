package kit

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/wallet"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/miner"
	libp2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// TestMiner represents a miner enrolled in an Ensemble.
type TestMiner struct {
	api.StorageMiner

	t *testing.T

	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr

	ActorAddr address.Address
	OwnerKey  *wallet.Key
	MineOne   func(context.Context, miner.MineReq) error
	Stop      func(context.Context) error

	FullNode   *TestFullNode
	PresealDir string

	Libp2p struct {
		PeerID  peer.ID
		PrivKey libp2pcrypto.PrivKey
	}

	options nodeOpts
}

func (tm *TestMiner) PledgeSectors(ctx context.Context, n, existing int, blockNotif <-chan struct{}) {
	toCheck := tm.StartPledge(ctx, n, existing, blockNotif)

	for len(toCheck) > 0 {
		tm.FlushSealingBatches(ctx)

		states := map[api.SectorState]int{}
		for n := range toCheck {
			st, err := tm.StorageMiner.SectorsStatus(ctx, n, false)
			require.NoError(tm.t, err)
			states[st.State]++
			if st.State == api.SectorState(sealing.Proving) {
				delete(toCheck, n)
			}
			if strings.Contains(string(st.State), "Fail") {
				tm.t.Fatal("sector in a failed state", st.State)
			}
		}

		build.Clock.Sleep(100 * time.Millisecond)
		fmt.Printf("WaitSeal: %d %+v\n", len(toCheck), states)
	}

}

func (tm *TestMiner) StartPledge(ctx context.Context, n, existing int, blockNotif <-chan struct{}) map[abi.SectorNumber]struct{} {
	for i := 0; i < n; i++ {
		if i%3 == 0 && blockNotif != nil {
			<-blockNotif
			tm.t.Log("WAIT")
		}
		tm.t.Logf("PLEDGING %d", i)
		_, err := tm.StorageMiner.PledgeSector(ctx)
		require.NoError(tm.t, err)
	}

	for {
		s, err := tm.StorageMiner.SectorsList(ctx) // Note - the test builder doesn't import genesis sectors into FSM
		require.NoError(tm.t, err)
		fmt.Printf("Sectors: %d\n", len(s))
		if len(s) >= n+existing {
			break
		}

		build.Clock.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("All sectors is fsm\n")

	s, err := tm.StorageMiner.SectorsList(ctx)
	require.NoError(tm.t, err)

	toCheck := map[abi.SectorNumber]struct{}{}
	for _, number := range s {
		toCheck[number] = struct{}{}
	}

	return toCheck
}

func (tm *TestMiner) FlushSealingBatches(ctx context.Context) {
	pcb, err := tm.StorageMiner.SectorPreCommitFlush(ctx)
	require.NoError(tm.t, err)
	if pcb != nil {
		fmt.Printf("PRECOMMIT BATCH: %+v\n", pcb)
	}

	cb, err := tm.StorageMiner.SectorCommitFlush(ctx)
	require.NoError(tm.t, err)
	if cb != nil {
		fmt.Printf("COMMIT BATCH: %+v\n", cb)
	}
}
