package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	"github.com/testground/sdk-go/sync"

	mbig "math/big"

	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/lotus/testplans/lotus-soup/testkit"
)

// This is the baseline test; Filecoin 101.
//
// A network with a bootstrapper, a number of miners, and a number of clients/full nodes
// is constructed and connected through the bootstrapper.
// Some funds are allocated to each node and a number of sectors are presealed in the genesis block.
//
// The test plan:
// One or more clients store content to one or more miners, testing storage deals.
// The plan ensures that the storage deals hit the blockchain and measure the time it took.
// Verification: one or more clients retrieve and verify the hashes of stored content.
// The plan ensures that all (previously) published content can be correctly retrieved
// and measures the time it took.
//
// Preparation of the genesis block: this is the responsibility of the bootstrapper.
// In order to compute the genesis block, we need to collect identities and presealed
// sectors from each node.
// Then we create a genesis block that allocates some funds to each node and collects
// the presealed sectors.
func dealsE2E(t *testkit.TestEnvironment) error {
	// Dispatch/forward non-client roles to defaults.
	if t.Role != "client" {
		return testkit.HandleDefaultRole(t)
	}

	// This is a client role
	fastRetrieval := t.BooleanParam("fast_retrieval")
	t.RecordMessage("running client, with fast retrieval set to: %v", fastRetrieval)

	cl, err := testkit.PrepareClient(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := cl.FullApi

	// select a random miner
	minerAddr := cl.MinerAddrs[rand.Intn(len(cl.MinerAddrs))]
	if err := client.NetConnect(ctx, minerAddr.MinerNetAddrs); err != nil {
		return err
	}
	t.D().Counter(fmt.Sprintf("send-data-to,miner=%s", minerAddr.MinerActorAddr)).Inc(1)

	t.RecordMessage("selected %s as the miner", minerAddr.MinerActorAddr)

	if fastRetrieval {
		err = initPaymentChannel(t, ctx, cl, minerAddr)
		if err != nil {
			return err
		}
	}

	// give some time to the miner, otherwise, we get errors like:
	// deal errored deal failed: (State=26) error calling node: publishing deal: GasEstimateMessageGas
	// error: estimating gas used: message execution failed: exit 19, reason: failed to lock balance: failed to lock client funds: not enough balance to lock for addr t0102: escrow balance 0 < locked 0 + required 640297000 (RetCode=19)
	time.Sleep(40 * time.Second)

	time.Sleep(time.Duration(t.GlobalSeq) * 5 * time.Second)

	// generate 1600 bytes of random data
	data := make([]byte, 5000000)
	rand.New(rand.NewSource(time.Now().UnixNano())).Read(data)

	file, err := ioutil.TempFile("/tmp", "data")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	fcid, err := client.ClientImport(ctx, api.FileRef{Path: file.Name(), IsCAR: false})
	if err != nil {
		return err
	}
	t.RecordMessage("file cid: %s", fcid)

	// start deal
	t1 := time.Now()
	deal := testkit.StartDeal(ctx, minerAddr.MinerActorAddr, client, fcid.Root, fastRetrieval)
	t.RecordMessage("started deal: %s", deal)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(2 * time.Second)

	t.RecordMessage("waiting for deal to be sealed")
	testkit.WaitDealSealed(t, ctx, client, deal)
	t.D().ResettingHistogram("deal.sealed").Update(int64(time.Since(t1)))

	// wait for all client deals to be sealed before trying to retrieve
	t.SyncClient.MustSignalAndWait(ctx, sync.State("done-sealing"), t.IntParam("clients"))

	carExport := true

	t.RecordMessage("trying to retrieve %s", fcid)
	t1 = time.Now()
	_ = testkit.RetrieveData(t, ctx, client, fcid.Root, nil, carExport, data)
	t.D().ResettingHistogram("deal.retrieved").Update(int64(time.Since(t1)))

	t.SyncClient.MustSignalEntry(ctx, testkit.StateStopMining)

	time.Sleep(10 * time.Second) // wait for metrics to be emitted

	// TODO broadcast published content CIDs to other clients
	// TODO select a random piece of content published by some other client and retrieve it

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}

// filToAttoFil converts a fractional filecoin value into AttoFIL, rounding if necessary
func filToAttoFil(f float64) big.Int {
	a := mbig.NewFloat(f)
	a.Mul(a, mbig.NewFloat(float64(build.FilecoinPrecision)))
	i, _ := a.Int(nil)
	return big.Int{Int: i}
}

func initPaymentChannel(t *testkit.TestEnvironment, ctx context.Context, cl *testkit.LotusClient, minerAddr testkit.MinerAddressesMsg) error {
	recv := minerAddr
	balance := filToAttoFil(10)
	t.RecordMessage("my balance: %d", balance)
	t.RecordMessage("creating payment channel; from=%s, to=%s, funds=%d", cl.Wallet.Address, recv.WalletAddr, balance)

	channel, err := cl.FullApi.PaychGet(ctx, cl.Wallet.Address, recv.WalletAddr, balance)
	if err != nil {
		return fmt.Errorf("failed to create payment channel: %w", err)
	}

	if addr := channel.Channel; addr != address.Undef {
		return fmt.Errorf("expected an Undef channel address, got: %s", addr)
	}

	t.RecordMessage("payment channel created; msg_cid=%s", channel.WaitSentinel)
	t.RecordMessage("waiting for payment channel message to appear on chain")

	// wait for the channel creation message to appear on chain.
	_, err = cl.FullApi.StateWaitMsg(ctx, channel.WaitSentinel, 2, api.LookbackNoLimit, true)
	if err != nil {
		return fmt.Errorf("failed while waiting for payment channel creation msg to appear on chain: %w", err)
	}

	// need to wait so that the channel is tracked.
	// the full API waits for build.MessageConfidence (=1 in tests) before tracking the channel.
	// we wait for 2 confirmations, so we have the assurance the channel is tracked.

	t.RecordMessage("reloading paych; now it should have an address")
	channel, err = cl.FullApi.PaychGet(ctx, cl.Wallet.Address, recv.WalletAddr, big.Zero())
	if err != nil {
		return fmt.Errorf("failed to reload payment channel: %w", err)
	}

	t.RecordMessage("channel address: %s", channel.Channel)

	return nil
}
