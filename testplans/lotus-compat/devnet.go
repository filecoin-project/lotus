package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func devnet(ctx context.Context, dir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup

	wg.Add(5)
	go func() {
		runLotusDaemon(ctx, home, dir)
		wg.Done()
	}()

	go func() {
		runLotusMiner(ctx, home, dir)
		wg.Done()
	}()

	go func() {
		publishDealsPeriodicallyCmd(ctx, dir)
		wg.Done()
	}()

	go func() {
		setSealDelayPeriodically(ctx, dir)
		wg.Done()
	}()

	go func() {
		setDefaultWalletCmd(ctx, dir)
		wg.Done()
	}()

	// setup a signal handler to cancel the context
	select {
	case <-ctx.Done():
	}

	wg.Wait()
}

func runCmdsWithLog(ctx context.Context, name string, commands [][]string) {
	logFile, err := os.Create(name + ".log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	for _, cmdArgs := range commands {
		log.Printf("command for %s: %s", name, strings.Join(cmdArgs, " "))
		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		// if ctx.Err()!=nil, we cancelled the command via SIGINT.
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			log.Printf("%s; check %s for details", err, logFile.Name())
			break
		}
	}
}

func runLotusDaemon(ctx context.Context, home string, dir string) {
	cmds := [][]string{
		{path.Join(dir, "lotus-seed"), "genesis", "new", "localnet.json"},
		{path.Join(dir, "lotus-seed"), "pre-seal", "--sector-size=2048", "--num-sectors=10"},
		{path.Join(dir, "lotus-seed"), "genesis", "add-miner", "localnet.json",
			filepath.Join(home, ".genesis-sectors", "pre-seal-t01000.json")},
		{path.Join(dir, "lotus"), "daemon", "--lotus-make-genesis=dev.gen",
			"--genesis-template=localnet.json", "--bootstrap=false"},
	}

	runCmdsWithLog(ctx, path.Join(dir, "lotus-daemon"), cmds)
}

func runLotusMiner(ctx context.Context, home, dir string) {
	cmds := [][]string{
		{path.Join(dir, "lotus"), "wait-api"}, // wait for lotus node to run

		{path.Join(dir, "lotus"), "wallet", "import",
			filepath.Join(home, ".genesis-sectors", "pre-seal-t01000.key")},
		{path.Join(dir, "lotus-miner"), "init", "--genesis-miner", "--actor=t01000", "--sector-size=2048",
			"--pre-sealed-sectors=" + filepath.Join(home, ".genesis-sectors"),
			"--pre-sealed-metadata=" + filepath.Join(home, ".genesis-sectors", "pre-seal-t01000.json"),
			"--nosync"},

		// Starting in network version 13,
		// pre-commits are batched by default,
		// and commits are aggregated by default.
		// This means deals could sit at StorageDealAwaitingPreCommit or
		// StorageDealSealing for a while, going past our 10m test timeout.
		{"sed", "-ri",
			"-e", `s/#(\s*BatchPreCommits\s*=\s*)true/ \1false/`,
			"-e", `s/#(\s*AggregateCommits\s*=\s*)true/ \1false/`,
			filepath.Join(home, ".lotusminer", "config.toml")},

		{path.Join(dir, "lotus-miner"), "run", "--nosync"},
	}

	runCmdsWithLog(ctx, path.Join(dir, "lotus-miner"), cmds)
}

func setSealDelayPeriodically(ctx context.Context, dir string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		cmd := exec.CommandContext(ctx, path.Join(dir, "lotus-miner"),
			"sectors", "set-seal-delay", "1")
		cmd.Run() // we ignore errors
	}
}

func publishDealsPeriodicallyCmd(ctx context.Context, dir string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		cmd := exec.CommandContext(ctx, path.Join(dir, "lotus-miner"),
			"storage-deals", "pending-publish", "--publish-now")
		cmd.Run() // we ignore errors
	}
}

func setDefaultWalletCmd(ctx context.Context, dir string) {
	lotusBinary := path.Join(dir, "lotus")
	setDefaultWalletCmd := fmt.Sprintf("%s wallet list | grep t3 | awk '{print $1}' | xargs %s wallet set-default", lotusBinary, lotusBinary)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", setDefaultWalletCmd)
		_, err := cmd.CombinedOutput()
		if err != nil {
			continue
		}
		// TODO: stop once we've set the default wallet once.
	}
}
