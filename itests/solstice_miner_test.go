package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMigrationNV29Solstice verifies FIP-0118 miner compatibility across the NV28→NV29 migration.
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

	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	client, _, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1}, // genesis is NV28
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)

	um, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(defaultSectorSize),
		kit.OwnerAddr(client.DefaultKey),
	)
	defer um.Stop()

	blockMiners := ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()
	blockMiners[0].WatchMinerForPost(um.ActorAddr)

	maddr := um.ActorAddr

	scc, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(scc, 1)

	preInfo, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(preInfo)
	req.Less(preInfo.Activation, upgradeEpoch, "legacy sector must be activated before the NV29 upgrade")
	req.Zero(preInfo.Flags&miner.FULL_QA_POWER, "legacy CC sector must not carry FULL_QA_POWER")

	um.WaitTillActivatedAndAssertPower(scc, uint64(defaultSectorSize), uint64(defaultSectorSize))

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

	// legacy sector keeps its weights/flags and QA power (non-retroactive).
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
	req.Equal(uint64(defaultSectorSize), postPower.MinerPower.QualityAdjPower.Uint64(),
		"legacy CC sector must not be bumped to 10x by the migration (FIP-0118 is non-retroactive)")
	req.Equal(prePower.MinerPower.QualityAdjPower.String(), postPower.MinerPower.QualityAdjPower.String(),
		"legacy sector QA power must be unchanged across the migration")

	// CLI read paths keep working on migrated legacy sectors.
	sectors, err := client.StateMinerSectors(ctx, maddr, nil, head.Key())
	req.NoError(err)
	req.Len(sectors, 1)
	req.Equal(scc[0], sectors[0].SectorNumber)
	req.Zero(sectors[0].Flags&miner.FULL_QA_POWER, "legacy claim must still read as a simple-power sector")
	req.Equal(postInfo.VerifiedDealWeight, sectors[0].VerifiedDealWeight)

	dl, err := client.StateMinerProvingDeadline(ctx, maddr, head.Key())
	req.NoError(err)
	req.NotNil(dl)
	deadlines, err := client.StateMinerDeadlines(ctx, maddr, head.Key())
	req.NoError(err)
	req.NotNil(deadlines)
	um.AssertNoWindowPostError()

	// new CC sector onboarded on NV29 gets 10x QA power.
	snew, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(snew, 1)

	um.WaitTillActivatedAndAssertPower(snew,
		uint64(defaultSectorSize)*2,      // raw 4096
		uint64(defaultSectorSize)*(1+10), // qa 22528: legacy 1x + new 10x
	)

	newInfo, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(newInfo)
	req.NotZero(newInfo.Flags&miner.FULL_QA_POWER, "new NV29 sector must carry FULL_QA_POWER (FIP-0118)")
	req.True(newInfo.DealWeight.NilOrZero(), "new NV29 sector DealWeight must be zero (FIP-0118)")

	finalPower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*10,
		finalPower.MinerPower.QualityAdjPower.Uint64()-prePower.MinerPower.QualityAdjPower.Uint64(),
		"new NV29 CC sector QA power must be 10x raw size")

	// ExtendSectorExpiration2 preserves QA multiplier: legacy stays 1x, new sector keeps FULL_QA_POWER.
	d5pre, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	d5Scc, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(d5Scc)
	d5Snew, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(d5Snew)
	req.Zero(d5Scc.Flags&miner.FULL_QA_POWER, "precondition: legacy sector is still 1x before extend")
	um.ExtendSectorExpiration(scc[0], d5Scc.Expiration+builtin.EpochsInDay)
	um.ExtendSectorExpiration(snew[0], d5Snew.Expiration+builtin.EpochsInDay)

	d5post, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(d5pre.MinerPower.QualityAdjPower.String(), d5post.MinerPower.QualityAdjPower.String(),
		"extend must not change total QA power")

	exScc, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(exScc)
	req.Greater(exScc.Expiration, d5Scc.Expiration, "legacy sector must have been extended")
	req.Equal(d5Scc.Flags, exScc.Flags, "extend must preserve legacy sector flags (still no FULL_QA_POWER)")
	req.Zero(exScc.Flags&miner.FULL_QA_POWER, "extend must NOT promote a legacy 1x sector to 10x")
	req.Equal(d5Scc.DealWeight, exScc.DealWeight, "extend must preserve legacy DealWeight")
	req.Equal(d5Scc.VerifiedDealWeight, exScc.VerifiedDealWeight, "extend must preserve legacy VerifiedDealWeight")

	exSnew, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(exSnew)
	req.Greater(exSnew.Expiration, d5Snew.Expiration, "new sector must have been extended")
	req.NotZero(exSnew.Flags&miner.FULL_QA_POWER, "extend must preserve FULL_QA_POWER on a 10x sector")

	// USQ raises legacy 1x sector to 10x and is idempotent.
	d4pre, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{scc[0]}, nil)
	req.NoError(err, "USQ of legacy CC sector must succeed")

	uInfo, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(uInfo)
	req.NotZero(uInfo.Flags&miner.FULL_QA_POWER, "USQ must set FULL_QA_POWER on a legacy sector")
	req.True(uInfo.DealWeight.NilOrZero(), "USQ'd legacy CC sector must keep zero DealWeight")

	d4after, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	usqDelta := d4after.MinerPower.QualityAdjPower.Uint64() - d4pre.MinerPower.QualityAdjPower.Uint64()
	req.Equal(uint64(defaultSectorSize)*9, usqDelta,
		"USQ must raise the legacy CC sector's QA power from 1x to 10x (+9x raw)")

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{scc[0]}, nil)
	req.NoError(err, "repeated USQ of the same sector must succeed (no-op on QA)")
	d4idem, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(d4after.MinerPower.QualityAdjPower.String(), d4idem.MinerPower.QualityAdjPower.String(),
		"repeated USQ must be a no-op on QA power")

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{snew[0]}, nil)
	req.NoError(err, "USQ of an already-10x sector must succeed (no-op on QA)")
	d4new, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(d4idem.MinerPower.QualityAdjPower.String(), d4new.MinerPower.QualityAdjPower.String(),
		"USQ on an already-10x sector must not change QA power")

	nInfo, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(nInfo)
	newExp := nInfo.Expiration + abi.ChainEpoch(1000)
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{snew[0]}, &newExp)
	req.NoError(err, "USQ with new-expiration must succeed")
	nInfo2, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(nInfo2)
	req.Greater(nInfo2.Expiration, nInfo.Expiration, "USQ with new-expiration must extend the sector's expiration")
	d4exp, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(d4new.MinerPower.QualityAdjPower.String(), d4exp.MinerPower.QualityAdjPower.String(),
		"USQ with new-expiration must not change QA power (multiplier carries forward)")
}

// TestMigrationNV29SolsticeAccounting verifies QAP accounting for partial batch USQ and termination.
func TestMigrationNV29SolsticeAccounting(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		// A bit higher than the base test since a 3-sector batch needs slightly more time to
		// activate pre-upgrade.
		upgradeEpoch = abi.ChainEpoch(3000)
	)

	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	client, _, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)

	um, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(defaultSectorSize),
		kit.OwnerAddr(client.DefaultKey),
	)
	defer um.Stop()

	blockMiners := ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()
	blockMiners[0].WatchMinerForPost(um.ActorAddr)

	maddr := um.ActorAddr

	legs, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(3))
	req.Len(legs, 3)
	req.Less(legs[0], legs[1])
	req.Less(legs[1], legs[2])

	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.Less(info.Activation, upgradeEpoch, "legacy sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must start without FULL_QA_POWER", sn)
	}

	um.WaitTillActivatedAndAssertPower(legs, uint64(defaultSectorSize)*3, uint64(defaultSectorSize)*3)

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	beforeUSQ, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*3, beforeUSQ.MinerPower.QualityAdjPower.Uint64(),
		"legacy sectors must not be bumped to 10x by the migration")

	// batch-USQ legs[1] and legs[2], leaving legs[0] at 1x.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{legs[1], legs[2]}, nil)
	req.NoError(err, "batch USQ of legacy sectors must succeed")
	for _, sn := range []abi.SectorNumber{legs[1], legs[2]} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "USQ'd sector %d must carry FULL_QA_POWER", sn)
	}
	leftInfo, err := client.StateSectorGetInfo(ctx, maddr, legs[0], types.EmptyTSK)
	req.NoError(err)
	req.Zero(leftInfo.Flags&miner.FULL_QA_POWER, "skipped sector %d must stay at 1x", legs[0])

	afterUSQ, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*(1+10+10), afterUSQ.MinerPower.QualityAdjPower.Uint64(),
		"batch USQ of 2 sectors must yield 1x + 2x10x")

	perSectorMul := uint64(defaultSectorSize) * 9
	req.Equal(perSectorMul*2, afterUSQ.MinerPower.QualityAdjPower.Uint64()-beforeUSQ.MinerPower.QualityAdjPower.Uint64(),
		"miner QAP delta over batch USQ of 2 legacy sectors")
	req.Equal(perSectorMul*2, afterUSQ.TotalPower.QualityAdjPower.Uint64()-beforeUSQ.TotalPower.QualityAdjPower.Uint64(),
		"network QAP delta must equal the miner QAP delta over batch USQ")

	um.TerminateSectors([]abi.SectorNumber{legs[1]})
	expectedQA := uint64(defaultSectorSize) * (1 + 10) // remaining: legs[0]=1x + legs[2]=10x
	kit.WaitForMinerQAP(ctx, t, client, maddr, expectedQA, 2*time.Minute)

	um.TerminateSectors([]abi.SectorNumber{legs[0]})
	kit.WaitForMinerQAP(ctx, t, client, maddr, uint64(defaultSectorSize)*10, 2*time.Minute)
}

// TestMigrationNV29SolsticeEconomic tests USQ+new-expiration convergence, idempotency, auth, and fees.
func TestMigrationNV29SolsticeEconomic(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(4000)
	)

	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	client, _, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)

	um, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(defaultSectorSize),
		kit.OwnerAddr(client.DefaultKey),
	)
	defer um.Stop()

	blockMiners := ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()
	blockMiners[0].WatchMinerForPost(um.ActorAddr)

	maddr := um.ActorAddr

	legs, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(4))
	req.Len(legs, 4)
	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.Less(info.Activation, upgradeEpoch, "legacy sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must start at 1x", sn)
	}
	um.WaitTillActivatedAndAssertPower(legs, uint64(defaultSectorSize)*4, uint64(defaultSectorSize)*4)

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	s0, s1, s2, s3 := legs[0], legs[1], legs[2], legs[3]

	// USQ+new-expiration and extend-then-USQ must converge to the same end state.
	s0Info, err := client.StateSectorGetInfo(ctx, maddr, s0, types.EmptyTSK)
	req.NoError(err)
	ext := s0Info.Expiration + abi.ChainEpoch(2000)

	e3pre, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{s0}, &ext)
	req.NoError(err, "USQ+new-expiration (path A) must succeed")
	um.ExtendSectorExpiration(s1, ext)
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{s1}, nil)
	req.NoError(err, "extend-then-USQ (path B) must succeed")

	aInfo, err := client.StateSectorGetInfo(ctx, maddr, s0, types.EmptyTSK)
	req.NoError(err)
	bInfo, err := client.StateSectorGetInfo(ctx, maddr, s1, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(aInfo.Flags&miner.FULL_QA_POWER, "path A sector must be raised to 10x")
	req.NotZero(bInfo.Flags&miner.FULL_QA_POWER, "path B sector must be raised to 10x")
	req.Equal(ext, aInfo.Expiration, "path A must extend to the requested expiration")
	req.Equal(ext, bInfo.Expiration, "path B must end at the same expiration")
	req.True(aInfo.DealWeight.NilOrZero() && bInfo.DealWeight.NilOrZero(), "both CC sectors keep zero deal weight")
	e3post, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(2*uint64(defaultSectorSize)*9,
		e3post.MinerPower.QualityAdjPower.Uint64()-e3pre.MinerPower.QualityAdjPower.Uint64(),
		"USQ+new-exp and extend-then-USQ must each add +9x for an identical power end-state")

	// re-USQ of an already-10x sector with --new-expiration is a quality no-op but still extends.
	nInfo, err := client.StateSectorGetInfo(ctx, maddr, s0, types.EmptyTSK)
	req.NoError(err)
	ext2 := nInfo.Expiration + abi.ChainEpoch(1000)
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{s0}, &ext2)
	req.NoError(err, "re-USQ with new-expiration on an already-10x sector must succeed")

	nInfo2, err := client.StateSectorGetInfo(ctx, maddr, s0, types.EmptyTSK)
	req.NoError(err)
	req.Equal(ext2, nInfo2.Expiration, "re-USQ must advance the expiration")
	req.Equal(aInfo.InitialPledge.String(), nInfo2.InitialPledge.String(),
		"re-USQ must not re-derive the pledge (multiplier carries forward)")
	req.NotZero(nInfo2.Flags&miner.FULL_QA_POWER, "sector stays 10x (not 100x) after re-USQ with new-expiration")
	e2post, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(e3post.MinerPower.QualityAdjPower.String(), e2post.MinerPower.QualityAdjPower.String(),
		"re-USQ with new-expiration must not change QA power (multiplier carries forward)")

	// unrelated address calling method 37 is rejected; StateCall is virtual so state is untouched.
	e1Addr, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	fund, err := client.MpoolPushMessage(ctx, &types.Message{
		From:  client.DefaultKey.Address,
		To:    e1Addr,
		Value: types.FromFil(1), // must create the account actor for StateCall to resolve From
	}, nil)
	req.NoError(err)
	_, err = client.StateWaitMsg(ctx, fund.Cid(), 1, lapi.LookbackNoLimit, true)
	req.NoError(err, "funding the unrelated address")

	e1loc, err := client.StateSectorPartition(ctx, maddr, s0, types.EmptyTSK)
	req.NoError(err)
	e1enc, aerr := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
		Upgrades: []stminer.UpgradeSectorQuality{{
			Deadline:  e1loc.Deadline,
			Partition: e1loc.Partition,
			Sectors:   bitfield.NewFromSet([]uint64{uint64(s0)}),
		}},
	})
	req.NoError(aerr)
	e1res, err := client.StateCall(ctx, &types.Message{
		From:   e1Addr,
		To:     maddr,
		Method: builtin.MethodsMiner.UpgradeSectorQuality,
		Params: e1enc,
		Value:  types.FromFil(0),
	}, types.EmptyTSK)
	req.NoError(err)
	req.Equal(exitcode.ErrForbidden, e1res.MsgRct.ExitCode,
		"an unrelated address must be forbidden from calling UpgradeSectorQuality")

	// 10x sector's max termination fee must exceed sibling 1x sector's.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{s3}, nil)
	req.NoError(err, "USQ of s3 must succeed")

	oneX, err := client.StateSectorGetInfo(ctx, maddr, s2, types.EmptyTSK)
	req.NoError(err)
	tenX, err := client.StateSectorGetInfo(ctx, maddr, s3, types.EmptyTSK)
	req.NoError(err)
	req.Zero(oneX.Flags&miner.FULL_QA_POWER, "s2 control must stay at 1x")
	req.NotZero(tenX.Flags&miner.FULL_QA_POWER, "s3 must be raised to 10x")
	req.Greater(tenX.InitialPledge.Uint64(), oneX.InitialPledge.Uint64(),
		"upgrade to 10x must re-derive a higher on-chain initial pledge")

	feeFor := func(power abi.StoragePower, pledge abi.TokenAmount) abi.TokenAmount {
		p, aerr := actors.SerializeParams(&miner.MaxTerminationFeeParams{Power: power, InitialPledge: pledge})
		req.NoError(aerr)
		m, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     maddr,
			From:   client.DefaultKey.Address,
			Method: builtin.MethodsMiner.MaxTerminationFeeExported,
			Params: p,
			Value:  types.FromFil(0),
		}, nil)
		req.NoError(err)
		r, err := client.StateWaitMsg(ctx, m.Cid(), 1, lapi.LookbackNoLimit, true)
		req.NoError(err)
		req.EqualValues(0, r.Receipt.ExitCode, "MaxTerminationFeeExported must succeed")
		var fee miner.MaxTerminationFeeReturn
		req.NoError(fee.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)))
		return fee
	}

	fee1x := feeFor(types.NewInt(uint64(defaultSectorSize)), oneX.InitialPledge)
	fee10x := feeFor(types.NewInt(uint64(defaultSectorSize)*10), tenX.InitialPledge)
	req.Greater(fee10x.Uint64(), fee1x.Uint64(),
		"the max termination fee of an upgraded 10x sector must exceed that of a 1x sibling")
}

// TestMigrationNV29SolsticePledge asserts a native NV29 CC sector is pledged at the FULL_QA (10x) tier.
func TestMigrationNV29SolsticePledge(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(1000)
	)

	sealProofType, err := miner.SealProofTypeFromSectorSize(defaultSectorSize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	client, _, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)

	um, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(defaultSectorSize),
		kit.OwnerAddr(client.DefaultKey),
	)
	defer um.Stop()

	blockMiners := ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()
	blockMiners[0].WatchMinerForPost(um.ActorAddr)

	maddr := um.ActorAddr

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	snew, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(snew, 1)
	um.WaitTillActivatedAndAssertPower(snew, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)

	info, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(info)
	req.NotZero(info.Flags&miner.FULL_QA_POWER, "post-upgrade CC sector must be FULL_QA")
	onChain := info.InitialPledge.Uint64()
	req.Greater(onChain, uint64(0), "post-upgrade CC sector must carry a nonzero initial pledge")

	// verifiedSize=0 → legacy-1x estimate; verifiedSize=full → FULL_QA (10x) estimate.
	duration := info.Expiration - info.PowerBaseEpoch
	head, err = client.ChainHead(ctx)
	req.NoError(err)

	oneX, err := client.StateMinerInitialPledgeForSector(ctx, duration, defaultSectorSize, 0, head.Key())
	req.NoError(err)
	full, err := client.StateMinerInitialPledgeForSector(ctx, duration, defaultSectorSize, uint64(defaultSectorSize), head.Key())
	req.NoError(err)

	oneXv, fullv := oneX.Uint64(), full.Uint64()
	req.Greater(fullv, oneXv, "FULL-QA pledge estimate must exceed the legacy-1x estimate for the same CC sector")
	// Loose bound: on-chain pledge must be above 1x and at least half of the 10x oracle estimate.
	// The oracle overestimates by ~10%; live reward state can drift from head-tipset estimate.
	req.Greater(onChain, oneXv, "chain must charge more than the legacy-1x pledge for a post-upgrade CC sector")
	req.GreaterOrEqual(onChain, fullv/2,
		"chain must charge the FULL-QA (10x) pledge tier for a post-upgrade CC sector (on-chain %d, 1x-est %d, 10x-est %d)",
		onChain, oneXv, fullv)
}
