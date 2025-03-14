package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/itests/kit"
)

// Manually onboard CC sectors, bypassing lotus-miner onboarding pathways
func TestManualSectorOnboarding(t *testing.T) {
	req := require.New(t)

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version23, miner.SealProofVariant_Standard)
	req.NoError(err)

	for _, withMockProofs := range []bool{true, false} {
		testName := "WithRealProofs"
		if withMockProofs {
			testName = "WithMockProofs"
		}
		t.Run(testName, func(t *testing.T) {
			if !withMockProofs {
				kit.Expensive(t)
			}
			kit.QuietMiningLogs()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var (
				// need to pick a balance value so that the test is not racy on CI by running through it's WindowPostDeadlines too fast
				blocktime = 2 * time.Millisecond
				client    kit.TestFullNode
				minerA    kit.TestMiner // A is a standard genesis miner
			)

			// Setup and begin mining with a single miner (A)
			// Miner A will only be a genesis Miner with power allocated in the genesis block and will not onboard any sectors from here on
			ens := kit.NewEnsemble(t, kit.MockProofs(withMockProofs)).
				FullNode(&client, kit.SectorSize(defaultSectorSize)).
				// preseal more than the default number of sectors to ensure that the genesis miner has power
				// because our unmanaged miners won't produce blocks so we may get null rounds
				Miner(&minerA, &client, kit.PresealSectors(5), kit.SectorSize(defaultSectorSize), kit.WithAllSubsystems()).
				Start().
				InterconnectAll()
			blockMiners := ens.BeginMiningMustPost(blocktime)
			req.Len(blockMiners, 1)
			blockMiner := blockMiners[0]

			// Instantiate MinerB to manually handle sector onboarding and power acquisition through sector activation.
			// Unlike other miners managed by the Lotus Miner storage infrastructure, MinerB operates independently,
			// performing all related tasks manually. Managed by the TestKit, MinerB has the capability to utilize actual proofs
			// for the processes of sector onboarding and activation.
			nodeOpts := []kit.NodeOpt{kit.SectorSize(defaultSectorSize), kit.OwnerAddr(client.DefaultKey)}
			minerB, ens := ens.UnmanagedMiner(ctx, &client, nodeOpts...)
			defer minerB.Stop()
			// MinerC is similar to MinerB, but onboards pieces instead of a pure CC sector
			minerC, ens := ens.UnmanagedMiner(ctx, &client, nodeOpts...)
			defer minerC.Stop()

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
			minerB.AssertNoPower()

			// Miner C should have no power as it has yet to onboard and activate any sectors
			minerC.AssertNoPower()

			// ---- Miner B onboards a CC sector
			bSectors, _ := minerB.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
			req.Len(bSectors, 1)
			// Miner B should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
			minerB.AssertNoPower()
			// Ensure that the block miner checks for and waits for posts during the appropriate proving window from our new miner with a sector
			blockMiner.WatchMinerForPost(minerB.ActorAddr)

			// --- Miner C onboards sector with data/pieces
			cSectors, _ := minerC.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
			// Miner C should still not have power as power can only be gained after sector is activated i.e. the first WindowPost is submitted for it
			minerC.AssertNoPower()
			// Ensure that the block miner checks for and waits for posts during the appropriate proving window from our new miner with a sector
			blockMiner.WatchMinerForPost(minerC.ActorAddr)

			// Wait till both miners' sectors have had their first post and are activated and check that this is reflected in miner power
			minerB.WaitTillActivatedAndAssertPower(bSectors, uint64(defaultSectorSize), uint64(defaultSectorSize))
			minerC.WaitTillActivatedAndAssertPower(cSectors, uint64(defaultSectorSize), uint64(defaultSectorSize))

			// Miner B has activated the CC sector -> upgrade it with snapdeals
			_, _ = minerB.SnapDeal(bSectors[0], kit.SectorManifest{Piece: kit.BogusPieceCid2})
		})
	}
}
