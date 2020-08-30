package cli

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"

	"github.com/multiformats/go-multiaddr"

	"github.com/filecoin-project/lotus/chain/events"

	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/chain/wallet"
	builder "github.com/filecoin-project/lotus/node/test"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func init() {
	power.ConsensusMinerMinPower = big.NewInt(2048)
	saminer.SupportedProofTypes = map[abi.RegisteredSealProof]struct{}{
		abi.RegisteredSealProof_StackedDrg2KiBV1: {},
	}
	verifreg.MinVerifiedDealSize = big.NewInt(256)
}

// TestPaymentChannels does a basic test to exercise the payment channel CLI
// commands
func TestPaymentChannels(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes, addrs := startTwoNodesOneMiner(ctx, t, blocktime)
	paymentCreator := nodes[0]
	paymentReceiver := nodes[0]
	creatorAddr := addrs[0]
	receiverAddr := addrs[1]

	// Create mock CLI
	mockCLI := newMockCLI(t)
	creatorCLI := mockCLI.client(paymentCreator.ListenAddr)
	receiverCLI := mockCLI.client(paymentReceiver.ListenAddr)

	// creator: paych get <creator> <receiver> <amount>
	channelAmt := "100000"
	cmd := []string{creatorAddr.String(), receiverAddr.String(), channelAmt}
	chstr := creatorCLI.runCmd(paychGetCmd, cmd)

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
	waitForHeight(ctx, t, paymentReceiver, chState.SettlingAt)

	// receiver: paych collect <channel>
	cmd = []string{chAddr.String()}
	receiverCLI.runCmd(paychCloseCmd, cmd)
}

type voucherSpec struct {
	serialized string
	amt        int
	lane       int
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

	// creator: paych get <creator> <receiver> <amount>
	channelAmt := "100000"
	cmd := []string{creatorAddr.String(), receiverAddr.String(), channelAmt}
	chstr := creatorCLI.runCmd(paychGetCmd, cmd)

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
	n, sn := builder.RPCMockSbBuilder(t, 2, test.OneMiner)

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
	receiverAddr, err := paymentReceiver.WalletNew(ctx, wallet.ActSigType("secp256k1"))
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
	// Create a CLI App with an --api flag so that we can specify which node
	// the command should be executed against
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:   "api",
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
	// prepend --api=<node api listener address>
	apiFlag := "--api=" + c.addr.String()
	input = append([]string{apiFlag}, input...)

	fs := c.flagSet(cmd)
	err := fs.Parse(input)
	require.NoError(c.t, err)

	err = cmd.Action(cli.NewContext(c.cctx.App, fs, c.cctx))
	require.NoError(c.t, err)

	// Get the output
	str := strings.TrimSpace(c.out.String())
	c.out.Reset()
	return str
}

func (c *mockCLIClient) flagSet(cmd *cli.Command) *flag.FlagSet {
	// Apply app level flags (so we can process --api flag)
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
	var chState paych.State
	err = store.Get(ctx, act.Head, &chState)
	require.NoError(t, err)

	return chState
}
