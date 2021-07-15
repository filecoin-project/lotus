package main

import (
	"context"
	"log"
	"os"
	"path"
	"sync"

	"github.com/testground/sdk-go/runtime"
)

func startMinerStack(ctx context.Context, runenv *runtime.RunEnv, dir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	wg.Add(5)
	go func() {
		runLotusDaemon(ctx, runenv, home, dir)
		wg.Done()
	}()

	go func() {
		runLotusMiner(ctx, runenv, home, dir)
		wg.Done()
	}()

	go func() {
		publishDealsPeriodicallyCmd(ctx, dir)
		wg.Done()
	}()

	// setup a signal handler to cancel the context
	select {
	case <-ctx.Done():
	}

	wg.Wait()
}

func runLotusDaemon(ctx context.Context, runenv *runtime.RunEnv, home string, dir string) {
	cmds := [][]string{
		{path.Join(dir, "lotus"), "daemon"},
	}

	runCmdsWithLog(ctx, runenv, path.Join(runenv.TestOutputsPath, "lotus-daemon"), cmds)
}

func runLotusMiner(ctx context.Context, runenv *runtime.RunEnv, home, dir string) {
	cmds := [][]string{
		{path.Join(dir, "lotus"), "wait-api"}, // wait for lotus node to run

		{path.Join(dir, "lotus-miner"), "run", "--nosync"},
	}

	runCmdsWithLog(ctx, runenv, path.Join(runenv.TestOutputsPath, "lotus-miner"), cmds)
}
