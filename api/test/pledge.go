package test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	bminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestSDRUpgrade(t *testing.T, b APIBuilder, blocktime time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, []FullNodeOpts{FullNodeWithSDRAt(500, 1000)}, OneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	build.Clock.Sleep(time.Second)

	pledge := make(chan struct{})
	mine := int64(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		round := 0
		for atomic.LoadInt64(&mine) != 0 {
			build.Clock.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, bminer.MineReq{Done: func(bool, abi.ChainEpoch, error) {

			}}); err != nil {
				t.Error(err)
			}

			// 3 sealing rounds: before, during after.
			if round >= 3 {
				continue
			}

			head, err := client.ChainHead(ctx)
			assert.NoError(t, err)

			// rounds happen every 100 blocks, with a 50 block offset.
			if head.Height() >= abi.ChainEpoch(round*500+50) {
				round++
				pledge <- struct{}{}

				ver, err := client.StateNetworkVersion(ctx, head.Key())
				assert.NoError(t, err)
				switch round {
				case 1:
					assert.Equal(t, network.Version6, ver)
				case 2:
					assert.Equal(t, network.Version7, ver)
				case 3:
					assert.Equal(t, network.Version8, ver)
				}
			}

		}
	}()

	// before.
	pledgeSectors(t, ctx, miner, 9, 0, pledge)

	s, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	sort.Slice(s, func(i, j int) bool {
		return s[i] < s[j]
	})

	for i, id := range s {
		info, err := miner.SectorsStatus(ctx, id, true)
		require.NoError(t, err)
		expectProof := abi.RegisteredSealProof_StackedDrg2KiBV1
		if i >= 3 {
			// after
			expectProof = abi.RegisteredSealProof_StackedDrg2KiBV1_1
		}
		assert.Equal(t, expectProof, info.SealProof, "sector %d, id %d", i, id)
	}

	atomic.StoreInt64(&mine, 0)
	<-done
}

func TestPledgeBatching(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, []FullNodeOpts{FullNodeWithLatestActorsAt(-1)}, OneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	build.Clock.Sleep(time.Second)

	mine := int64(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for atomic.LoadInt64(&mine) != 0 {
			build.Clock.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, bminer.MineReq{Done: func(bool, abi.ChainEpoch, error) {

			}}); err != nil {
				t.Error(err)
			}
		}
	}()

	for {
		h, err := client.ChainHead(ctx)
		require.NoError(t, err)
		if h.Height() > 10 {
			break
		}
	}

	toCheck := startPledge(t, ctx, miner, nSectors, 0, nil)

	for len(toCheck) > 0 {
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
		if states[api.SectorState(sealing.SubmitPreCommitBatch)] == nSectors ||
			(states[api.SectorState(sealing.SubmitPreCommitBatch)] > 0 && states[api.SectorState(sealing.PreCommit1)] == 0 && states[api.SectorState(sealing.PreCommit2)] == 0) {
			pcb, err := miner.SectorPreCommitFlush(ctx)
			require.NoError(t, err)
			if pcb != nil {
				fmt.Printf("PRECOMMIT BATCH: %+v\n", pcb)
			}
		}

		if states[api.SectorState(sealing.SubmitCommitAggregate)] == nSectors ||
			(states[api.SectorState(sealing.SubmitCommitAggregate)] > 0 && states[api.SectorState(sealing.WaitSeed)] == 0 && states[api.SectorState(sealing.Committing)] == 0) {
			cb, err := miner.SectorCommitFlush(ctx)
			require.NoError(t, err)
			if cb != nil {
				fmt.Printf("COMMIT BATCH: %+v\n", cb)
			}
		}

		build.Clock.Sleep(100 * time.Millisecond)
		fmt.Printf("WaitSeal: %d %+v\n", len(toCheck), states)
	}

	atomic.StoreInt64(&mine, 0)
	<-done
}

func TestPledgeBeforeNv13(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, []FullNodeOpts{
		{
			Opts: func(nodes []TestNode) node.Option {
				return node.Override(new(stmgr.UpgradeSchedule), stmgr.UpgradeSchedule{{
					Network:   network.Version9,
					Height:    1,
					Migration: stmgr.UpgradeActorsV2,
				}, {
					Network:   network.Version10,
					Height:    2,
					Migration: stmgr.UpgradeActorsV3,
				}, {
					Network:   network.Version12,
					Height:    3,
					Migration: stmgr.UpgradeActorsV4,
				}, {
					Network:   network.Version13,
					Height:    1000000000,
					Migration: stmgr.UpgradeActorsV5,
				}})
			},
		},
	}, OneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	build.Clock.Sleep(time.Second)

	mine := int64(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for atomic.LoadInt64(&mine) != 0 {
			build.Clock.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, bminer.MineReq{Done: func(bool, abi.ChainEpoch, error) {

			}}); err != nil {
				t.Error(err)
			}
		}
	}()

	for {
		h, err := client.ChainHead(ctx)
		require.NoError(t, err)
		if h.Height() > 10 {
			break
		}
	}

	toCheck := startPledge(t, ctx, miner, nSectors, 0, nil)

	for len(toCheck) > 0 {
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

	atomic.StoreInt64(&mine, 0)
	<-done
}

func TestPledgeSector(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, OneFull, OneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	build.Clock.Sleep(time.Second)

	mine := int64(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for atomic.LoadInt64(&mine) != 0 {
			build.Clock.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, bminer.MineReq{Done: func(bool, abi.ChainEpoch, error) {

			}}); err != nil {
				t.Error(err)
			}
		}
	}()

	pledgeSectors(t, ctx, miner, nSectors, 0, nil)

	atomic.StoreInt64(&mine, 0)
	<-done
}

func flushSealingBatches(t *testing.T, ctx context.Context, miner TestStorageNode) {
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

func startPledge(t *testing.T, ctx context.Context, miner TestStorageNode, n, existing int, blockNotif <-chan struct{}) map[abi.SectorNumber]struct{} {
	for i := 0; i < n; i++ {
		if i%3 == 0 && blockNotif != nil {
			<-blockNotif
			log.Errorf("WAIT")
		}
		log.Errorf("PLEDGING %d", i)
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

func pledgeSectors(t *testing.T, ctx context.Context, miner TestStorageNode, n, existing int, blockNotif <-chan struct{}) {
	toCheck := startPledge(t, ctx, miner, n, existing, blockNotif)

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
