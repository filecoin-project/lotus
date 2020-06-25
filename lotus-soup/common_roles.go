package main

import (
	"context"
	"math/rand"
	"time"
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

	ctx := context.Background()

	clients := t.IntParam("clients")
	miners := t.IntParam("miners")

	// mine / stop mining
	mine := true
	done := make(chan struct{})
	go func() {
		defer close(done)
		for mine {

			// synchronize all miners to mine the next block
			t.RecordMessage("synchronizing all miners to mine next block")
			t.SyncClient.MustSignalAndWait(ctx, stateMineNext, miners)

			// add some random delay to encourage a different miner winning each round
			time.Sleep(time.Duration(100 + rand.Intn(int(100*time.Millisecond))))

			err := miner.MineOne(ctx, func(bool) {
				// after a block is mined
			})
			if err != nil {
				panic(err)
			}
		}
	}()

	// wait for a signal from all clients to stop mining
	err = <-t.SyncClient.MustBarrier(ctx, stateStopMining, clients).C
	if err != nil {
		return err
	}

	mine = false
	t.RecordMessage("shutting down mining")
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
