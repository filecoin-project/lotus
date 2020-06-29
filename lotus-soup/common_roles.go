package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/build"
	"github.com/testground/sdk-go/sync"
)

func runBootstrapper(t *TestEnvironment) error {
	t.RecordMessage("running bootstrapper")
	_, err := prepareBootstrapper(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func runMiner(t *TestEnvironment) error {
	t.RecordMessage("running miner")
	miner, err := prepareMiner(t)
	if err != nil {
		return err
	}

	t.RecordMessage("block delay: %v", build.BlockDelay)
	t.D().Gauge("miner.block-delay").Update(build.BlockDelay)

	ctx := context.Background()

	clients := t.IntParam("clients")
	miners := t.IntParam("miners")

	myActorAddr, err := miner.minerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	// mine / stop mining
	mine := true
	done := make(chan struct{})

	if miner.MineOne != nil {
		go func() {
			defer t.RecordMessage("shutting down mining")
			defer close(done)

			var i int
			for i = 0; mine; i++ {
				// synchronize all miners to mine the next block
				t.RecordMessage("synchronizing all miners to mine next block [%d]", i)
				stateMineNext := sync.State(fmt.Sprintf("mine-block-%d", i))
				t.SyncClient.MustSignalAndWait(ctx, stateMineNext, miners)

				ch := make(chan struct{})
				err := miner.MineOne(ctx, func(mined bool) {
					if mined {
						t.D().Counter(fmt.Sprintf("block.mine,miner=%s", myActorAddr)).Inc(1)
					}
					close(ch)
				})
				if err != nil {
					panic(err)
				}
				<-ch
			}

			// signal the last block to make sure no miners are left stuck waiting for the next block signal
			// while the others have stopped
			stateMineLast := sync.State(fmt.Sprintf("mine-block-%d", i))
			t.SyncClient.MustSignalEntry(ctx, stateMineLast)
		}()
	} else {
		close(done)
	}

	// wait for a signal from all clients to stop mining
	err = <-t.SyncClient.MustBarrier(ctx, stateStopMining, clients).C
	if err != nil {
		return err
	}

	mine = false
	<-done

	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}

func runDrandNode(t *TestEnvironment) error {
	t.RecordMessage("running drand node")
	dr, err := prepareDrandNode(t)
	if err != nil {
		return err
	}
	defer dr.Cleanup()

	// TODO add ability to halt / recover on demand
	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}
