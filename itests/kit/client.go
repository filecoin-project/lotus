package kit

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/stretchr/testify/require"
	lcli "github.com/urfave/cli/v2"
)

// RunClientTest exercises some of the Client CLI commands
func RunClientTest(t *testing.T, cmds []*lcli.Command, clientNode TestNode) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create mock CLI
	mockCLI := NewMockCLI(ctx, t, cmds)
	clientCLI := mockCLI.Client(clientNode.ListenAddr)

	// Get the Miner address
	addrs, err := clientNode.StateListMiners(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, addrs, 1)

	minerAddr := addrs[0]
	fmt.Println("Miner:", minerAddr)

	// Client query-ask <Miner addr>
	out := clientCLI.RunCmd("Client", "query-ask", minerAddr.String())
	require.Regexp(t, regexp.MustCompile("Ask:"), out)

	// Create a deal (non-interactive)
	// Client deal --start-epoch=<start epoch> <cid> <Miner addr> 1000000attofil <duration>
	res, _, err := CreateClientFile(ctx, clientNode, 1)
	require.NoError(t, err)
	startEpoch := fmt.Sprintf("--start-epoch=%d", 2<<12)
	dataCid := res.Root
	price := "1000000attofil"
	duration := fmt.Sprintf("%d", build.MinDealDuration)
	out = clientCLI.RunCmd("Client", "deal", startEpoch, dataCid.String(), minerAddr.String(), price, duration)
	fmt.Println("Client deal", out)

	// Create a deal (interactive)
	// Client deal
	// <cid>
	// <duration> (in days)
	// <Miner addr>
	// "no" (verified Client)
	// "yes" (confirm deal)
	res, _, err = CreateClientFile(ctx, clientNode, 2)
	require.NoError(t, err)
	dataCid2 := res.Root
	duration = fmt.Sprintf("%d", build.MinDealDuration/builtin.EpochsInDay)
	cmd := []string{"Client", "deal"}
	interactiveCmds := []string{
		dataCid2.String(),
		duration,
		minerAddr.String(),
		"no",
		"yes",
	}
	out = clientCLI.RunInteractiveCmd(cmd, interactiveCmds)
	fmt.Println("Client deal:\n", out)

	// Wait for provider to start sealing deal
	dealStatus := ""
	for {
		// Client list-deals
		out = clientCLI.RunCmd("Client", "list-deals")
		fmt.Println("list-deals:\n", out)

		lines := strings.Split(out, "\n")
		require.GreaterOrEqual(t, len(lines), 2)
		re := regexp.MustCompile(`\s+`)
		parts := re.Split(lines[1], -1)
		if len(parts) < 4 {
			require.Fail(t, "bad list-deals output format")
		}
		dealStatus = parts[3]
		fmt.Println("  Deal status:", dealStatus)
		if dealComplete(t, dealStatus) {
			break
		}

		time.Sleep(time.Second)
	}

	// Retrieve the first file from the Miner
	// Client retrieve <cid> <file path>
	tmpdir, err := ioutil.TempDir(os.TempDir(), "test-cli-Client")
	require.NoError(t, err)
	path := filepath.Join(tmpdir, "outfile.dat")
	out = clientCLI.RunCmd("Client", "retrieve", dataCid.String(), path)
	fmt.Println("retrieve:\n", out)
	require.Regexp(t, regexp.MustCompile("Success"), out)
}

func dealComplete(t *testing.T, dealStatus string) bool {
	switch dealStatus {
	case "StorageDealFailing", "StorageDealError":
		t.Fatal(xerrors.Errorf("Storage deal failed with status: " + dealStatus))
	case "StorageDealStaged", "StorageDealAwaitingPreCommit", "StorageDealSealing", "StorageDealActive", "StorageDealExpired", "StorageDealSlashed":
		return true
	}

	return false
}
