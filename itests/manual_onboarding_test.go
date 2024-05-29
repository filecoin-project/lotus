package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

const sectorSize = abi.SectorSize(2 << 10) // 2KiB

// Manually onboard CC sectors, bypassing lotus-miner onboarding pathways
func TestManualCCOnboarding(t *testing.T) {
	req := require.New(t)

	kit.QuietMiningLogs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		blocktime = 2 * time.Millisecond
		client    kit.TestFullNode
		minerA    kit.TestMiner // A is a standard genesis miner
	)

	// Setup and begin mining with a single miner (A)
	// Miner A will only be a genesis Miner with power allocated in the genesis block and will not onboard any sectors from here on
	kitOpts := []kit.EnsembleOpt{}
	nodeOpts := []kit.NodeOpt{kit.SectorSize(sectorSize), kit.WithAllSubsystems()}
	ens := kit.NewEnsemble(t, kitOpts...).
		FullNode(&client, nodeOpts...).
		Miner(&minerA, &client, nodeOpts...).
		Start().
		InterconnectAll()
	ens.BeginMining(blocktime)

	// Instantiate MinerB to manually handle sector onboarding and power acquisition through sector activation.
	// Unlike other miners managed by the Lotus Miner storage infrastructure, MinerB operates independently,
	// performing all related tasks manually. Managed by the TestKit, MinerB has the capability to utilize actual proofs
	// for the processes of sector onboarding and activation.
	nodeOpts = append(nodeOpts, kit.OwnerAddr(client.DefaultKey))
	minerB, ens := ens.UnmanagedMiner(&client, nodeOpts...)
	ens.Start()

	build.Clock.Sleep(time.Second)

	t.Log("Checking initial power ...")

	// Miner A should have power as it has already onboarded sectors in the genesis block
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	p, err := client.StateMinerPower(ctx, minerA.ActorAddr, head.Key())
	req.NoError(err)
	t.Logf("MinerA RBP: %v, QaP: %v", p.MinerPower.QualityAdjPower.String(), p.MinerPower.RawBytePower.String())

	// Miner B should have no power as it has yet to onboard and activate any sectors
	minerB.AssertNoPower(ctx)

	var bSectorNum abi.SectorNumber
	var respCh chan kit.WindowPostResp

	bSectorNum, respCh = minerB.OnboardCCSectorWithRealProofs(ctx, kit.TestSpt)

	// Miner B should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
	minerB.AssertNoPower(ctx)

	// wait till sector is activated
	select {
	case resp := <-respCh:
		req.NoError(resp.Error)
		req.Equal(resp.SectorNumber, bSectorNum)
	case <-ctx.Done():
		t.Fatal("timed out waiting for sector activation")
	}

	// Fetch on-chain sector properties
	head, err = client.ChainHead(ctx)
	req.NoError(err)

	soi, err := client.StateSectorGetInfo(ctx, minerB.ActorAddr, bSectorNum, head.Key())
	req.NoError(err)
	t.Logf("Miner B SectorOnChainInfo %d: %+v", bSectorNum, soi)

	head = client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+5))

	t.Log("Checking power after PoSt ...")

	// Miner B should now have power
	minerB.AssertPower(ctx, (uint64(2 << 10)), (uint64(2 << 10)))

	// WindowPost Dispute should fail
	assertDisputeFails(t, ctx, minerB, bSectorNum)
}

func assertDisputeFails(t *testing.T, ctx context.Context, miner *kit.TestUnmanagedMiner, sector abi.SectorNumber) {
	err := miner.SubmitPostDispute(ctx, sector)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to dispute valid post")
	require.Contains(t, err.Error(), "(RetCode=16)")
}