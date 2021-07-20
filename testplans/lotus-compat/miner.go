package main

import (
	"context"
	"log"
	"os"
	"path"
	"sync"

	"github.com/testground/sdk-go/runtime"
)

func startMinerStack(ctx context.Context, runenv *runtime.RunEnv) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	wg.Add(5)
	go func() {
		runLotusDaemon(ctx, runenv, home)
		wg.Done()
	}()

	go func() {
		runLotusMiner(ctx, runenv, home)
		wg.Done()
	}()

	go func() {
		publishDealsPeriodicallyCmd(ctx)
		wg.Done()
	}()

	// setup a signal handler to cancel the context
	select {
	case <-ctx.Done():
	}

	wg.Wait()
}

func runLotusDaemon(ctx context.Context, runenv *runtime.RunEnv, home string) {
	lotusBinary := path.Join(minerStackVersion, "lotus")
	cmds := [][]string{
		{lotusBinary, "daemon", "--genesis=dev.gen", "--bootstrap=false"},
	}

	runCmdsWithLog(ctx, runenv, path.Join(runenv.TestOutputsPath, "lotus-daemon"), cmds)
}

func runLotusMiner(ctx context.Context, runenv *runtime.RunEnv, home string) {
	cmds := [][]string{
		{path.Join(minerStackVersion, "lotus"), "wait-api"}, // wait for lotus node to run

		{path.Join(minerStackVersion, "lotus-miner"), "run", "--nosync"},
	}

	runCmdsWithLog(ctx, runenv, path.Join(runenv.TestOutputsPath, "lotus-miner"), cmds)
}
