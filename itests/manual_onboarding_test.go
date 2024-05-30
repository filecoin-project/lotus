package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/itests/kit"
)

const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB

// Manually onboard CC sectors, bypassing lotus-miner onboarding pathways
func TestManualCCOnboarding(t *testing.T) {
	req := require.New(t)

	for _, withMockProofs := range []bool{true, false} {
		testName := "WithRealProofs"
		if withMockProofs {
			testName = "WithMockProofs"
		}
		t.Run(testName, func(t *testing.T) {
			kit.QuietMiningLogs()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var (
				// need to pick a balance value so that the test is not racy on CI by running through it's WindowPostDeadlines too fast
				blocktime = 5 * time.Millisecond
				client    kit.TestFullNode
				minerA    kit.TestMiner // A is a standard genesis miner
			)

			// Setup and begin mining with a single miner (A)
			// Miner A will only be a genesis Miner with power allocated in the genesis block and will not onboard any sectors from here on
			kitOpts := []kit.EnsembleOpt{}
			if withMockProofs {
				kitOpts = append(kitOpts, kit.MockProofs())
			}
			nodeOpts := []kit.NodeOpt{kit.SectorSize(defaultSectorSize), kit.WithAllSubsystems()}
			ens := kit.NewEnsemble(t, kitOpts...).
				FullNode(&client, nodeOpts...).
				Miner(&minerA, &client, nodeOpts...).
				Start().
				InterconnectAll()
			ens.BeginMiningMustPost(blocktime)

			// Instantiate MinerB to manually handle sector onboarding and power acquisition through sector activation.
			// Unlike other miners managed by the Lotus Miner storage infrastructure, MinerB operates independently,
			// performing all related tasks manually. Managed by the TestKit, MinerB has the capability to utilize actual proofs
			// for the processes of sector onboarding and activation.
			nodeOpts = append(nodeOpts, kit.OwnerAddr(client.DefaultKey))
			minerB, ens := ens.UnmanagedMiner(&client, nodeOpts...)
			minerC, ens := ens.UnmanagedMiner(&client, nodeOpts...)

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

			// Miner C should have no power as it has yet to onboard and activate any sectors
			minerC.AssertNoPower(ctx)

			// ---- Miner B onboards a CC sector
			var bSectorNum abi.SectorNumber
			var respCh chan kit.WindowPostResp

			if withMockProofs {
				bSectorNum, respCh = minerB.OnboardSectorWithPiecesAndMockProofs(ctx, kit.TestSpt)
			} else {
				bSectorNum, respCh = minerB.OnboardSectorWithPiecesAndRealProofs(ctx, kit.TestSpt)
			}
			// Miner B should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
			minerB.AssertNoPower(ctx)
			// Activate CC Sector for Miner B and assert power
			activateAndAssertPower(ctx, t, minerB, respCh, bSectorNum, uint64(defaultSectorSize), withMockProofs)

			// --- Miner C onboards sector with data/pieces
			var cSectorNum abi.SectorNumber
			var cRespCh chan kit.WindowPostResp

			if withMockProofs {
				cSectorNum, cRespCh = minerC.OnboardCCSectorWithMockProofs(ctx, kit.TestSpt)
			} else {
				cSectorNum, cRespCh = minerC.OnboardCCSectorWithRealProofs(ctx, kit.TestSpt)
			}
			// Miner C should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
			minerC.AssertNoPower(ctx)
			// Activate CC Sector for Miner C and assert power
			activateAndAssertPower(ctx, t, minerC, cRespCh, cSectorNum, uint64(defaultSectorSize), withMockProofs)
		})
	}
}

func activateAndAssertPower(ctx context.Context, t *testing.T, miner *kit.TestUnmanagedMiner, respCh chan kit.WindowPostResp, sector abi.SectorNumber,
	sectorSize uint64, withMockProofs bool) {
	req := require.New(t)
	// wait till sector is activated
	select {
	case resp := <-respCh:
		req.NoError(resp.Error)
		req.Equal(resp.SectorNumber, sector)
	case <-ctx.Done():
		t.Fatal("timed out waiting for sector activation")
	}

	// Fetch on-chain sector properties
	head, err := miner.FullNode.ChainHead(ctx)
	req.NoError(err)

	soi, err := miner.FullNode.StateSectorGetInfo(ctx, miner.ActorAddr, sector, head.Key())
	req.NoError(err)
	t.Logf("Miner %s SectorOnChainInfo %d: %+v", miner.ActorAddr.String(), sector, soi)

	_ = miner.FullNode.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+5))

	t.Log("Checking power after PoSt ...")

	// Miner B should now have power
	miner.AssertPower(ctx, sectorSize, sectorSize)

	if withMockProofs {
		// WindowPost Dispute should succeed as we are using mock proofs
		err := miner.SubmitPostDispute(ctx, sector)
		require.NoError(t, err)
	} else {
		// WindowPost Dispute should fail
		assertDisputeFails(ctx, t, miner, sector)
	}

}

func assertDisputeFails(ctx context.Context, t *testing.T, miner *kit.TestUnmanagedMiner, sector abi.SectorNumber) {
	err := miner.SubmitPostDispute(ctx, sector)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to dispute valid post")
	require.Contains(t, err.Error(), "(RetCode=16)")
}
