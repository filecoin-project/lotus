package itests

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestFEVMEvents does a basic events smoke test.
func TestFEVMEvents(t *testing.T) {
	require := require.New(t)

	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	// See https://github.com/filecoin-project/builtin-actors/blob/next/actors/evm/tests/events.rs#L12
	contractHex, err := os.ReadFile("contracts/events.bin")
	require.NoError(err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(err)

	result := client.EVM().DeployContract(ctx, fromAddr, contract)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(err)
	t.Logf("actor ID address is %s", idAddr)

	var (
		earliest = "earliest"
		latest   = "latest"
	)

	// Install a filter.
	filter, err := client.EthNewFilter(ctx, &ethtypes.EthFilterSpec{
		FromBlock: &earliest,
		ToBlock:   &latest,
	})
	require.NoError(err)

	// No logs yet.
	res, err := client.EthGetFilterLogs(ctx, filter)
	require.NoError(err)
	require.Empty(res.Results)

	// log a zero topic event with data
	ret, err := client.EVM().InvokeSolidity(ctx, fromAddr, idAddr, []byte{0x00, 0x00, 0x00, 0x00}, nil)
	require.NoError(err)
	require.True(ret.Receipt.ExitCode.IsSuccess(), "contract execution failed")
	require.NotNil(ret.Receipt.EventsRoot)
	fmt.Println(ret)
	fmt.Printf("Events:\n %+v\n", client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot))

	// log a zero topic event with no data
	ret, err = client.EVM().InvokeSolidity(ctx, fromAddr, idAddr, []byte{0x00, 0x00, 0x00, 0x01}, nil)
	require.NoError(err)
	require.True(ret.Receipt.ExitCode.IsSuccess(), "contract execution failed")
	fmt.Println(ret)
	fmt.Printf("Events:\n %+v\n", client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot))

	// log a four topic event with data
	ret, err = client.EVM().InvokeSolidity(ctx, fromAddr, idAddr, []byte{0x00, 0x00, 0x00, 0x02}, nil)
	require.NoError(err)
	require.True(ret.Receipt.ExitCode.IsSuccess(), "contract execution failed")
	fmt.Println(ret)
	fmt.Printf("Events:\n %+v\n", client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot))
}
