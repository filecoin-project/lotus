package kit

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/stretchr/testify/require"
	lcli "github.com/urfave/cli/v2"
)

// RunClientTest exercises some of the Client CLI commands
func RunClientTest(t *testing.T, cmds []*lcli.Command, clientNode *TestFullNode) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create mock CLI
	mockCLI := NewMockCLI(ctx, t, cmds, api.NodeFull)
	clientCLI := mockCLI.Client(clientNode.ListenAddr)

	// Get the Miner address
	addrs, err := clientNode.StateListMiners(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, addrs, 1)

	minerAddr := addrs[0]
	fmt.Println("Miner:", minerAddr)

	// client query-ask <Miner addr>
	out := clientCLI.RunCmd("client", "query-ask", minerAddr.String())
	require.Regexp(t, regexp.MustCompile("Ask:"), out)

	// Create a deal (non-interactive)
	// client deal --start-epoch=<start epoch> <cid> <miner addr> 1000000attofil <duration>
	res, _, _, err := CreateImportFile(ctx, clientNode, 1, 0)

	require.NoError(t, err)
	startEpoch := fmt.Sprintf("--start-epoch=%d", 2<<12)
	dataCid := res.Root
	price := "1000000attofil"
	duration := fmt.Sprintf("%d", build.MinDealDuration)
	out = clientCLI.RunCmd("client", "deal", startEpoch, dataCid.String(), minerAddr.String(), price, duration)
	fmt.Println("client deal", out)

	// Create a deal (interactive)
	// client deal
	// <cid>
	// <duration> (in days)
	// <miner addr>
	// "no" (verified Client)
	// "yes" (confirm deal)
	res, _, _, err = CreateImportFile(ctx, clientNode, 2, 0)
	require.NoError(t, err)
	dataCid2 := res.Root
	duration = fmt.Sprintf("%d", build.MinDealDuration/builtin.EpochsInDay)
	cmd := []string{"client", "deal"}
	interactiveCmds := []string{
		dataCid2.String(),
		duration,
		minerAddr.String(),
		"no",
		"yes",
	}
	out = clientCLI.RunInteractiveCmd(cmd, interactiveCmds)
	fmt.Println("client deal:\n", out)

	// Wait for provider to start sealing deal
	dealStatus := ""
	for {
		// client list-deals
		out = clientCLI.RunCmd("client", "list-deals")
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

		st := CategorizeDealState(dealStatus)
		require.NotEqual(t, TestDealStateFailed, st)
		if st == TestDealStateComplete {
			break
		}

		time.Sleep(time.Second)
	}

	// Retrieve the first file from the Miner
	// client retrieve <cid> <file path>
	tmpdir, err := ioutil.TempDir(os.TempDir(), "test-cli-Client")
	require.NoError(t, err)
	path := filepath.Join(tmpdir, "outfile.dat")
	out = clientCLI.RunCmd("client", "retrieve", dataCid.String(), path)
	fmt.Println("retrieve:\n", out)
	require.Regexp(t, regexp.MustCompile("Success"), out)
}

func CreateImportFile(ctx context.Context, client api.FullNode, rseed int, size int) (res *api.ImportRes, path string, data []byte, err error) {
	data, path, err = createRandomFile(rseed, size)
	if err != nil {
		return nil, "", nil, err
	}

	res, err = client.ClientImport(ctx, api.FileRef{Path: path})
	if err != nil {
		return nil, "", nil, err
	}
	return res, path, data, nil
}

func createRandomFile(rseed, size int) ([]byte, string, error) {
	if size == 0 {
		size = 1600
	}
	data := make([]byte, size)
	rand.New(rand.NewSource(int64(rseed))).Read(data)

	dir, err := ioutil.TempDir(os.TempDir(), "test-make-deal-")
	if err != nil {
		return nil, "", err
	}

	path := filepath.Join(dir, "sourcefile.dat")
	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		return nil, "", err
	}

	return data, path, nil
}
