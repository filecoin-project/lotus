package itests

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/itests/kit"
	bminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestPledgeSectors(t *testing.T) {
	kit.QuietMiningLogs()

	t.Run("1", func(t *testing.T) {
		runPledgeSectorTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 1)
	})

	t.Run("100", func(t *testing.T) {
		runPledgeSectorTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 100)
	})

	t.Run("1000", func(t *testing.T) {
		if testing.Short() { // takes ~16s
			t.Skip("skipping test in short mode")
		}

		runPledgeSectorTest(t, kit.MockMinerBuilder, 50*time.Millisecond, 1000)
	})
}

func runPledgeSectorTest(t *testing.T, b kit.APIBuilder, blocktime time.Duration, nSectors int) {
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
