package itests

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/itests/kit"
	bminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/stretchr/testify/require"
)

func TestPledgeSectors(t *testing.T) {
	kit.QuietMiningLogs()

	runTest := func(t *testing.T, b kit.APIBuilder, blocktime time.Duration, nSectors int) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		n, sn := b(t, kit.OneFull, kit.OneMiner)
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

		kit.PledgeSectors(t, ctx, miner, nSectors, 0, nil)

		atomic.StoreInt64(&mine, 0)
		<-done
	}

	t.Run("1", func(t *testing.T) {
		runTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 1)
	})

	t.Run("100", func(t *testing.T) {
		runTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 100)
	})

	t.Run("1000", func(t *testing.T) {
		if testing.Short() { // takes ~16s
			t.Skip("skipping test in short mode")
		}

		runTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 1000)
	})
}

func TestPledgeBatching(t *testing.T) {
	runTest := func(t *testing.T, b kit.APIBuilder, blocktime time.Duration, nSectors int) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		n, sn := b(t, []kit.FullNodeOpts{kit.FullNodeWithLatestActorsAt(-1)}, kit.OneMiner)
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

		toCheck := kit.StartPledge(t, ctx, miner, nSectors, 0, nil)

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

	t.Run("100", func(t *testing.T) {
		runTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 100)
	})
}

func TestPledgeBeforeNv13(t *testing.T) {
	runTest := func(t *testing.T, b kit.APIBuilder, blocktime time.Duration, nSectors int) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		n, sn := b(t, []kit.FullNodeOpts{
			{
				Opts: func(nodes []kit.TestFullNode) node.Option {
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
		}, kit.OneMiner)
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

		toCheck := kit.StartPledge(t, ctx, miner, nSectors, 0, nil)

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

	t.Run("100-before-nv13", func(t *testing.T) {
		runTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 100)
	})
}
