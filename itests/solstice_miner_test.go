package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMigrationNV29Solstice verifies the FIP-0118 (Solstice) miner compatibility across the NV28→NV29
// migration on a real chain:
//
//   - D1: legacy (pre-upgrade) CC sectors keep their QA power across the migration — the change is
//     non-retroactive, so they do NOT get bumped to 10x. Their DealWeight/VerifiedDealWeight/Flags
//     are preserved byte-for-byte (v19 has no miner migration).
//   - D2: after the migration, legacy claims become historical: the read paths used by the CLI
//     (StateMinerSectors / StateSectorGetInfo / proving-deadline) keep working and report the
//     sector's original weights; the miner keeps submitting WindowPoSt without error.
//   - D3: new CC sectors onboarded on NV29 automatically receive 10x QA power via FULL_QA_POWER,
//     regardless of content (an empty CC sector is enough).
func TestMigrationNV29Solstice(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		// NV28→NV29 upgrade height. Generous so the legacy CC sector can be proven to activate on
		// NV28 (its activation epoch is set by ProveCommit) and gain its first power well before the
		// fork.
		upgradeEpoch = abi.ChainEpoch(2000)
	)

	// Standard 2KiB seal proof type; the same proof type is used for pre- and post-upgrade CC sectors.
	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	client, _, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1}, // genesis is NV28
			stmgr.Upgrade{
				Network: network.Version29,
				Height:  upgradeEpoch,
				// The neutral bootstrap has no distribution writer / SRA dependencies, so the reward
				// migration is self-contained and cannot fail on address validation.
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)

	// Add a manually-managed miner so we can onboard CC sectors through the fast (mock-proof) path.
	// The miner is only *registered* here; it is actually created (CreateMiner) in Start(), which
	// must run while the chain is already mining so the CreateMiner message can confirm.
	um, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(defaultSectorSize),
		kit.OwnerAddr(client.DefaultKey),
	)
	defer um.Stop()

	blockMiners := ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()
	blockMiners[0].WatchMinerForPost(um.ActorAddr)

	maddr := um.ActorAddr

	// ---- D1: onboard a legacy CC sector on NV28 and prove it is a pre-upgrade sector.
	scc, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(scc, 1)

	// The sector is a legacy sector iff it was activated before the upgrade epoch (activation epoch
	// is fixed by ProveCommit, which is submitted on NV28).
	preInfo, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(preInfo)
	req.Less(preInfo.Activation, upgradeEpoch, "legacy sector must be activated before the NV29 upgrade")
	req.Zero(preInfo.Flags&miner.FULL_QA_POWER, "legacy CC sector must not carry FULL_QA_POWER")

	// Wait for the sector to gain power (first successful WindowPoSt). A pre-upgrade sector always
	// contributes 1x QA power (raw size), so this must be 2048 — not 10x.
	um.WaitTillActivatedAndAssertPower(scc, uint64(defaultSectorSize), uint64(defaultSectorSize))

	// Freeze the legacy on-chain sector info and miner power.
	preInfo, err = client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	prePower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	// Cross the migration.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// D1: after migration the legacy sector keeps its weights/flags and QA power (non-retroactive).
	postInfo, err := client.StateSectorGetInfo(ctx, maddr, scc[0], head.Key())
	req.NoError(err)
	req.NotNil(postInfo)
	req.Equal(preInfo.DealWeight, postInfo.DealWeight, "legacy DealWeight must be preserved across migration")
	req.Equal(preInfo.VerifiedDealWeight, postInfo.VerifiedDealWeight, "legacy VerifiedDealWeight must be preserved across migration")
	req.Equal(preInfo.Flags, postInfo.Flags, "legacy sector Flags must be preserved across migration")
	req.Equal(preInfo.Activation, postInfo.Activation)
	req.Equal(preInfo.Expiration, postInfo.Expiration)

	postPower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)
	// Definitively: the migrated legacy sector is still 1x (raw size), NOT retroactively 10x.
	req.Equal(uint64(defaultSectorSize), postPower.MinerPower.QualityAdjPower.Uint64(),
		"legacy CC sector must not be bumped to 10x by the migration (FIP-0118 is non-retroactive)")
	req.Equal(prePower.MinerPower.QualityAdjPower.String(), postPower.MinerPower.QualityAdjPower.String(),
		"legacy sector QA power must be unchanged across the migration")

	// ---- D2: legacy claims are historical; CLI read paths keep working and report original weights.
	sectors, err := client.StateMinerSectors(ctx, maddr, nil, head.Key())
	req.NoError(err)
	req.Len(sectors, 1)
	req.Equal(scc[0], sectors[0].SectorNumber)
	req.Zero(sectors[0].Flags&miner.FULL_QA_POWER, "legacy claim must still read as a simple-power sector")
	req.Equal(postInfo.VerifiedDealWeight, sectors[0].VerifiedDealWeight)

	// The miner keeps operating: proving-deadline state is readable and WindowPoSt has no errors.
	dl, err := client.StateMinerProvingDeadline(ctx, maddr, head.Key())
	req.NoError(err)
	req.NotNil(dl)
	deadlines, err := client.StateMinerDeadlines(ctx, maddr, head.Key())
	req.NoError(err)
	req.NotNil(deadlines)
	um.AssertNoWindowPostError()

	// ---- D3: a new CC sector onboarded on NV29 automatically gets 10x QA power.
	//
	// FIP-0118 gives every new sector FULL_QA_POWER regardless of content, empty CC included, so
	// the miner actor sets the flag at activation and the sector carries qa_power_max.
	snew, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(snew, 1)

	// Cumulative power: the legacy sector keeps its pre-upgrade 1x (nothing migrates existing
	// sectors), while the new CC sector contributes 10x.
	um.WaitTillActivatedAndAssertPower(snew,
		uint64(defaultSectorSize)*2,      // raw 4096
		uint64(defaultSectorSize)*(1+10), // qa 22528: legacy 1x + new 10x
	)

	newInfo, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(newInfo)
	req.NotZero(newInfo.Flags&miner.FULL_QA_POWER, "new NV29 sector must carry FULL_QA_POWER (FIP-0118)")
	req.True(newInfo.DealWeight.NilOrZero(), "new NV29 sector DealWeight must be zero (FIP-0118)")

	// prePower snapshots the legacy sector, which is unchanged, so the delta is the new sector's QAP.
	finalPower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*10,
		finalPower.MinerPower.QualityAdjPower.Uint64()-prePower.MinerPower.QualityAdjPower.Uint64(),
		"new NV29 CC sector QA power must be 10x raw size")
}
