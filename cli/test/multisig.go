package test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/stretchr/testify/require"
	lcli "github.com/urfave/cli/v2"
)

func RunMultisigTest(t *testing.T, cmds []*lcli.Command, clientNode test.TestNode) {
	ctx := context.Background()

	// Create mock CLI
	mockCLI := newMockCLI(t, cmds)
	clientCLI := mockCLI.client(clientNode.ListenAddr)

	// Create some wallets on the node to use for testing multisig
	var walletAddrs []address.Address
	for i := 0; i < 4; i++ {
		addr, err := clientNode.WalletNew(ctx, types.KTSecp256k1)
		require.NoError(t, err)

		walletAddrs = append(walletAddrs, addr)

		test.SendFunds(ctx, t, clientNode, addr, types.NewInt(1e15))
	}

	// Create an msig with three of the addresses and threshold of two sigs
	// msig create --required=2 --duration=50 --value=1000attofil <addr1> <addr2> <addr3>
	amtAtto := types.NewInt(1000)
	threshold := 2
	paramDuration := "--duration=50"
	paramRequired := fmt.Sprintf("--required=%d", threshold)
	paramValue := fmt.Sprintf("--value=%dattofil", amtAtto)
	cmd := []string{
		"msig", "create",
		paramRequired,
		paramDuration,
		paramValue,
		walletAddrs[0].String(),
		walletAddrs[1].String(),
		walletAddrs[2].String(),
	}
	out := clientCLI.runCmd(cmd)
	fmt.Println(out)

	// Extract msig robust address from output
	expCreateOutPrefix := "Created new multisig:"
	require.Regexp(t, regexp.MustCompile(expCreateOutPrefix), out)
	parts := strings.Split(strings.TrimSpace(strings.Replace(out, expCreateOutPrefix, "", -1)), " ")
	require.Len(t, parts, 2)
	msigRobustAddr := parts[1]
	fmt.Println("msig robust address:", msigRobustAddr)

	// Propose to add a new address to the msig
	// msig add-propose --from=<addr> <msig> <addr>
	paramFrom := fmt.Sprintf("--from=%s", walletAddrs[0])
	cmd = []string{
		"msig", "add-propose",
		paramFrom,
		msigRobustAddr,
		walletAddrs[3].String(),
	}
	out = clientCLI.runCmd(cmd)
	fmt.Println(out)

	// msig inspect <msig>
	cmd = []string{"msig", "inspect", "--vesting", "--decode-params", msigRobustAddr}
	out = clientCLI.runCmd(cmd)
	fmt.Println(out)

	// Expect correct balance
	require.Regexp(t, regexp.MustCompile("Balance: 0.000000000000001 FIL"), out)
	// Expect 1 transaction
	require.Regexp(t, regexp.MustCompile(`Transactions:\s*1`), out)
	// Expect transaction to be "AddSigner"
	require.Regexp(t, regexp.MustCompile(`AddSigner`), out)

	// Approve adding the new address
	// msig add-approve --from=<addr> <msig> <addr> 0 <addr> false
	txnID := "0"
	paramFrom = fmt.Sprintf("--from=%s", walletAddrs[1])
	cmd = []string{
		"msig", "add-approve",
		paramFrom,
		msigRobustAddr,
		walletAddrs[0].String(),
		txnID,
		walletAddrs[3].String(),
		"false",
	}
	out = clientCLI.runCmd(cmd)
	fmt.Println(out)
}
