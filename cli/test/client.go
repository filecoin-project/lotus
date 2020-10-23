package test

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

	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/stretchr/testify/require"
	lcli "github.com/urfave/cli/v2"
)

// RunClientTest exercises some of the client CLI commands
func RunClientTest(t *testing.T, cmds []*lcli.Command, clientNode test.TestNode) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create mock CLI
	mockCLI := newMockCLI(t, cmds)
	clientCLI := mockCLI.client(clientNode.ListenAddr)

	// Get the miner address
	addrs, err := clientNode.StateListMiners(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, addrs, 1)

	minerAddr := addrs[0]
	fmt.Println("Miner:", minerAddr)

	// client query-ask <miner addr>
	cmd := []string{
		"client", "query-ask", minerAddr.String(),
	}
	out := clientCLI.runCmd(cmd)
	require.Regexp(t, regexp.MustCompile("Ask:"), out)

	// Create a deal (non-interactive)
	// client deal <cid> <miner addr> 1000000attofil <duration>
	res, _, err := test.CreateClientFile(ctx, clientNode, 1)
	require.NoError(t, err)
	dataCid := res.Root
	price := "1000000attofil"
	duration := fmt.Sprintf("%d", build.MinDealDuration)
	cmd = []string{
		"client", "deal", dataCid.String(), minerAddr.String(), price, duration,
	}
	out = clientCLI.runCmd(cmd)
	fmt.Println("client deal", out)

	// Create a deal (interactive)
	// client deal
	// <cid>
	// <duration> (in days)
	// <miner addr>
	// "no" (verified client)
	// "yes" (confirm deal)
	res, _, err = test.CreateClientFile(ctx, clientNode, 2)
	require.NoError(t, err)
	dataCid2 := res.Root
	duration = fmt.Sprintf("%d", build.MinDealDuration/builtin.EpochsInDay)
	cmd = []string{
		"client", "deal",
	}
	interactiveCmds := []string{
		dataCid2.String(),
		duration,
		minerAddr.String(),
		"no",
		"yes",
	}
	out = clientCLI.runInteractiveCmd(cmd, interactiveCmds)
	fmt.Println("client deal:\n", out)

	// Wait for provider to start sealing deal
	dealStatus := ""
	for dealStatus != "StorageDealSealing" {
		// client list-deals
		cmd = []string{"client", "list-deals"}
		out = clientCLI.runCmd(cmd)
		fmt.Println("list-deals:\n", out)

		lines := strings.Split(out, "\n")
		require.Len(t, lines, 2)
		re := regexp.MustCompile(`\s+`)
		parts := re.Split(lines[1], -1)
		if len(parts) < 4 {
			require.Fail(t, "bad list-deals output format")
		}
		dealStatus = parts[3]
		fmt.Println("  Deal status:", dealStatus)

		time.Sleep(time.Second)
	}

	// Retrieve the first file from the miner
	// client retrieve <cid> <file path>
	tmpdir, err := ioutil.TempDir(os.TempDir(), "test-cli-client")
	require.NoError(t, err)
	path := filepath.Join(tmpdir, "outfile.dat")
	cmd = []string{
		"client", "retrieve", dataCid.String(), path,
	}
	out = clientCLI.runCmd(cmd)
	fmt.Println("retrieve:\n", out)
	require.Regexp(t, regexp.MustCompile("Success"), out)
}
