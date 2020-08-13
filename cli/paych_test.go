package cli

import (
	"bytes"
	"context"
	"flag"
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
