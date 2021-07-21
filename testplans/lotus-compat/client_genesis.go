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

func startClientStackFromGenesis(ctx context.Context, runenv *runtime.RunEnv, walletChan chan string) {
	// wait for miner/fullnode to come up
	time.Sleep(20 * time.Second)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		runLotusClientDaemon(ctx, runenv, walletChan)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		// wait for lotus client daemon to start
		time.Sleep(20 * time.Second)

		connectAndSetWallet(ctx, runenv, walletChan)
	}()

	// setup a signal handler to cancel the context
	select {
	case <-ctx.Done():
	}

	wg.Wait()
}

func connectAndSetWallet(ctx context.Context, runenv *runtime.RunEnv, walletChan chan string) {
	name := path.Join(runenv.TestOutputsPath, "lotus-daemon-client")

	logFile, err := os.Create(name + ".log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	multiaddr := getMultiaddr(ctx)

	cmdArgs := []string{path.Join(clientVersion, "lotus"), "net", "connect", multiaddr}
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

	lotusBinary := path.Join(clientVersion, "lotus")
	walletNew := fmt.Sprintf("%s wallet new", lotusBinary)

	runenv.RecordMessage("command for %s", walletNew)
	cmd = exec.CommandContext(ctx, "sh", "-c", walletNew)
	cmd.Env = append(os.Environ(),
		"LOTUS_PATH=~/.lotus-client",
	)
	result, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	wallet := strings.TrimSuffix(string(result), "\n")
	runenv.RecordMessage("got wallet: %s", wallet)

	defaultWallet := fmt.Sprintf("%s wallet default %s", lotusBinary, wallet)

	runenv.RecordMessage("command for %s", defaultWallet)
	cmd = exec.CommandContext(ctx, "sh", "-c", defaultWallet)
	cmd.Env = append(os.Environ(),
		"LOTUS_PATH=~/.lotus-client",
	)
	_, err = cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	walletChan <- wallet

	runenv.RecordMessage("sent wallet msg across to miner")
}

func runLotusClientDaemon(ctx context.Context, runenv *runtime.RunEnv, walletChan chan string) {
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
}

func getMultiaddr(ctx context.Context) string {
	lotusBinary := path.Join(minerStackVersion, "lotus-miner")
	importFile := fmt.Sprintf("%s net peers | head -1", lotusBinary)

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
