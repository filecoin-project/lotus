package test

import (
	"context"
	"fmt"
	"github.com/filecoin-project/lotus/api"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/specs-actors/actors/abi"
	miner2 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	sealing "github.com/filecoin-project/storage-fsm"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestPledgeSector(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int) {
	os.Setenv("BELLMAN_NO_GPU", "1")

	ctx := context.Background()
	n, sn := b(t, 1, oneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)

	mine := true
	done := make(chan struct{})
	go func() {
		defer close(done)
		for mine {
			time.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, func(bool, error) {}); err != nil {
				t.Error(err)
			}
		}
	}()

	pledgeSectors(t, ctx, miner, nSectors)

	mine = false
	<-done
}

func pledgeSectors(t *testing.T, ctx context.Context, miner TestStorageNode, n int) {
	for i := 0; i < n; i++ {
		err := miner.PledgeSector(ctx)
		require.NoError(t, err)
	}

	for {
		s, err := miner.SectorsList(ctx) // Note - the test builder doesn't import genesis sectors into FSM
		require.NoError(t, err)
		fmt.Printf("Sectors: %d\n", len(s))
		if len(s) >= n {
			break
		}

		time.Sleep(100 * time.Millisecond)
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
			st, err := miner.SectorsStatus(ctx, n)
			require.NoError(t, err)
			if st.State == api.SectorState(sealing.Proving) {
				delete(toCheck, n)
			}
			if strings.Contains(string(st.State), "Fail") {
				t.Fatal("sector in a failed state", st.State)
			}
		}

		time.Sleep(100 * time.Millisecond)
		fmt.Printf("WaitSeal: %d\n", len(s))
	}
}

func TestWindowPost(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int) {
	os.Setenv("BELLMAN_NO_GPU", "1")

	ctx := context.Background()
	n, sn := b(t, 1, oneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)

	mine := true
	done := make(chan struct{})
	go func() {
		defer close(done)
		for mine {
			time.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, func(bool, error) {}); err != nil {
				t.Error(err)
			}
		}
	}()

	pledgeSectors(t, ctx, miner, nSectors)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Printf("Running one proving periods\n")

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+(miner2.WPoStProvingPeriod)+2 {
			break
		}

		if head.Height()%100 == 0 {
			fmt.Printf("@%d\n", head.Height())
		}
		time.Sleep(blocktime)
	}

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors+GenesisPreseals)))

	// TODO: Inject faults here

	mine = false
	<-done
}
