package main

import (
	"context"
	"fmt"
	"log"
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

	dir := "/lotus-new"
	genesisCtx, genesisCancel := context.WithCancel(context.Background())
	defer genesisCancel()
	go startMinerStackFromGenesis(genesisCtx, runenv, dir)

	time.Sleep(60 * time.Second)

	//TODO: start another fullnode client - do not use the fullnode from the miner

	runenv.RecordMessage("import file...")
	datacid := importFile(ctx, dir, "/qbf10.txt")

	runenv.RecordMessage("got datacid: %s", datacid)
	dealcid := makeDeal(ctx, dir, datacid)

	runenv.RecordMessage("got dealcid: %s", dealcid)

	// wait for deal to be sealed
	time.Sleep(300 * time.Second)

	runenv.RecordMessage("retrieve file...")
	retrieveFile(ctx, dir, datacid)

	// kill miner and its full node
	runenv.RecordMessage("kill miner...")
	genesisCancel()
	time.Sleep(20 * time.Second)

	// restart miner and its full node
	runenv.RecordMessage("restart miner stack...")
	ctxAfterRst := context.Background()
	go startMinerStack(ctxAfterRst, runenv, dir)

	time.Sleep(30 * time.Second)

	runenv.RecordMessage("retrieve file again...")
	retrieveFile(ctx, dir, datacid)

	//TODO: do another storage deal and retrieval
	//TODO: do the same retrieval as above

	return nil
}

func importFile(ctx context.Context, dir string, filepath string) string {
	lotusBinary := path.Join(dir, "lotus")
	importFile := fmt.Sprintf("%s client import %s | awk '{print $4}'", lotusBinary, filepath)

	cmd := exec.CommandContext(ctx, "sh", "-c", importFile)
	result, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	datacid := strings.TrimSuffix(string(result), "\n")
	return datacid
}

func makeDeal(ctx context.Context, dir string, datacid string) string {
	lotusBinary := path.Join(dir, "lotus")
	makeDeal := fmt.Sprintf("%s client deal %s t01000 0.000000000309210552 1299217", lotusBinary, datacid)

	cmd := exec.CommandContext(ctx, "sh", "-c", makeDeal)
	result, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	dealcid := strings.TrimSuffix(string(result), "\n")
	return dealcid
}

func retrieveFile(ctx context.Context, dir string, datacid string) {
	lotusBinary := path.Join(dir, "lotus")
	makeDeal := fmt.Sprintf("%s client retrieve %s /tmp/file", lotusBinary, datacid)

	cmd := exec.CommandContext(ctx, "sh", "-c", makeDeal)
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	//TODO: compare files
}
