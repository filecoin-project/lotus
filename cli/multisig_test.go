package cli

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/test"
	clitest "github.com/filecoin-project/lotus/cli/test"
	builder "github.com/filecoin-project/lotus/node/test"
)

// TestMultisig does a basic test to exercise the multisig CLI
// commands
func TestMultisig(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, _ := startNodes(ctx, t, blocktime)
	clientNode := nodes[0]
	clitest.RunMultisigTest(t, Commands, clientNode)
}

func startNodes(ctx context.Context, t *testing.T, blocktime time.Duration) ([]test.TestNode, []address.Address) {
	n, sn := builder.RPCMockSbBuilder(t, test.OneFull, test.OneMiner)

	full := n[0]
	miner := sn[0]

	// Get everyone connected
	addrs, err := full.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrs); err != nil {
		t.Fatal(err)
	}

	// Start mining blocks
	bm := test.NewBlockMiner(ctx, t, miner, blocktime)
	bm.MineBlocks()

	// Get the creator's address
	creatorAddr, err := full.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create mock CLI
	return n, []address.Address{creatorAddr}
}
