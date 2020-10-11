package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/actors/policy"

	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	builder "github.com/filecoin-project/lotus/node/test"
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

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := startTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	paymentReceiver := nodes[1]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := newMockCLI(t)
	creatorCLI := mockCLI.client(paymentCreator.ListenAddr)
	receiverCLI := mockCLI.client(paymentReceiver.ListenAddr)

	// creator: paych add-funds <creator> <receiver> <amount>
	channelAmt := "100000"
	cmd := []string{creatorAddr.String(), receiverAddr.String(), channelAmt}
	chstr := creatorCLI.runCmd(paychAddFundsCmd, cmd)

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	// creator: paych voucher create <channel> <amount>
	voucherAmt := 100
	vamt := strconv.Itoa(voucherAmt)
	cmd = []string{chAddr.String(), vamt}
	voucher := creatorCLI.runCmd(paychVoucherCreateCmd, cmd)

	// receiver: paych voucher add <channel> <voucher>
	cmd = []string{chAddr.String(), voucher}
	receiverCLI.runCmd(paychVoucherAddCmd, cmd)

	// creator: paych settle <channel>
	cmd = []string{chAddr.String()}
	creatorCLI.runCmd(paychSettleCmd, cmd)

	// Wait for the chain to reach the settle height
	chState := getPaychState(ctx, t, paymentReceiver, chAddr)
	sa, err := chState.SettlingAt()
	require.NoError(t, err)
	waitForHeight(ctx, t, paymentReceiver, sa)

	// receiver: paych collect <channel>
	cmd = []string{chAddr.String()}
	receiverCLI.runCmd(paychCloseCmd, cmd)
}

type voucherSpec struct {
	serialized string
	amt        int
	lane       int
}

// TestPaymentChannelStatus tests the payment channel status CLI command
func TestPaymentChannelStatus(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := startTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := newMockCLI(t)
	creatorCLI := mockCLI.client(paymentCreator.ListenAddr)

	cmd := []string{creatorAddr.String(), receiverAddr.String()}
	out := creatorCLI.runCmd(paychStatusByFromToCmd, cmd)
	fmt.Println(out)
	noChannelState := "Channel does not exist"
	require.Regexp(t, regexp.MustCompile(noChannelState), out)

	channelAmt := uint64(100)
	create := make(chan string)
	go func() {
		// creator: paych add-funds <creator> <receiver> <amount>
		cmd := []string{creatorAddr.String(), receiverAddr.String(), fmt.Sprintf("%d", channelAmt)}
		create <- creatorCLI.runCmd(paychAddFundsCmd, cmd)
	}()

	// Wait for the output to stop being "Channel does not exist"
	for regexp.MustCompile(noChannelState).MatchString(out) {
		cmd := []string{creatorAddr.String(), receiverAddr.String()}
		out = creatorCLI.runCmd(paychStatusByFromToCmd, cmd)
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

	cmd = []string{chstr}
	out = creatorCLI.runCmd(paychStatusCmd, cmd)
	fmt.Println(out)
	// Output should have the channel address
	require.Regexp(t, regexp.MustCompile("Channel.*"+chstr), out)
	// Output should have confirmed amount
	require.Regexp(t, regexp.MustCompile("Confirmed.*"+channelAmtStr), out)

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	// creator: paych voucher create <channel> <amount>
	voucherAmt := uint64(10)
	cmd = []string{chAddr.String(), fmt.Sprintf("%d", voucherAmt)}
	creatorCLI.runCmd(paychVoucherCreateCmd, cmd)

	cmd = []string{chstr}
	out = creatorCLI.runCmd(paychStatusCmd, cmd)
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

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := startTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	paymentReceiver := nodes[1]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := newMockCLI(t)
	creatorCLI := mockCLI.client(paymentCreator.ListenAddr)
	receiverCLI := mockCLI.client(paymentReceiver.ListenAddr)

	// creator: paych add-funds <creator> <receiver> <amount>
	channelAmt := "100000"
	cmd := []string{creatorAddr.String(), receiverAddr.String(), channelAmt}
	chstr := creatorCLI.runCmd(paychAddFundsCmd, cmd)

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	var vouchers []voucherSpec

	// creator: paych voucher create <channel> <amount>
	// Note: implied --lane=0
	voucherAmt1 := 100
	cmd = []string{chAddr.String(), strconv.Itoa(voucherAmt1)}
	voucher1 := creatorCLI.runCmd(paychVoucherCreateCmd, cmd)
	vouchers = append(vouchers, voucherSpec{serialized: voucher1, lane: 0, amt: voucherAmt1})

	// creator: paych voucher create <channel> <amount> --lane=5
	lane5 := "--lane=5"
	voucherAmt2 := 50
	cmd = []string{lane5, chAddr.String(), strconv.Itoa(voucherAmt2)}
	voucher2 := creatorCLI.runCmd(paychVoucherCreateCmd, cmd)
	vouchers = append(vouchers, voucherSpec{serialized: voucher2, lane: 5, amt: voucherAmt2})

	// creator: paych voucher create <channel> <amount> --lane=5
	voucherAmt3 := 70
	cmd = []string{lane5, chAddr.String(), strconv.Itoa(voucherAmt3)}
	voucher3 := creatorCLI.runCmd(paychVoucherCreateCmd, cmd)
	vouchers = append(vouchers, voucherSpec{serialized: voucher3, lane: 5, amt: voucherAmt3})

	// creator: paych voucher create <channel> <amount> --lane=5
	voucherAmt4 := 80
	cmd = []string{lane5, chAddr.String(), strconv.Itoa(voucherAmt4)}
	voucher4 := creatorCLI.runCmd(paychVoucherCreateCmd, cmd)
	vouchers = append(vouchers, voucherSpec{serialized: voucher4, lane: 5, amt: voucherAmt4})

	// creator: paych voucher list <channel> --export
	cmd = []string{"--export", chAddr.String()}
	list := creatorCLI.runCmd(paychVoucherListCmd, cmd)

	// Check that voucher list output is correct on creator
	checkVoucherOutput(t, list, vouchers)

	// creator: paych voucher best-spendable <channel>
	cmd = []string{"--export", chAddr.String()}
	bestSpendable := creatorCLI.runCmd(paychVoucherBestSpendableCmd, cmd)

	// Check that best spendable output is correct on creator
	bestVouchers := []voucherSpec{
		{serialized: voucher1, lane: 0, amt: voucherAmt1},
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// receiver: paych voucher add <voucher>
	cmd = []string{chAddr.String(), voucher1}
	receiverCLI.runCmd(paychVoucherAddCmd, cmd)

	// receiver: paych voucher add <voucher>
	cmd = []string{chAddr.String(), voucher2}
	receiverCLI.runCmd(paychVoucherAddCmd, cmd)

	// receiver: paych voucher add <voucher>
	cmd = []string{chAddr.String(), voucher3}
	receiverCLI.runCmd(paychVoucherAddCmd, cmd)

	// receiver: paych voucher add <voucher>
	cmd = []string{chAddr.String(), voucher4}
	receiverCLI.runCmd(paychVoucherAddCmd, cmd)

	// receiver: paych voucher list <channel> --export
	cmd = []string{"--export", chAddr.String()}
	list = receiverCLI.runCmd(paychVoucherListCmd, cmd)

	// Check that voucher list output is correct on receiver
	checkVoucherOutput(t, list, vouchers)

	// receiver: paych voucher best-spendable <channel>
	cmd = []string{"--export", chAddr.String()}
	bestSpendable = receiverCLI.runCmd(paychVoucherBestSpendableCmd, cmd)

	// Check that best spendable output is correct on receiver
	bestVouchers = []voucherSpec{
		{serialized: voucher1, lane: 0, amt: voucherAmt1},
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// receiver: paych voucher submit <channel> <voucher>
	cmd = []string{chAddr.String(), voucher1}
	receiverCLI.runCmd(paychVoucherSubmitCmd, cmd)

	// receiver: paych voucher best-spendable <channel>
	cmd = []string{"--export", chAddr.String()}
	bestSpendable = receiverCLI.runCmd(paychVoucherBestSpendableCmd, cmd)

	// Check that best spendable output no longer includes submitted voucher
	bestVouchers = []voucherSpec{
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// There are three vouchers in lane 5: 50, 70, 80
	// Submit the voucher for 50. Best spendable should still be 80.
	// receiver: paych voucher submit <channel> <voucher>
	cmd = []string{chAddr.String(), voucher2}
	receiverCLI.runCmd(paychVoucherSubmitCmd, cmd)

	// receiver: paych voucher best-spendable <channel>
	cmd = []string{"--export", chAddr.String()}
	bestSpendable = receiverCLI.runCmd(paychVoucherBestSpendableCmd, cmd)

	// Check that best spendable output still includes the voucher for 80
	bestVouchers = []voucherSpec{
		{serialized: voucher4, lane: 5, amt: voucherAmt4},
	}
	checkVoucherOutput(t, bestSpendable, bestVouchers)

	// Submit the voucher for 80
	// receiver: paych voucher submit <channel> <voucher>
	cmd = []string{chAddr.String(), voucher4}
	receiverCLI.runCmd(paychVoucherSubmitCmd, cmd)

	// receiver: paych voucher best-spendable <channel>
	cmd = []string{"--export", chAddr.String()}
	bestSpendable = receiverCLI.runCmd(paychVoucherBestSpendableCmd, cmd)

	// Check that best spendable output no longer includes submitted voucher
	bestVouchers = []voucherSpec{}
	checkVoucherOutput(t, bestSpendable, bestVouchers)
}

// TestPaymentChannelVoucherCreateShortfall verifies that if a voucher amount
// is greater than what's left in the channel, voucher create fails
func TestPaymentChannelVoucherCreateShortfall(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := startTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := newMockCLI(t)
	creatorCLI := mockCLI.client(paymentCreator.ListenAddr)

	// creator: paych add-funds <creator> <receiver> <amount>
	channelAmt := 100
	cmd := []string{creatorAddr.String(), receiverAddr.String(), fmt.Sprintf("%d", channelAmt)}
	chstr := creatorCLI.runCmd(paychAddFundsCmd, cmd)

	chAddr, err := address.NewFromString(chstr)
	require.NoError(t, err)

	// creator: paych voucher create <channel> <amount> --lane=1
	voucherAmt1 := 60
	lane1 := "--lane=1"
	cmd = []string{lane1, chAddr.String(), strconv.Itoa(voucherAmt1)}
	voucher1 := creatorCLI.runCmd(paychVoucherCreateCmd, cmd)
	fmt.Println(voucher1)

	// creator: paych voucher create <channel> <amount> --lane=2
	lane2 := "--lane=2"
	voucherAmt2 := 70
	cmd = []string{lane2, chAddr.String(), strconv.Itoa(voucherAmt2)}
	_, err = creatorCLI.runCmdRaw(paychVoucherCreateCmd, cmd)

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

func startTwoNodesOneMiner(ctx context.Context, t *testing.T, blocktime time.Duration) ([]test.TestNode, []address.Address) {
	n, sn := builder.RPCMockSbBuilder(t, test.TwoFull, test.OneMiner)

	paymentCreator := n[0]
	paymentReceiver := n[1]
	miner := sn[0]

	// Get everyone connected
	addrs, err := paymentCreator.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := paymentReceiver.NetConnect(ctx, addrs); err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrs); err != nil {
		t.Fatal(err)
	}

	// Start mining blocks
	bm := test.NewBlockMiner(ctx, t, miner, blocktime)
	bm.MineBlocks()

	// Send some funds to register the receiver
	receiverAddr, err := paymentReceiver.WalletNew(ctx, types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	test.SendFunds(ctx, t, paymentCreator, receiverAddr, abi.NewTokenAmount(1e18))

	// Get the creator's address
	creatorAddr, err := paymentCreator.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create mock CLI
	return n, []address.Address{creatorAddr, receiverAddr}
}

type mockCLI struct {
	t    *testing.T
	cctx *cli.Context
	out  *bytes.Buffer
}

func newMockCLI(t *testing.T) *mockCLI {
	// Create a CLI App with an --api-url flag so that we can specify which node
	// the command should be executed against
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:   "api-url",
			Hidden: true,
		},
	}
	var out bytes.Buffer
	app.Writer = &out
	app.Setup()

	cctx := cli.NewContext(app, &flag.FlagSet{}, nil)
	return &mockCLI{t: t, cctx: cctx, out: &out}
}

func (c *mockCLI) client(addr multiaddr.Multiaddr) *mockCLIClient {
	return &mockCLIClient{t: c.t, addr: addr, cctx: c.cctx, out: c.out}
}

// mockCLIClient runs commands against a particular node
type mockCLIClient struct {
	t    *testing.T
	addr multiaddr.Multiaddr
	cctx *cli.Context
	out  *bytes.Buffer
}

func (c *mockCLIClient) runCmd(cmd *cli.Command, input []string) string {
	out, err := c.runCmdRaw(cmd, input)
	require.NoError(c.t, err)

	return out
}

func (c *mockCLIClient) runCmdRaw(cmd *cli.Command, input []string) (string, error) {
	// prepend --api-url=<node api listener address>
	apiFlag := "--api-url=" + c.addr.String()
	input = append([]string{apiFlag}, input...)

	fs := c.flagSet(cmd)
	err := fs.Parse(input)
	require.NoError(c.t, err)

	err = cmd.Action(cli.NewContext(c.cctx.App, fs, c.cctx))

	// Get the output
	str := strings.TrimSpace(c.out.String())
	c.out.Reset()
	return str, err
}

func (c *mockCLIClient) flagSet(cmd *cli.Command) *flag.FlagSet {
	// Apply app level flags (so we can process --api-url flag)
	fs := &flag.FlagSet{}
	for _, f := range c.cctx.App.Flags {
		err := f.Apply(fs)
		if err != nil {
			c.t.Fatal(err)
		}
	}
	// Apply command level flags
	for _, f := range cmd.Flags {
		err := f.Apply(fs)
		if err != nil {
			c.t.Fatal(err)
		}
	}
	return fs
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
