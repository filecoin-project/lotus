package itests

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestFEVMSmoke does a basic fevm contract installation and invocation
func TestFEVMSmoke(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create mock CLI
	mockCLI := kit.NewMockCLI(ctx, t, cli.Commands, api.NodeFull)
	clientCLI := mockCLI.Client(client.ListenAddr)

	idAddrRx := regexp.MustCompile("ID Address: (.*)")
	// Install contract
	out := clientCLI.RunCmd("chain", "create-evm-actor", "--hex", "contracts/SimpleCoin.bin")
	require.Regexp(t, idAddrRx, out)

	// Extract robust address
	idAddr := idAddrRx.FindStringSubmatch(out)[1]
	require.NotEmpty(t, idAddr)

	// Invoke contract with owner
	out = clientCLI.RunCmd("chain", "invoke-evm-actor", idAddr, "f8b2cb4f", "000000000000000000000000ff00000000000000000000000000000000000064")
	require.NotEmpty(t, out)

	outLines := strings.Split(out, "\n")
	result := outLines[len(outLines)-1]
	require.Equal(t, result, "0000000000000000000000000000000000000000000000000000000000002710")

	// Invoke the contract with non owner
	out = clientCLI.RunCmd("chain", "invoke-evm-actor", idAddr, "f8b2cb4f", "000000000000000000000000ff00000000000000000000000000000000000065")
	require.NotEmpty(t, out)

	outLines = strings.Split(out, "\n")
	result = outLines[len(outLines)-1]
	require.Equal(t, result, "0000000000000000000000000000000000000000000000000000000000000000")
}
