package cli

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	clitest "github.com/filecoin-project/lotus/cli/test"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

// TestPaymentChannels does a basic test to exercise the payment channel CLI
// commands
func TestPaymentChannels(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := clitest.StartTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	paymentReceiver := nodes[1]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := clitest.NewMockCLI(t, Commands)
	creatorCLI := mockCLI.Client(paymentCreator.ListenAddr)
	receiverCLI := mockCLI.Client(paymentReceiver.ListenAddr)

	// creator: paych add-funds <creator> <receiver> <amount>
	channelAmt := "100000"
	chstr := creatorCLI.RunCmd("paych", "add-funds", creatorAddr.String(), receiverAddr.String(), channelAmt)

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	// creator: paych voucher create <channel> <amount>
	voucherAmt := 100
	vamt := strconv.Itoa(voucherAmt)
	voucher := creatorCLI.RunCmd("paych", "voucher", "create", chAddr.String(), vamt)

	// receiver: paych voucher add <channel> <voucher>
	receiverCLI.RunCmd("paych", "voucher", "add", chAddr.String(), voucher)

	// creator: paych settle <channel>
	creatorCLI.RunCmd("paych", "settle", chAddr.String())

	// Wait for the chain to reach the settle height
	chState := getPaychState(ctx, t, paymentReceiver, chAddr)
	sa, err := chState.SettlingAt()
	require.NoError(t, err)
	waitForHeight(ctx, t, paymentReceiver, sa)

	// receiver: paych collect <channel>
	receiverCLI.RunCmd("paych", "collect", chAddr.String())
}

type voucherSpec struct {
	serialized string
	amt        int
	lane       int
}

// TestPaymentChannelStatus tests the payment channel status CLI command
func TestPaymentChannelStatus(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := clitest.StartTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := clitest.NewMockCLI(t, Commands)
	creatorCLI := mockCLI.Client(paymentCreator.ListenAddr)

	// creator: paych status-by-from-to <creator> <receiver>
	out := creatorCLI.RunCmd("paych", "status-by-from-to", creatorAddr.String(), receiverAddr.String())
	fmt.Println(out)
	noChannelState := "Channel does not exist"
	require.Regexp(t, regexp.MustCompile(noChannelState), out)

	channelAmt := uint64(100)
	create := make(chan string)
	go func() {
		// creator: paych add-funds <creator> <receiver> <amount>
		create <- creatorCLI.RunCmd(
			"paych",
			"add-funds",
			creatorAddr.String(),
			receiverAddr.String(),
			fmt.Sprintf("%d", channelAmt))
	}()

	// Wait for the output to stop being "Channel does not exist"
	for regexp.MustCompile(noChannelState).MatchString(out) {
		out = creatorCLI.RunCmd("paych", "status-by-from-to", creatorAddr.String(), receiverAddr.String())
	}
	fmt.Println(out)

	// The next state should be creating channel or channel created, depending
	// on timing
	stateCreating := regexp.MustCompile("Creating channel").MatchString(out)
	stateCreated := regexp.MustCompile("Channel exists").MatchString(out)
	require.True(t, stateCreating || stateCreated)

	channelAmtAtto := types.BigMul(types.NewInt(channelAmt), types.NewInt(build.FilecoinPrecision))
	channelAmtStr := fmt.Sprintf("%d", channelAmtAtto)
	if stateCreating {
		// If we're in the creating state (most likely) the amount should be pending
		require.Regexp(t, regexp.MustCompile("Pending.*"+channelAmtStr), out)
	}

	// Wait for create channel to complete
	chstr := <-create

	out = creatorCLI.RunCmd("paych", "status", chstr)
	fmt.Println(out)
	// Output should have the channel address
	require.Regexp(t, regexp.MustCompile("Channel.*"+chstr), out)
	// Output should have confirmed amount
	require.Regexp(t, regexp.MustCompile("Confirmed.*"+channelAmtStr), out)

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	// creator: paych voucher create <channel> <amount>
	voucherAmt := uint64(10)
	creatorCLI.RunCmd("paych", "voucher", "create", chAddr.String(), fmt.Sprintf("%d", voucherAmt))

	out = creatorCLI.RunCmd("paych", "status", chstr)
	fmt.Println(out)
	voucherAmtAtto := types.BigMul(types.NewInt(voucherAmt), types.NewInt(build.FilecoinPrecision))
	voucherAmtStr := fmt.Sprintf("%d", voucherAmtAtto)
	// Output should include voucher amount
	require.Regexp(t, regexp.MustCompile("Voucher.*"+voucherAmtStr), out)
}

// TestPaymentChannelVouchers does a basic test to exercise some payment
// channel voucher commands
func TestPaymentChannelVouchers(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := clitest.StartTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	paymentReceiver := nodes[1]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := clitest.NewMockCLI(t, Commands)
	creatorCLI := mockCLI.Client(paymentCreator.ListenAddr)
	receiverCLI := mockCLI.Client(paymentReceiver.ListenAddr)

	// creator: paych add-funds <creator> <receiver> <amount>
	channelAmt := "100000"
	chstr := creatorCLI.RunCmd("paych", "add-funds", creatorAddr.String(), receiverAddr.String(), channelAmt)

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	var vouchers []voucherSpec

	// creator: paych voucher create <channel> <amount>
	// Note: implied --lane=0
	voucherAmt1 := 100
	voucher1 := creatorCLI.RunCmd("paych", "voucher", "create", chAddr.String(), strconv.Itoa(voucherAmt1))
	vouchers = append(vouchers, voucherSpec{serialized: voucher1, lane: 0, amt: voucherAmt1})

	// creator: paych voucher create <channel> <amount> --lane=5
	lane5 := "--lane=5"
	voucherAmt2 := 50
	voucher2 := creatorCLI.RunCmd("paych", "voucher", "create", lane5, chAddr.String(), strconv.Itoa(voucherAmt2))
	vouchers = append(vouchers, voucherSpec{serialized: voucher2, lane: 5, amt: voucherAmt2})

	// creator: paych voucher create <channel> <amount> --lane=5
	voucherAmt3 := 70
	voucher3 := creatorCLI.RunCmd("paych", "voucher", "create", lane5, chAddr.String(), strconv.Itoa(voucherAmt3))
	vouchers = append(vouchers, voucherSpec{serialized: voucher3, lane: 5, amt: voucherAmt3})

	// creator: paych voucher create <channel> <amount> --lane=5
	voucherAmt4 := 80
	voucher4 := creatorCLI.RunCmd("paych", "voucher", "create", lane5, chAddr.String(), strconv.Itoa(voucherAmt4))
	vouchers = append(vouchers, voucherSpec{serialized: voucher4, lane: 5, amt: voucherAmt4})

	// creator: paych voucher list <channel> --export
	list := creatorCLI.RunCmd("paych", "voucher", "list", "--export", chAddr.String())

	// Check that voucher list output is correct on creator
	checkVoucherOutput(t, list, vouchers)

	// creator: paych voucher best-spendable <channel>
	bestSpendable := creatorCLI.RunCmd("paych", "voucher", "best-spendable", "--export", chAddr.String())

	// Check that best spendable output is correct on creator
	bestVouchers := []voucherSpec{
		{serialized: voucher1, lane: 0, amt: voucherAmt1},
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// receiver: paych voucher add <voucher>
	receiverCLI.RunCmd("paych", "voucher", "add", chAddr.String(), voucher1)

	// receiver: paych voucher add <voucher>
	receiverCLI.RunCmd("paych", "voucher", "add", chAddr.String(), voucher2)

	// receiver: paych voucher add <voucher>
	receiverCLI.RunCmd("paych", "voucher", "add", chAddr.String(), voucher3)

	// receiver: paych voucher add <voucher>
	receiverCLI.RunCmd("paych", "voucher", "add", chAddr.String(), voucher4)

	// receiver: paych voucher list <channel> --export
	list = receiverCLI.RunCmd("paych", "voucher", "list", "--export", chAddr.String())

	// Check that voucher list output is correct on receiver
	checkVoucherOutput(t, list, vouchers)

	// receiver: paych voucher best-spendable <channel>
	bestSpendable = receiverCLI.RunCmd("paych", "voucher", "best-spendable", "--export", chAddr.String())

	// Check that best spendable output is correct on receiver
	bestVouchers = []voucherSpec{
		{serialized: voucher1, lane: 0, amt: voucherAmt1},
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// receiver: paych voucher submit <channel> <voucher>
	receiverCLI.RunCmd("paych", "voucher", "submit", chAddr.String(), voucher1)

	// receiver: paych voucher best-spendable <channel>
	bestSpendable = receiverCLI.RunCmd("paych", "voucher", "best-spendable", "--export", chAddr.String())

	// Check that best spendable output no longer includes submitted voucher
	bestVouchers = []voucherSpec{
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// There are three vouchers in lane 5: 50, 70, 80
	// Submit the voucher for 50. Best spendable should still be 80.
	// receiver: paych voucher submit <channel> <voucher>
	receiverCLI.RunCmd("paych", "voucher", "submit", chAddr.String(), voucher2)

	// receiver: paych voucher best-spendable <channel>
	bestSpendable = receiverCLI.RunCmd("paych", "voucher", "best-spendable", "--export", chAddr.String())

	// Check that best spendable output still includes the voucher for 80
	bestVouchers = []voucherSpec{
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// Submit the voucher for 80
	// receiver: paych voucher submit <channel> <voucher>
	receiverCLI.RunCmd("paych", "voucher", "submit", chAddr.String(), voucher4)

	// receiver: paych voucher best-spendable <channel>
	bestSpendable = receiverCLI.RunCmd("paych", "voucher", "best-spendable", "--export", chAddr.String())

	// Check that best spendable output no longer includes submitted voucher
	bestVouchers = []voucherSpec{}
	checkVoucherOutput(t, bestSpendable, bestVouchers)
}

// TestPaymentChannelVoucherCreateShortfall verifies that if a voucher amount
// is greater than what's left in the channel, voucher create fails
func TestPaymentChannelVoucherCreateShortfall(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := clitest.StartTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := clitest.NewMockCLI(t, Commands)
	creatorCLI := mockCLI.Client(paymentCreator.ListenAddr)

	// creator: paych add-funds <creator> <receiver> <amount>
	channelAmt := 100
	chstr := creatorCLI.RunCmd(
		"paych",
		"add-funds",
		creatorAddr.String(),
		receiverAddr.String(),
		fmt.Sprintf("%d", channelAmt))

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	// creator: paych voucher create <channel> <amount> --lane=1
	voucherAmt1 := 60
	lane1 := "--lane=1"
	voucher1 := creatorCLI.RunCmd(
		"paych",
		"voucher",
		"create",
		lane1,
		chAddr.String(),
		strconv.Itoa(voucherAmt1))
	fmt.Println(voucher1)

	// creator: paych voucher create <channel> <amount> --lane=2
	lane2 := "--lane=2"
	voucherAmt2 := 70
	_, err = creatorCLI.RunCmdRaw(
		"paych",
		"voucher",
		"create",
		lane2,
		chAddr.String(),
		strconv.Itoa(voucherAmt2))

	// Should fail because channel doesn't have required amount
	require.Error(t, err)

	shortfall := voucherAmt1 + voucherAmt2 - channelAmt
	require.Regexp(t, regexp.MustCompile(fmt.Sprintf("shortfall: %d", shortfall)), err.Error())
}

func checkVoucherOutput(t *testing.T, list string, vouchers []voucherSpec) {
	lines := strings.Split(list, "\n")
	listVouchers := make(map[string]string)
	for _, line := range lines {
		parts := strings.Split(line, ";")
		if len(parts) == 2 {
			serialized := strings.TrimSpace(parts[1])
			listVouchers[serialized] = strings.TrimSpace(parts[0])
		}
	}
	for _, vchr := range vouchers {
		res, ok := listVouchers[vchr.serialized]
		require.True(t, ok)
		require.Regexp(t, fmt.Sprintf("Lane %d", vchr.lane), res)
		require.Regexp(t, fmt.Sprintf("%d", vchr.amt), res)
		delete(listVouchers, vchr.serialized)
	}
	for _, vchr := range listVouchers {
		require.Fail(t, "Extra voucher "+vchr)
	}
}

// waitForHeight waits for the node to reach the given chain epoch
func waitForHeight(ctx context.Context, t *testing.T, node test.TestNode, height abi.ChainEpoch) {
	atHeight := make(chan struct{})
	chainEvents := events.NewEvents(ctx, node)
	err := chainEvents.ChainAt(func(ctx context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		close(atHeight)
		return nil
	}, nil, 1, height)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-atHeight:
	case <-ctx.Done():
	}
}

// getPaychState gets the state of the payment channel with the given address
func getPaychState(ctx context.Context, t *testing.T, node test.TestNode, chAddr address.Address) paych.State {
	act, err := node.StateGetActor(ctx, chAddr, types.EmptyTSK)
	require.NoError(t, err)

	store := cbor.NewCborStore(apibstore.NewAPIBlockstore(node))
	chState, err := paych.Load(adt.WrapStore(ctx, store), act)
	require.NoError(t, err)

	return chState
}
