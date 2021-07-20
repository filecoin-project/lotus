package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/testground/sdk-go/runtime"
)

func startClientStackFromGenesis(ctx context.Context, runenv *runtime.RunEnv) {
	// wait for miner/fullnode to come up
	time.Sleep(20 * time.Second)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		runLotusClientDaemon(ctx, runenv)
		wg.Done()
	}()

	// TODO: add wallet?

	// setup a signal handler to cancel the context
	select {
	case <-ctx.Done():
	}

	wg.Wait()
}

func runLotusClientDaemon(ctx context.Context, runenv *runtime.RunEnv) {
	name := path.Join(runenv.TestOutputsPath, "lotus-daemon-client")

	logFile, err := os.Create(name + ".log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	cmdArgs := []string{path.Join(clientVersion, "lotus"), "daemon", "--api=1239", "--genesis=dev.gen", "--bootstrap=false"}
	runenv.RecordMessage("command for %s: %s", name, strings.Join(cmdArgs, " "))
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"LOTUS_PATH=~/.lotus-client",
	)
	// if ctx.Err()!=nil, we cancelled the command via SIGINT.
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		log.Printf("%s; check %s for details", err, logFile.Name())
	}

	multiaddr := getMultiaddr(ctx)

	cmdArgs = []string{path.Join(clientVersion, "lotus"), "net", "connect", multiaddr}
	runenv.RecordMessage("command for %s: %s", name, strings.Join(cmdArgs, " "))
	cmd = exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"LOTUS_PATH=~/.lotus-client",
	)
	// if ctx.Err()!=nil, we cancelled the command via SIGINT.
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		log.Printf("%s; check %s for details", err, logFile.Name())
	}
}

func getMultiaddr(ctx context.Context) string {
	lotusBinary := path.Join(minerStackVersion, "lotus-miner")
	importFile := fmt.Sprintf("%s net peers", lotusBinary)

	cmd := exec.CommandContext(ctx, "sh", "-c", importFile)
	result, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	res := strings.TrimSuffix(string(result), "\n")

	splitted := strings.Split(res, ",")
	multiaddr := splitted[1]
	multiaddr = strings.Trim(multiaddr, " [")
	multiaddr = strings.Trim(multiaddr, "]")
	multiaddr += "/p2p/" + splitted[0]

	return multiaddr
}
