package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
)

var testcases = map[string]interface{}{
	"default": run.InitializedTestCaseFn(compat),
}

var (
	clientVersion     string
	minerStackVersion string
)

func main() {
	run.InvokeMap(testcases)
}

func compat(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	netclient := initCtx.NetClient

	config := &network.Config{
		Network:       "default",
		Enable:        true,
		CallbackState: "network-configured-with-policy",
		RoutingPolicy: network.AllowAll,
	}

	runenv.RecordMessage("configuring network...")
	netclient.MustConfigureNetwork(ctx, config)

	runenv.RecordMessage("running main...")

	// setting initial versions for client and miner
	clientVersion = "/lotus-new"
	minerStackVersion = "/lotus-old"

	//clientVersion = "/Users/nonsense/code/src/github.com/filecoin-project/lotus"
	//minerStackVersion = "/Users/nonsense/code/src/github.com/filecoin-project/lotus"

	genesisCtx, genesisCancel := context.WithCancel(context.Background())
	defer genesisCancel()

	clientWalletChan := make(chan string)
	go startMinerStackFromGenesis(genesisCtx, runenv, clientWalletChan)
	go startClientStackFromGenesis(genesisCtx, runenv, clientWalletChan)

	time.Sleep(130 * time.Second)

	runenv.RecordMessage("import file...")
	datacid := importFile(ctx, "/qbf10.txt")
	//datacid := importFile(ctx, "/Users/nonsense/go.mod")

	runenv.RecordMessage("got datacid: %s", datacid)
	dealcid := makeDeal(ctx, datacid)

	runenv.RecordMessage("got dealcid: %s", dealcid)

	// wait for deal to be sealed
	time.Sleep(300 * time.Second)

	runenv.RecordMessage("retrieve file...")
	retrieveFile(ctx, datacid)

	// kill miner and its full node
	runenv.RecordMessage("kill miner...")
	genesisCancel()
	time.Sleep(20 * time.Second)

	// TODO: optionally change miner or client versions

	// restart miner and its full node
	runenv.RecordMessage("restart miner stack...")
	ctxAfterRst := context.Background()
	go startMinerStack(ctxAfterRst, runenv)

	time.Sleep(30 * time.Second)

	runenv.RecordMessage("retrieve file again...")
	retrieveFile(ctx, datacid)

	//TODO: do another storage deal and retrieval

	return nil
}

func importFile(ctx context.Context, filepath string) string {
	lotusBinary := path.Join(clientVersion, "lotus")
	importFile := fmt.Sprintf("%s client import %s | awk '{print $4}'", lotusBinary, filepath)

	cmd := exec.CommandContext(ctx, "sh", "-c", importFile)
	cmd.Env = append(os.Environ(),
		"LOTUS_PATH=~/.lotus-client",
	)
	result, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	datacid := strings.TrimSuffix(string(result), "\n")
	return datacid
}

func makeDeal(ctx context.Context, datacid string) string {
	lotusBinary := path.Join(minerStackVersion, "lotus")
	makeDeal := fmt.Sprintf("%s client deal %s t01000 0.000000000309210552 1299217", lotusBinary, datacid)

	cmd := exec.CommandContext(ctx, "sh", "-c", makeDeal)
	cmd.Env = append(os.Environ(),
		"LOTUS_PATH=~/.lotus-client",
	)
	result, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	dealcid := strings.TrimSuffix(string(result), "\n")
	return dealcid
}

func retrieveFile(ctx context.Context, datacid string) {
	lotusBinary := path.Join(clientVersion, "lotus")
	makeDeal := fmt.Sprintf("%s client retrieve %s /tmp/file", lotusBinary, datacid)

	cmd := exec.CommandContext(ctx, "sh", "-c", makeDeal)
	cmd.Env = append(os.Environ(),
		"LOTUS_PATH=~/.lotus-client",
	)
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	//TODO: compare files
}
