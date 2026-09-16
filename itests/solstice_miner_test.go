package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-keccak"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	reward18 "github.com/filecoin-project/go-state-types/builtin/v18/reward"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	adt19 "github.com/filecoin-project/go-state-types/builtin/v19/util/adt"
	rewardMath "github.com/filecoin-project/go-state-types/builtin/v19/util/math"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	chainstate "github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
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

// TestMigrationNV29SolsticeAccounting verifies QAP and gas accounting across multiple USQ messages,
// keeping a legacy 1x control until both upgraded and untouched sectors are terminated.
func TestMigrationNV29SolsticeAccounting(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		nSectors          = 7
		sectorsPerMessage = 2
		upgradeEpoch      = abi.ChainEpoch(3000)
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

	legs, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(nSectors))
	req.Len(legs, nSectors)

	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.Less(info.Activation, upgradeEpoch, "legacy sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must start without FULL_QA_POWER", sn)
	}

	um.WaitTillActivatedAndAssertPower(legs, uint64(defaultSectorSize)*nSectors, uint64(defaultSectorSize)*nSectors)

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	beforeUSQ, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*nSectors, beforeUSQ.MinerPower.QualityAdjPower.Uint64(),
		"legacy sectors must not be bumped to 10x by the migration")

	// Submit three real two-sector messages, leaving legs[0] at 1x throughout.
	// The CLI's packing and splitting decisions are covered by its unit tests.
	var gasUsed []int64
	var totalGas int64
	var upgraded uint64
	perSectorDelta := uint64(defaultSectorSize) * 9
	for start := 1; start < len(legs); start += sectorsPerMessage {
		batch := legs[start : start+sectorsPerMessage]
		lookup, err := um.UpgradeSectorQuality(batch, nil)
		req.NoError(err, "USQ message covering sectors %v must succeed", batch)
		req.Equal(exitcode.Ok, lookup.Receipt.ExitCode)
		req.Positive(lookup.Receipt.GasUsed, "each USQ message must burn gas")
		req.Less(lookup.Receipt.GasUsed, buildconstants.BlockGasLimit,
			"each USQ message must fit below the block gas limit")
		gasUsed = append(gasUsed, lookup.Receipt.GasUsed)
		totalGas += lookup.Receipt.GasUsed

		afterBatch, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
		req.NoError(err)
		upgraded += uint64(len(batch))
		req.Equal(perSectorDelta*upgraded,
			afterBatch.MinerPower.QualityAdjPower.Uint64()-beforeUSQ.MinerPower.QualityAdjPower.Uint64(),
			"miner QAP must increase by exactly 9x for each sector upgraded across the messages")
		req.Equal(perSectorDelta*upgraded,
			afterBatch.TotalPower.QualityAdjPower.Uint64()-beforeUSQ.TotalPower.QualityAdjPower.Uint64(),
			"network QAP must match the miner QAP increase across the messages")
	}
	req.Len(gasUsed, 3, "all three USQ messages must execute")
	req.Less(totalGas, buildconstants.BlockGasLimit, "the combined batch gas must stay below the block gas limit")
	t.Logf("USQ messages: per-message gas %v, total %d (block gas limit %d)",
		gasUsed, totalGas, buildconstants.BlockGasLimit)

	for _, sn := range legs[1:] {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "USQ'd sector %d must carry FULL_QA_POWER", sn)
	}
	leftInfo, err := client.StateSectorGetInfo(ctx, maddr, legs[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(leftInfo)
	req.Zero(leftInfo.Flags&miner.FULL_QA_POWER, "skipped sector %d must stay at 1x", legs[0])

	afterUSQ, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*(1+10*(nSectors-1)), afterUSQ.MinerPower.QualityAdjPower.Uint64(),
		"partial USQ must leave one legacy 1x sector and six upgraded 10x sectors")

	um.TerminateSectors([]abi.SectorNumber{legs[1]})
	expectedQA := uint64(defaultSectorSize) * (1 + 10*(nSectors-2))
	kit.WaitForMinerQAP(ctx, t, client, maddr, expectedQA, 2*time.Minute)

	um.TerminateSectors([]abi.SectorNumber{legs[0]})
	kit.WaitForMinerQAP(ctx, t, client, maddr, uint64(defaultSectorSize)*10*(nSectors-2), 2*time.Minute)
	um.AssertNoWindowPostError()
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

// TestMigrationNV29SolsticeUpgradeQualityAuth asserts USQ (method 37) caller authorization: owner/worker and control accepted, unrelated rejected with USR_FORBIDDEN.
func TestMigrationNV29SolsticeUpgradeQualityAuth(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// fundAccount creates an account actor so `a` can be used as a StateCall From.
	fundAccount := func(a address.Address) {
		kit.SendFunds(ctx, t, client, a, types.FromFil(1))
	}

	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))
	sn := legacy[0]

	lInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER before USQ")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	ctrlAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelatedAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	fundAccount(ctrlAddr)
	fundAccount(unrelatedAddr)

	mi, err := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	cwp := &stminer.ChangeWorkerAddressParams{
		NewWorker:       mi.Worker, // unchanged -> no worker handover delay
		NewControlAddrs: []address.Address{ctrlAddr},
	}
	cwEnc, aerr := actors.SerializeParams(cwp)
	req.NoError(aerr)
	cwMsg, err := client.MpoolPushMessage(ctx, &types.Message{
		From:   client.DefaultKey.Address, // owner is the only authorised caller of ChangeWorkerAddress
		To:     maddr,
		Method: builtin.MethodsMiner.ChangeWorkerAddress,
		Params: cwEnc,
		Value:  types.FromFil(0),
	}, nil)
	req.NoError(err)
	_, err = client.StateWaitMsg(ctx, cwMsg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err, "ChangeWorkerAddress must be confirmed")

	ctrlID, err := client.StateLookupID(ctx, ctrlAddr, types.EmptyTSK)
	req.NoError(err)
	defaultID, err := client.StateLookupID(ctx, client.DefaultKey.Address, types.EmptyTSK)
	req.NoError(err)
	mi, err = client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Contains(mi.ControlAddresses, ctrlID, "ctrlAddr must now be a control address")
	req.Equal(defaultID, mi.Owner, "owner unchanged")
	req.Equal(defaultID, mi.Worker, "worker unchanged (owner==worker in the kit ensemble)")

	// usqCallExit probes USQ (method 37) via StateCall; returns the exit code.
	usqCallExit := func(from address.Address) exitcode.ExitCode {
		loc, lerr := client.StateSectorPartition(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(lerr)
		enc, sErr := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
			Upgrades: []stminer.UpgradeSectorQuality{{
				Deadline:  loc.Deadline,
				Partition: loc.Partition,
				Sectors:   bitfield.NewFromSet([]uint64{uint64(sn)}),
			}},
		})
		req.NoError(sErr)
		res, cErr := client.StateCall(ctx, &types.Message{
			From:   from,
			To:     maddr,
			Method: builtin.MethodsMiner.UpgradeSectorQuality,
			Params: enc,
			Value:  types.FromFil(0),
		}, types.EmptyTSK)
		req.NoError(cErr)
		return res.MsgRct.ExitCode
	}

	req.Equal(exitcode.Ok, usqCallExit(ctrlAddr),
		"a control address must be authorized to call UpgradeSectorQuality (Ok, not USR_FORBIDDEN)")

	req.Equal(exitcode.ErrForbidden, usqCallExit(unrelatedAddr),
		"an unrelated address must be forbidden from calling UpgradeSectorQuality")

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.NoError(err, "owner/worker USQ must be accepted")

	info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(info.Flags&miner.FULL_QA_POWER,
		"the owner/worker USQ must actually raise the legacy sector to FULL_QA(10x)")
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*10, power.MinerPower.QualityAdjPower.Uint64(),
		"owner/worker USQ lifts the miner's only sector to 10x QAP")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeUpgradeQualityPureOwnerAuth asserts that a pure owner (owner != worker, not control) is authorized for USQ (method 37) while unrelated is USR_FORBIDDEN.
func TestMigrationNV29SolsticeUpgradeQualityPureOwnerAuth(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof

	ownerA := client.DefaultKey.Address

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	onboarded, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(onboarded, 1)
	um.WaitTillActivatedAndAssertPower(onboarded, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)
	sn := onboarded[0]
	um.Stop()

	// workerB must be BLS (actor rejects secp worker); control and unrelated may be secp.
	workerB, err := client.WalletNew(ctx, types.KTBLS)
	req.NoError(err)
	controlC, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelatedD, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	for _, a := range []address.Address{workerB, controlC, unrelatedD} {
		kit.SendFunds(ctx, t, client, a, types.FromFil(1))
	}

	resolveToID := func(a address.Address) address.Address {
		id, lerr := client.StateLookupID(ctx, a, types.EmptyTSK)
		req.NoError(lerr)
		return id
	}
	workerBID := resolveToID(workerB)

	cwp := &stminer.ChangeWorkerAddressParams{
		NewWorker:       workerB,
		NewControlAddrs: []address.Address{controlC},
	}
	cwEnc, aerr := actors.SerializeParams(cwp)
	req.NoError(aerr)
	cwMsg, err := client.MpoolPushMessage(ctx, &types.Message{
		From:   ownerA,
		To:     maddr,
		Method: builtin.MethodsMiner.ChangeWorkerAddress,
		Params: cwEnc,
		Value:  types.FromFil(0),
	}, nil)
	req.NoError(err)
	_, err = client.StateWaitMsg(ctx, cwMsg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err, "ChangeWorkerAddress must be confirmed")

	saAct, saErr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
	req.NoError(saErr)
	bs := gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client))
	var mst stminer.State
	req.NoError(bs.Get(ctx, saAct.Head, &mst))
	var mInfo stminer.MinerInfo
	req.NoError(bs.Get(ctx, mst.Info, &mInfo))
	req.NotNil(mInfo.PendingWorkerKey, "a real worker change must register a pending worker key")
	effectiveAt := mInfo.PendingWorkerKey.EffectiveAt
	t.Logf("worker handover to %s pending, effective at epoch %d", workerBID, effectiveAt)

	client.WaitTillChain(ctx, kit.HeightAtLeast(effectiveAt+20))

	cfmMsg, cerr := client.MpoolPushMessage(ctx, &types.Message{
		From:   ownerA,
		To:     maddr,
		Method: builtin.MethodsMiner.ConfirmChangeWorkerAddress,
		Params: nil, // *abi.EmptyValue
		Value:  types.FromFil(0),
	}, nil)
	req.NoError(cerr)
	_, cerr = client.StateWaitMsg(ctx, cfmMsg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(cerr, "ConfirmChangeWorkerAddress must be confirmed")

	// stateCallExit probes method m via StateCall; returns the exit code.
	stateCallExit := func(from address.Address, m abi.MethodNum, params []byte) exitcode.ExitCode {
		res, cerr := client.StateCall(ctx, &types.Message{
			From:   from,
			To:     maddr,
			Method: m,
			Params: params,
			Value:  types.FromFil(0),
		}, types.EmptyTSK)
		req.NoError(cerr)
		return res.MsgRct.ExitCode
	}

	mi, merr := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	req.NoError(merr)
	ownerAID := resolveToID(ownerA)
	controlCID := resolveToID(controlC)
	aIsControl := false
	for _, c := range mi.ControlAddresses {
		if c == ownerAID {
			aIsControl = true
			break
		}
	}
	req.True(mi.Owner == ownerAID, "owner must be A after the handover; got %s want %s", mi.Owner, ownerAID)
	req.True(mi.Worker == workerBID, "worker must be B after the handover (A is no longer worker); got %s want %s", mi.Worker, workerBID)
	req.Contains(mi.ControlAddresses, controlCID, "control C must be installed after the handover")
	req.False(aIsControl, "A must not be a control address (pure owner); controls=%v", mi.ControlAddresses)
	req.True(mi.Owner == ownerAID && mi.Worker != ownerAID && !aIsControl,
		"guard: A must be owner-only (Owner=A, Worker=B, A not in controls)")
	t.Logf("guard ok: after handover Owner=%s Worker=%s Controls=%v -> A is a pure owner", mi.Owner, mi.Worker, mi.ControlAddresses)

	loc, lerr := client.StateSectorPartition(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(lerr)
	usqEnc, sErr := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
		Upgrades: []stminer.UpgradeSectorQuality{{
			Deadline:  loc.Deadline,
			Partition: loc.Partition,
			Sectors:   bitfield.NewFromSet([]uint64{uint64(sn)}),
		}},
	})
	req.NoError(sErr)

	ownerUSQ := stateCallExit(ownerA, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	workerUSQ := stateCallExit(workerB, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	controlUSQ := stateCallExit(controlC, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	unrelatedUSQ := stateCallExit(unrelatedD, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	t.Logf("UpgradeSectorQuality exit codes: owner(A)=%d worker(B)=%d control(C)=%d unrelated(D)=%d",
		ownerUSQ, workerUSQ, controlUSQ, unrelatedUSQ)

	req.Equal(exitcode.ErrForbidden, unrelatedUSQ, "an unrelated address must be forbidden from method 37")
	req.NotEqual(exitcode.ErrForbidden, workerUSQ, "the distinct worker B must remain authorized for method 37")
	req.NotEqual(exitcode.ErrForbidden, controlUSQ, "the control address must remain authorized for method 37")

	// A pure owner cannot surface USR_FORBIDDEN; sector may fault after A lost worker role.
	faults, ferr := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	req.NoError(ferr)
	ownerSectorFaulted, fserr := faults.IsSet(uint64(sn))
	req.NoError(fserr)
	if ownerSectorFaulted {
		req.NotEqual(exitcode.ErrForbidden, ownerUSQ,
			"a pure owner (owner != worker, not a control) must be authorized for method 37; got %d (sector faulted: authorized owner surfaces only a sector-state error, not USR_FORBIDDEN)", ownerUSQ)
		t.Logf("sector %d is faulted at probe time; pure-owner authorization asserted at the caller gate (ownerUSQ=%d)", sn, ownerUSQ)
	} else {
		req.Equal(exitcode.Ok, ownerUSQ,
			"a pure owner (owner != worker, not a control) must be authorized for method 37 AND run it to completion on an active sector; got %d", ownerUSQ)
		t.Logf("sector %d active at probe time; pure owner ran method 37 to OK on an active sector", sn)
	}
}

// TestSolsticeRewardLifecycle drives f02's FIP-0118 reward streams through the governance
// contracts which run as UUPS proxies.
// Every SWA-gated f02 write arrives through the SWA proxy and stream 2's share map
// through the SRA proxy, covering:
//
//   - the v18 to v19 migration, award continuity, and circulating supply;
//   - the bootstrap weight ramp and the reward split it settles on;
//   - the SWA's writes: the deferred-write queue, cancellation, registration, removal;
//   - the SRA's quarterly gate stepping stream 2's weight and submitting its share map;
//   - share settlement, wallet payouts, and tombstone claims on a stream the SWA registers.
func TestSolsticeRewardLifecycle(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	const (
		blockTime    = 100 * time.Millisecond
		upgradeEpoch = abi.ChainEpoch(180)
		activation   = upgradeEpoch + 1
		// The pre-migration starts this many epochs before the upgrade; setup must finish before it.
		preMigrationStart           = abi.ChainEpoch(15)
		timelock                    = abi.ChainEpoch(20)
		consensusWeightRampDuration = abi.ChainEpoch(81)
		splitSampleEpochs           = abi.ChainEpoch(10)

		sraActivationEpoch = uint64(activation)
		epochsPerQuarter   = uint64(90)
		postPeriod         = uint64(60)
		verificationWindow = uint64(10)
		hold               = uint64(0) // no upgrade delay, fine for test deployment

		// The SWA initializes at quarter 1, so its first quarterlyGateCheck reads quarter 2.
		gateQuarter = uint64(2)

		// w3 has its own writer, for share and claim tests outside the SRA's rules.
		w3 = reward19.StreamID(3)
	)
	pct := reward19.Denom / 100

	quarterStart := abi.ChainEpoch(sraActivationEpoch + gateQuarter*epochsPerQuarter)
	postingEnd := quarterStart + abi.ChainEpoch(postPeriod)
	bindingEpoch := postingEnd + abi.ChainEpoch(verificationWindow)
	// The gate's first volume threshold is VOL_TARGET_ENTRY, 3500 USD in 1e18 fixed point.
	gateVolumeUSD := big.Mul(big.NewInt(4000), big.NewInt(1_000_000_000_000_000_000))
	// A passing step multiplies the threshold by VOL_TARGET_RATIO, putting it at 9450 USD.
	belowThresholdUSD := big.Mul(big.NewInt(5000), big.NewInt(1_000_000_000_000_000_000))
	// quarterlyGateCheck's first passing step sets stream 2 flat at (steps + 3) * 5%.
	steppedWeight := reward19.WeightRecord{
		VStart: 15 * pct, TStart: quarterStart, Floor: 15 * pct, Cap: 15 * pct,
	}

	owner1Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	owner2Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	orchestratorKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	w3WriterKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	recipient1Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	recipient2Key, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)
	deployerKey, err := key.GenerateKey(types.KTSecp256k1)
	req.NoError(err)

	bootstrapParams := buildconstants.SolsticeRewardBootstrapParams{
		SWATimelockEpochs:                 timelock,
		ConsensusWeightRampDurationEpochs: consensusWeightRampDuration,
		ConsensusWeight: buildconstants.SolsticeRewardWeightParams{
			VStart: 95 * pct,
			Floor:  50 * pct,
			Cap:    95 * pct,
		},
		ServiceWeight: buildconstants.SolsticeRewardWeightParams{
			VStart: 5 * pct,
			Floor:  5 * pct,
			Cap:    10 * pct,
		},
	}
	rampTotal := bootstrapParams.ConsensusWeight.VStart - bootstrapParams.ConsensusWeight.Floor
	// The service stream's bootstrap rise is one ninth of the consensus ramp (FIP-0118).
	req.Equal(rampTotal/9, bootstrapParams.ServiceWeight.Cap-bootstrapParams.ServiceWeight.VStart)
	rampEpochs := uint64(bootstrapParams.ConsensusWeightRampDurationEpochs)
	req.NotZero(rampTotal % rampEpochs)
	bootstrapSlope := rampTotal / rampEpochs
	if rampTotal%rampEpochs != 0 {
		bootstrapSlope++
	}
	req.Positive(bootstrapSlope)
	consensusFloorEpoch := activation + bootstrapParams.ConsensusWeightRampDurationEpochs
	splitStartEpoch := consensusFloorEpoch + 1
	splitEndEpoch := splitStartEpoch + splitSampleEpochs

	// deployerKey's first four messages are the deploys below, at nonces 0 to 3, which fixes their
	// CREATE addresses; the migration resolves the f410 forms from its input state tree.
	deployerEth := kit.EthAddressForCreate(t, deployerKey.Address)
	createAddress := func(nonce uint64) address.Address {
		addr, err := kit.ComputeContractAddress(t, deployerEth, nonce).ToFilecoinAddress()
		req.NoError(err)
		return addr
	}
	sraImplF4, sraF4, swaImplF4, swaF4 := createAddress(0), createAddress(1), createAddress(2), createAddress(3)

	bootstrapParams.SWAActor = swaF4
	bootstrapParams.SRAActor = sraF4
	// The migration resolves the orchestrator's f1 address the same way.
	bootstrapParams.InitialOrchestrator = orchestratorKey.Address

	// The pre-migration runs on its own goroutine and only logs its failures, so record its result.
	var preMigrationRuns atomic.Int64
	var preMigrationErr atomic.Pointer[error]
	preMigration := filcns.PreUpgradeActorsV19With(bootstrapParams)

	// Capture the real migration boundary, including any cron work in null epochs.
	// StateCompute intentionally skips that cron work, so replaying from an earlier
	// tipset would not provide the same input when the upgrade epoch is null.
	type migrationStateRoots struct{ before, after cid.Cid }
	var migrationRootsLk sync.Mutex
	migrationRoots := make(map[types.TipSetKey]migrationStateRoots)
	migrate := filcns.UpgradeActorsV19With(bootstrapParams)
	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.ThroughRPC(),
		kit.Account(owner1Key, types.FromFil(100)),
		kit.Account(owner2Key, types.FromFil(100)),
		kit.Account(orchestratorKey, types.FromFil(100)),
		kit.Account(w3WriterKey, types.FromFil(100)),
		kit.Account(recipient1Key, types.FromFil(100)),
		kit.Account(recipient2Key, types.FromFil(100)),
		kit.Account(deployerKey, types.FromFil(100)),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network: network.Version29,
				Height:  upgradeEpoch,
				Migration: func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache, cb stmgr.ExecMonitor, oldState cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
					newState, err := migrate(ctx, sm, cache, cb, oldState, height, ts)
					if err == nil {
						// Sibling tipsets share their parent's migration input. Key by that
						// parent so a repeated migration or reorg cannot select another fork.
						migrationRootsLk.Lock()
						migrationRoots[ts.Parents()] = migrationStateRoots{before: oldState, after: newState}
						migrationRootsLk.Unlock()
					}
					return newState, err
				},
				PreMigrations: []stmgr.PreMigration{{
					PreMigration: func(ctx context.Context, sm *stmgr.StateManager, cache stmgr.MigrationCache,
						root cid.Cid, epoch abi.ChainEpoch, ts *types.TipSet) error {
						err := preMigration(ctx, sm, cache, root, epoch, ts)
						preMigrationErr.Store(&err)
						preMigrationRuns.Add(1)
						return err
					},
					StartWithin:     preMigrationStart,
					DontStartWithin: 2,
					StopWithin:      2,
				}},
			},
		),
	)
	blockMiners := ens.InterconnectAll().BeginMining(blockTime)

	for _, account := range []*key.Key{
		owner1Key, owner2Key, orchestratorKey, w3WriterKey, recipient1Key, recipient2Key, deployerKey,
	} {
		_, err := client.WalletImport(ctx, &account.KeyInfo)
		req.NoError(err)
	}

	// deployer owns neither contract, so it doubles as the non-owner in the governance checks.
	deployer := deployerKey.Address
	owner1 := kit.EvmWordFromAddr(ctx, t, client, owner1Key.Address)
	owner2 := kit.EvmWordFromAddr(ctx, t, client, owner2Key.Address)
	orchestrator := kit.EvmWordFromAddr(ctx, t, client, orchestratorKey.Address)
	orchestratorID, err := client.StateLookupID(ctx, orchestratorKey.Address, types.EmptyTSK)
	req.NoError(err)
	w3WriterID, err := client.StateLookupID(ctx, w3WriterKey.Address, types.EmptyTSK)
	req.NoError(err)
	recipient1ID, err := client.StateLookupID(ctx, recipient1Key.Address, types.EmptyTSK)
	req.NoError(err)
	recipient2ID, err := client.StateLookupID(ctx, recipient2Key.Address, types.EmptyTSK)
	req.NoError(err)

	// Each deploy must land on the address the bootstrap identifies.
	requireDeployedAt := func(what string, predicted address.Address, actorID uint64) address.Address {
		actual := solsticeIDAddress(t, actorID)
		resolved, err := client.StateLookupID(ctx, predicted, types.EmptyTSK)
		req.NoErrorf(err, "%s must deploy at its predicted address %s", what, predicted)
		req.Equalf(actual, resolved, "%s deployed at %s, not its predicted address %s", what, actual, predicted)
		return actual
	}

	sraImpl := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "ServiceRewardsActor",
		solsticeAddress(owner1),
		solsticeAddress(owner2),
		solsticeUint64(epochsPerQuarter),
		solsticeUint64(postPeriod),
		solsticeUint64(verificationWindow),
		solsticeUint64(sraActivationEpoch),
		solsticeUint64(hold),
	))
	sraImplAddr := requireDeployedAt("SRA implementation", sraImplF4, sraImpl.ActorID)

	sraProxy := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "ERC1967Proxy",
		solsticeAddress(kit.EvmWordBytes(sraImpl.EthAddress[:])),
		solsticeBytes(solsticeSelector(t, "ServiceRewardsActor", "initialize()")),
	))
	sraAddr := requireDeployedAt("SRA proxy", sraF4, sraProxy.ActorID)

	// The SWA reads volumes and quarter anchors from the SRA proxy.
	swaImpl := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "StreamWeightActor",
		solsticeAddress(owner1),
		solsticeAddress(owner2),
		solsticeUint64(hold),
		solsticeAddress(kit.EvmWordBytes(sraProxy.EthAddress[:])),
	))
	swaImplAddr := requireDeployedAt("SWA implementation", swaImplF4, swaImpl.ActorID)

	swaProxy := client.EVM().DeployContract(ctx, deployer, solsticeCreationCode(t, "ERC1967Proxy",
		solsticeAddress(kit.EvmWordBytes(swaImpl.EthAddress[:])),
		solsticeBytes(solsticeSelector(t, "StreamWeightActor", "initialize()")),
	))
	swaAddr := requireDeployedAt("SWA proxy", swaF4, swaProxy.ActorID)

	t.Logf("SRA implementation %s proxy %s; SWA implementation %s proxy %s",
		sraImplAddr, sraAddr, swaImplAddr, swaAddr)

	addOrchestrator := solsticeSelector(t, "ServiceRewardsActor", "addOrchestrator(address,address)")
	isAdmitted := solsticeSelector(t, "ServiceRewardsActor", "isAdmitted(address)")
	admission := [][]byte{orchestrator, orchestrator}

	implCall := solsticeInvoke(ctx, t, client, owner1Key.Address, sraImplAddr, addOrchestrator, admission...)
	req.False(implCall.Receipt.ExitCode.IsSuccess(),
		"the SRA implementation disabled its initializers, so it has no owners and an owner-gated call must revert")

	nonGovCall := solsticeInvoke(ctx, t, client, deployer, sraAddr, addOrchestrator, admission...)
	req.False(nonGovCall.Receipt.ExitCode.IsSuccess(), "a non-owner must not reach SRA governance")

	firstApproval := solsticeInvoke(ctx, t, client, owner1Key.Address, sraAddr, addOrchestrator, admission...)
	kit.RequireMessageSuccess(t, firstApproval)
	req.False(solsticeBool(t, solsticeInvoke(ctx, t, client, deployer, sraAddr, isAdmitted, orchestrator)),
		"one owner's approval must not admit an orchestrator on its own")

	secondApproval := solsticeInvoke(ctx, t, client, owner2Key.Address, sraAddr, addOrchestrator, admission...)
	kit.RequireMessageSuccess(t, secondApproval)
	req.True(solsticeBool(t, solsticeInvoke(ctx, t, client, deployer, sraAddr, isAdmitted, orchestrator)),
		"unanimous approval must admit the orchestrator")

	setupHead, err := client.ChainHead(ctx)
	req.NoError(err)
	t.Logf("deployment and governance complete at epoch %d", setupHead.Height())
	req.Lessf(setupHead.Height(), upgradeEpoch-preMigrationStart,
		"setup must finish before epoch %d, when the pre-migration may start",
		upgradeEpoch-preMigrationStart)

	store := cbor.NewCborStore(blockstore.NewAPIBlockstore(client))
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+8))

	preTS := tipsetAtOrBefore(ctx, t, client, upgradeEpoch)
	preActor, err := client.StateGetActor(ctx, builtin.RewardActorAddr, preTS.Key())
	req.NoError(err)
	var pre reward18.State
	req.NoError(store.Get(ctx, preActor.Head, &pre))
	// v18 issuance totals, compared with the v19 constants later.
	preSimpleTotal := pre.SimpleTotal
	preBaselineTotal := pre.BaselineTotal
	initialAllocation := big.Add(preActor.Balance, pre.TotalStoragePowerReward)

	activationTS := kit.TipsetAtOrAfter(ctx, t, client, activation)
	migrationRootsLk.Lock()
	roots, captured := migrationRoots[activationTS.Parents()]
	migrationRootsLk.Unlock()
	req.True(captured, "migration state roots must match the canonical activation tipset's parent")
	migrationInputTree, err := chainstate.LoadStateTree(store, roots.before)
	req.NoError(err)
	migrationInputActor, err := migrationInputTree.GetActor(builtin.RewardActorAddr)
	req.NoError(err)
	var migrationInput reward18.State
	req.NoError(store.Get(ctx, migrationInputActor.Head, &migrationInput))

	req.Positive(preMigrationRuns.Load(), "the nv29 pre-migration must have run")
	req.NoError(*preMigrationErr.Load(), "the nv29 pre-migration must warm the cache, not fail")

	migrationTree, err := chainstate.LoadStateTree(store, roots.after)
	req.NoError(err)
	migrationActor, err := migrationTree.GetActor(builtin.RewardActorAddr)
	req.NoError(err)
	var migrated reward19.State
	req.NoError(store.Get(ctx, migrationActor.Head, &migrated))
	adtStore := adt19.WrapStore(ctx, store)
	migratedStreams, err := migrated.LoadStreams(adtStore)
	req.NoError(err)
	_, invariantMessages := reward19.CheckStateInvariants(
		&migrated, adtStore, upgradeEpoch, migrationActor.Balance,
	)
	req.Empty(invariantMessages.Messages())
	req.Equal(initialAllocation, rewardAllocationAt(t, migrationActor, &migrated, migratedStreams))

	lifecycle := &solsticeRewardLifecycle{
		ctx:                 ctx,
		client:              client,
		miner:               miner,
		blockMiners:         blockMiners,
		store:               store,
		pct:                 pct,
		upgradeEpoch:        upgradeEpoch,
		activation:          activation,
		timelock:            timelock,
		bootstrapSlope:      bootstrapSlope,
		splitStart:          splitStartEpoch,
		splitEnd:            splitEndEpoch,
		bootstrap:           bootstrapParams,
		deployer:            deployer,
		owner1:              owner1Key.Address,
		owner2:              owner2Key.Address,
		sraAddr:             sraAddr,
		swaAddr:             swaAddr,
		orchestrator:        orchestratorKey.Address,
		orchestratorID:      orchestratorID,
		w3WriterKey:         w3WriterKey,
		w3WriterID:          w3WriterID,
		recipients:          []address.Address{recipient1ID, recipient2ID},
		newStream:           w3,
		gateQuarter:         gateQuarter,
		gateVolumeUSD:       gateVolumeUSD,
		belowThresholdUSD:   belowThresholdUSD,
		epochsPerQuarter:    abi.ChainEpoch(epochsPerQuarter),
		quarterStart:        quarterStart,
		postingEnd:          postingEnd,
		bindingEpoch:        bindingEpoch,
		steppedWeight:       steppedWeight,
		preTS:               preTS,
		activationTS:        activationTS,
		preSimpleTotal:      preSimpleTotal,
		preBaselineTotal:    preBaselineTotal,
		initialAllocation:   initialAllocation,
		migrationInputActor: migrationInputActor,
		migrationActor:      migrationActor,
		migrationInput:      migrationInput,
		migrated:            migrated,
		migratedStreams:     migratedStreams,
	}

	t.Run("migration and award continuity", lifecycle.testMigrationAndAwardContinuity)
	t.Run("circulating supply continuity", lifecycle.testCirculatingSupplyContinuity)
	t.Run("reward total constants replace stored state", lifecycle.testRewardTotalConstants)
	t.Run("sloped bootstrap weight", lifecycle.testSlopedBootstrapWeight)
	t.Run("reward split economics", lifecycle.testRewardSplitEconomics)
	t.Run("queue controls and write event", lifecycle.testQueueControlsAndEvent)
	t.Run("deferred weight schedule", lifecycle.testDeferredWeightSchedule)
	t.Run("quarterly gate step and share submission", lifecycle.testQuarterlyGateAndShares)
	t.Run("failed gate holds the stepped weight", lifecycle.testFailedGate)
	// The w3 phases fill the wait for the missed-post quarter to bind and leave stream 2 as the only
	// explicit stream by the time it runs.
	t.Run("stream registration", lifecycle.testStreamRegistration)
	t.Run("share settlement and wallet payouts", lifecycle.testShareSettlementAndWalletPayouts)
	t.Run("remove stream tombstone claim", lifecycle.testRemoveStreamTombstoneClaim)
	t.Run("missed volume post submits no shares", lifecycle.testMissedVolumePost)
}

func tipsetAtOrBefore(ctx context.Context, t *testing.T, node lapi.FullNode, target abi.ChainEpoch) *types.TipSet {
	t.Helper()
	req := require.New(t)
	ts, err := node.ChainGetTipSetByHeight(ctx, target, types.EmptyTSK)
	req.NoError(err)
	req.LessOrEqual(ts.Height(), target)
	return ts
}

// adjacentReward19States finds a single-epoch transition so reward recomputation doesn't straddle nulls.
func adjacentReward19States(
	ctx context.Context,
	t *testing.T,
	node lapi.FullNode,
	store cbor.IpldStore,
	start abi.ChainEpoch,
) (*reward19.State, *reward19.State) {
	t.Helper()
	req := require.New(t)
	head, err := node.ChainHead(ctx)
	req.NoError(err)
	req.GreaterOrEqual(head.Height(), start, "chain head has not reached search start")
	for height := start; height <= head.Height(); height++ {
		ts, err := node.ChainGetTipSetByHeight(ctx, height, head.Key())
		req.NoError(err)
		parent, err := node.ChainGetTipSet(ctx, ts.Parents())
		req.NoError(err)
		if ts.Height() != parent.Height()+1 {
			continue
		}
		_, previous, _ := kit.LoadReward19(ctx, t, node, store, parent.Key())
		_, current, _ := kit.LoadReward19(ctx, t, node, store, ts.Key())
		if current.Epoch == previous.Epoch+1 {
			return previous, current
		}
	}
	req.FailNow("no adjacent v19 reward states", "epoch %d through head %d", start, head.Height())
	return nil, nil
}

// computeRewardWithTotals evaluates the v19 formula with caller-supplied issuance totals.
func computeRewardWithTotals(
	epoch abi.ChainEpoch,
	prevTheta big.Int,
	currTheta big.Int,
	simpleTotal abi.TokenAmount,
	baselineTotal abi.TokenAmount,
) abi.TokenAmount {
	simpleReward := big.Mul(simpleTotal, reward19.ExpLamSubOne)
	epochLam := big.Mul(big.NewInt(int64(epoch)), reward19.Lambda)
	simpleReward = big.Mul(simpleReward, big.NewFromGo(rewardMath.ExpNeg(epochLam.Int)))
	simpleReward = big.Rsh(simpleReward, rewardMath.Precision128)

	baselineReward := big.Sub(
		computeBaselineSupplyWithTotal(currTheta, baselineTotal),
		computeBaselineSupplyWithTotal(prevTheta, baselineTotal),
	)
	return big.Rsh(big.Add(simpleReward, baselineReward), rewardMath.Precision128)
}

// computeBaselineSupplyWithTotal is ComputeBaselineSupply with an injectable allocation total.
func computeBaselineSupplyWithTotal(theta big.Int, baselineTotal abi.TokenAmount) abi.TokenAmount {
	thetaLam := big.Mul(theta, reward19.Lambda)
	thetaLam = big.Rsh(thetaLam, rewardMath.Precision128)
	expNegThetaLam := big.NewFromGo(rewardMath.ExpNeg(thetaLam.Int))
	one := big.Lsh(big.NewInt(1), rewardMath.Precision128)
	return big.Mul(baselineTotal, big.Sub(one, expNegThetaLam))
}

// explicitServiceLiabilities sums stream accruals (net of claimed) plus tombstone payables held by f02.
func explicitServiceLiabilities(t *testing.T, state *reward19.State, streams *reward19.StreamsState) abi.TokenAmount {
	t.Helper()
	req := require.New(t)
	total := big.Zero()
	accruals := make(map[reward19.StreamID]abi.TokenAmount, len(state.Accrued))
	for _, accrual := range state.Accrued {
		accruals[accrual.ID] = accrual.Amount
	}
	for _, stream := range streams.Streams {
		if stream.Distribution == nil {
			continue
		}
		amount, ok := accruals[stream.ID]
		req.True(ok)
		total = big.Add(total, amount)
		for _, row := range stream.Distribution.Payable {
			total = big.Add(total, row.Amount)
		}
		for _, row := range stream.Distribution.ClaimedPeriod {
			total = big.Sub(total, row.Amount)
		}
	}
	for _, tombstone := range streams.Tombstones {
		for _, row := range tombstone.Payable {
			total = big.Add(total, row.Amount)
		}
	}
	return total
}

func rewardAllocationAt(t *testing.T, actor *types.Actor, state *reward19.State, streams *reward19.StreamsState) abi.TokenAmount {
	t.Helper()
	explicitLiabilities := explicitServiceLiabilities(t, state, streams)
	remainingReserve := big.Sub(actor.Balance, explicitLiabilities)
	return big.Add(state.TotalMintedReward, remainingReserve)
}

// requireShareWithinAtto checks part/total ≈ share/Denom with an attoFIL rounding allowance.
func requireShareWithinAtto(t *testing.T, part abi.TokenAmount, total abi.TokenAmount, share uint64, roundingAtto int64) {
	t.Helper()
	req := require.New(t)
	req.GreaterOrEqual(roundingAtto, int64(0))
	scaledActual := big.Mul(part, big.NewInt(int64(reward19.Denom)))
	scaledExpected := big.Mul(total, big.NewInt(int64(share)))
	scaledError := big.Sub(scaledActual, scaledExpected).Abs()
	scaledSlack := big.Mul(big.NewInt(roundingAtto), big.NewInt(int64(reward19.Denom)))
	req.LessOrEqual(big.Cmp(scaledError, scaledSlack), 0)
}

func streamByID(t *testing.T, streams *reward19.StreamsState, id reward19.StreamID) reward19.Stream {
	t.Helper()
	for _, stream := range streams.Streams {
		if stream.ID == id {
			return stream
		}
	}
	require.FailNowf(t, "stream is not live", "stream %d", id)
	return reward19.Stream{}
}

// accrualOf returns the current period's gross accrual for one explicit stream.
func accrualOf(t *testing.T, state *reward19.State, id reward19.StreamID) abi.TokenAmount {
	t.Helper()
	for _, accrual := range state.Accrued {
		if accrual.ID == id {
			return accrual.Amount
		}
	}
	require.FailNowf(t, "stream has no accrual", "stream %d", id)
	return big.Zero()
}

type solsticeRewardLifecycle struct {
	ctx         context.Context
	client      *kit.TestFullNode
	miner       *kit.TestMiner
	blockMiners []*kit.BlockMiner
	store       cbor.IpldStore

	pct            uint64
	upgradeEpoch   abi.ChainEpoch
	activation     abi.ChainEpoch
	timelock       abi.ChainEpoch
	bootstrapSlope uint64
	splitStart     abi.ChainEpoch
	splitEnd       abi.ChainEpoch
	bootstrap      buildconstants.SolsticeRewardBootstrapParams

	deployer       address.Address
	owner1         address.Address
	owner2         address.Address
	sraAddr        address.Address
	swaAddr        address.Address
	orchestrator   address.Address
	orchestratorID address.Address
	w3WriterKey    *key.Key
	w3WriterID     address.Address
	recipients     []address.Address
	newStream      reward19.StreamID

	gateQuarter       uint64
	gateVolumeUSD     big.Int
	belowThresholdUSD big.Int
	epochsPerQuarter  abi.ChainEpoch
	quarterStart      abi.ChainEpoch
	postingEnd        abi.ChainEpoch
	bindingEpoch      abi.ChainEpoch
	steppedWeight     reward19.WeightRecord

	preTS               *types.TipSet
	activationTS        *types.TipSet
	preSimpleTotal      abi.TokenAmount
	preBaselineTotal    abi.TokenAmount
	initialAllocation   abi.TokenAmount
	migrationInputActor *types.Actor
	migrationActor      *types.Actor
	migrationInput      reward18.State
	migrated            reward19.State
	migratedStreams     *reward19.StreamsState
}

type solsticeRewardDeltas struct {
	total   abi.TokenAmount
	service abi.TokenAmount
	burn    abi.TokenAmount
	miner   abi.TokenAmount
}

type solsticeClaimResult struct {
	tipset  *types.TipSet
	amounts []abi.TokenAmount
	deltas  []abi.TokenAmount
	state   *reward19.State
	streams *reward19.StreamsState
}

// testMigrationAndAwardContinuity checks v18 to v19 field mapping and that awards continue post-migration.
func (f *solsticeRewardLifecycle) testMigrationAndAwardContinuity(t *testing.T) {
	req := require.New(t)
	expectedCode, ok := actors.GetActorCodeID(actorstypes.Version19, manifest.RewardKey)
	req.True(ok)
	req.Equal(expectedCode, f.migrationActor.Code)

	// Input and output are the same epoch's state: the v18 parent state and its migrated form.
	req.Equal(f.migrationInputActor.Balance, f.migrationActor.Balance)
	req.Equal(f.migrationInput.CumsumBaseline, f.migrated.CumsumBaseline)
	req.Equal(f.migrationInput.CumsumRealized, f.migrated.CumsumRealized)
	req.Equal(f.migrationInput.EffectiveNetworkTime, f.migrated.EffectiveNetworkTime)
	req.Equal(f.migrationInput.EffectiveBaselinePower, f.migrated.EffectiveBaselinePower)
	req.Equal(f.migrationInput.ThisEpochReward, f.migrated.ThisEpochReward)
	req.Equal(f.migrationInput.ThisEpochRewardSmoothed.PositionEstimate, f.migrated.ThisEpochRewardSmoothed.PositionEstimate)
	req.Equal(f.migrationInput.ThisEpochRewardSmoothed.VelocityEstimate, f.migrated.ThisEpochRewardSmoothed.VelocityEstimate)
	req.Equal(f.migrationInput.ThisEpochBaselinePower, f.migrated.ThisEpochBaselinePower)
	req.Equal(f.migrationInput.Epoch, f.migrated.Epoch)
	req.Equal(f.migrationInput.TotalStoragePowerReward, f.migrated.TotalMintedReward)
	req.Equal(big.Zero(), f.migrated.TotalBurnMinted)
	req.Equal(big.Zero(), f.migrated.TotalExplicitMinted)
	req.Equal([]reward19.StreamAccrual{{ID: 2, Amount: big.Zero()}}, f.migrated.Accrued)
	req.Equal(f.timelock, f.migrated.SWATimelockEpochs)
	req.Equal(f.swaAddr, f.migrated.SWAActor)
	req.Len(f.migratedStreams.Streams, 2)
	req.Empty(f.migratedStreams.PendingWritesQueue)
	req.Empty(f.migratedStreams.Tombstones)
	req.Equal(f.activation, f.migratedStreams.Streams[0].Weight.TStart)
	req.Equal(f.activation, f.migratedStreams.Streams[1].Weight.TStart)
	req.Equal(f.bootstrap.ConsensusWeight.VStart, f.migratedStreams.Streams[0].Weight.VStart)
	req.Equal(f.bootstrap.ServiceWeight.VStart, f.migratedStreams.Streams[1].Weight.VStart)
	req.Equal(-int64(f.bootstrapSlope), f.migratedStreams.Streams[0].Weight.Slope)
	req.Equal(int64(f.bootstrapSlope), f.migratedStreams.Streams[1].Weight.Slope)
	req.Equal(reward19.StreamID(1), f.migratedStreams.Streams[0].ID)
	req.Equal(reward19.StreamID(2), f.migratedStreams.Streams[1].ID)
	req.Nil(f.migratedStreams.Streams[0].Distribution, "the consensus stream is implicit")
	req.NotNil(f.migratedStreams.Streams[1].Distribution)
	req.Equal(f.sraAddr, f.migratedStreams.Streams[1].Distribution.Writer)
	req.Equal([]reward19.RecipientShare{{Recipient: f.orchestratorID, Share: reward19.Denom}}, f.migratedStreams.Streams[1].Distribution.Shares)

	laterTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, f.upgradeEpoch+8)
	laterActor, later, laterStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, laterTS.Key())
	req.Positive(big.Cmp(later.TotalMintedReward, f.migrated.TotalMintedReward))
	req.Positive(big.Cmp(later.TotalExplicitMinted, f.migrated.TotalExplicitMinted))
	f.requireAllocation(t, laterActor, later, laterStreams)

	// The sole miner receives all issuance minus burn and explicit accrual, plus gas rewards.
	consensusMinted := big.Sub(
		big.Sub(later.TotalMintedReward, f.migrated.TotalMintedReward),
		big.Add(
			big.Sub(later.TotalExplicitMinted, f.migrated.TotalExplicitMinted),
			big.Sub(later.TotalBurnMinted, f.migrated.TotalBurnMinted)))
	req.Positive(consensusMinted.Sign())
	preMiner := kit.MustActor(f.ctx, t, f.client, f.miner.ActorAddr, f.activationTS.Key())
	postMiner := kit.MustActor(f.ctx, t, f.client, f.miner.ActorAddr, laterTS.Key())
	req.GreaterOrEqual(big.Cmp(big.Sub(postMiner.Balance, preMiner.Balance), consensusMinted), 0,
		"the miner must receive at least the consensus share minted since the migration")
}

// testCirculatingSupplyContinuity proves FilMined tracks TotalMintedReward across and after migration.
func (f *solsticeRewardLifecycle) testCirculatingSupplyContinuity(t *testing.T) {
	req := require.New(t)
	postMigrationTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, f.activationTS.Height()+1)
	postActor, postMigration, _ := kit.LoadReward19(f.ctx, t, f.client, f.store, postMigrationTS.Key())
	preSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, f.activationTS.Key())
	req.NoError(err)
	postSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, postMigrationTS.Key())
	req.NoError(err)
	mined := big.Sub(postSupply.FilMined, preSupply.FilMined)
	req.Equal(big.Sub(postMigration.TotalMintedReward, f.migrationInput.TotalStoragePowerReward), mined)
	// Explicit accrual stays in f02 until claimed; the rest of what is mined leaves at once.
	req.Equal(
		big.Sub(mined, big.Sub(postMigration.TotalExplicitMinted, f.migrated.TotalExplicitMinted)),
		big.Sub(f.migrationInputActor.Balance, postActor.Balance),
	)

	laterEpoch := postMigrationTS.Height() + 7
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(laterEpoch+1))
	laterTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, laterEpoch)
	laterActor, later, _ := kit.LoadReward19(f.ctx, t, f.client, f.store, laterTS.Key())
	laterSupply, err := f.client.StateVMCirculatingSupplyInternal(f.ctx, laterTS.Key())
	req.NoError(err)
	mined = big.Sub(laterSupply.FilMined, postSupply.FilMined)
	req.Equal(big.Sub(later.TotalMintedReward, postMigration.TotalMintedReward), mined)
	req.Equal(
		big.Sub(mined, big.Sub(later.TotalExplicitMinted, postMigration.TotalExplicitMinted)),
		big.Sub(postActor.Balance, laterActor.Balance),
	)
}

// testRewardTotalConstants distinguishes the v19 issuance constants from the v18 state totals.
func (f *solsticeRewardLifecycle) testRewardTotalConstants(t *testing.T) {
	req := require.New(t)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.activation+25))
	previous, current := adjacentReward19States(f.ctx, t, f.client, f.store, f.activation+5)
	prevTheta := reward19.ComputeRTheta(
		previous.EffectiveNetworkTime,
		previous.EffectiveBaselinePower,
		previous.CumsumRealized,
		previous.CumsumBaseline,
	)
	currTheta := reward19.ComputeRTheta(
		current.EffectiveNetworkTime,
		current.EffectiveBaselinePower,
		current.CumsumRealized,
		current.CumsumBaseline,
	)
	fromConstants := computeRewardWithTotals(
		current.Epoch, prevTheta, currTheta, reward19.SimpleTotal, reward19.BaselineTotal,
	)
	fromStoredTotals := computeRewardWithTotals(
		current.Epoch, prevTheta, currTheta, f.preSimpleTotal, f.preBaselineTotal,
	)

	req.NotEqual(fromStoredTotals, fromConstants)
	req.Equal(fromConstants, current.ThisEpochReward)
	req.NotEqual(fromStoredTotals, current.ThisEpochReward)
}

// testSlopedBootstrapWeight observes the consensus ramp reducing the miner share by at least slope*epochs.
func (f *solsticeRewardLifecycle) testSlopedBootstrapWeight(t *testing.T) {
	req := require.New(t)
	const window = abi.ChainEpoch(5)
	earlyStartEpoch := f.activation + 2
	earlyEndEpoch := earlyStartEpoch + window
	serviceRise := f.bootstrap.ServiceWeight.Cap - f.bootstrap.ServiceWeight.VStart
	serviceClampOffset := (serviceRise + f.bootstrapSlope - 1) / f.bootstrapSlope
	req.Less(uint64(earlyEndEpoch-f.activation), serviceClampOffset)
	lateStartEpoch := f.activation + 30
	lateEndEpoch := lateStartEpoch + window
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(lateEndEpoch+1))

	_, earlyStart, _ := f.stateAtOrAfter(t, earlyStartEpoch)
	_, earlyEnd, _ := f.stateAtOrAfter(t, earlyEndEpoch)
	_, lateStart, _ := f.stateAtOrAfter(t, lateStartEpoch)
	_, lateEnd, _ := f.stateAtOrAfter(t, lateEndEpoch)
	early := rewardDeltas(earlyStart, earlyEnd)
	late := rewardDeltas(lateStart, lateEnd)
	req.Positive(early.total.Sign())
	req.Positive(late.total.Sign())
	earlyAwardCount := int64(earlyEnd.Epoch - earlyStart.Epoch)
	req.Positive(earlyAwardCount)
	req.LessOrEqual(big.Cmp(early.burn, big.NewInt(earlyAwardCount)), 0, "bootstrap burn %s exceeds %d-award rounding bound", early.burn, earlyAwardCount)

	separation := lateStart.Epoch - earlyEnd.Epoch - 1
	req.Positive(separation)
	minimumDrop := uint64(separation) * f.bootstrapSlope
	actualDrop := big.Sub(big.Mul(early.miner, late.total), big.Mul(late.miner, early.total))
	minimumDropValue := big.Div(
		big.Mul(big.Mul(early.total, late.total), big.NewInt(int64(minimumDrop))),
		big.NewInt(int64(reward19.Denom)),
	)
	earlyRoundingSlack := big.Mul(big.NewInt(int64(earlyEnd.Epoch-earlyStart.Epoch)), late.total)
	lowerBound := big.Sub(minimumDropValue, earlyRoundingSlack)
	req.GreaterOrEqual(big.Sub(actualDrop, lowerBound).Sign(), 0,
		"miner share drop %s over %d epochs is below the slope's floor %s", actualDrop, separation, lowerBound)
}

// testRewardSplitEconomics checks settled bootstrap weights split gross issuance among miner, service, and burn.
func (f *solsticeRewardLifecycle) testRewardSplitEconomics(t *testing.T) {
	req := require.New(t)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.splitEnd+1))
	startActor, start, startStreams := f.stateAtOrAfter(t, f.splitStart)
	endActor, end, endStreams := f.stateAtOrAfter(t, f.splitEnd)
	delta := rewardDeltas(start, end)
	req.Positive(delta.total.Sign())
	req.Positive(delta.service.Sign())
	req.Positive(delta.burn.Sign())
	req.Positive(delta.miner.Sign())
	awardUpperBound := int64(end.Epoch - start.Epoch)
	req.Positive(awardUpperBound)
	expectedBurnShare := reward19.Denom - f.bootstrap.ConsensusWeight.Floor - f.bootstrap.ServiceWeight.Cap
	requireShareWithinAtto(t, delta.service, delta.total, f.bootstrap.ServiceWeight.Cap, awardUpperBound)
	requireShareWithinAtto(t, delta.miner, delta.total, f.bootstrap.ConsensusWeight.Floor, awardUpperBound)
	requireShareWithinAtto(t, delta.burn, delta.total, expectedBurnShare, 2*awardUpperBound)
	f.requireAllocation(t, startActor, start, startStreams)
	f.requireAllocation(t, endActor, end, endStreams)
}

// testQueueControlsAndEvent checks SWA auth, occupied-slot rejection, cancellation, and write-queued event.
func (f *solsticeRewardLifecycle) testQueueControlsAndEvent(t *testing.T) {
	req := require.New(t)
	updates := []reward19.WeightRecordUpdate{
		{ID: 1, Weight: flatWeight(65 * f.pct)},
		{ID: 2, Weight: flatWeight(10 * f.pct)},
	}
	beforeTS, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, _, beforeStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, beforeTS.Key())
	beforeWeights := []reward19.WeightRecord{beforeStreams.Streams[0].Weight, beforeStreams.Streams[1].Weight}

	// A single owner cancelling an empty slot is a no-op that proves proxy ownership and f02's
	// SWAActor.
	req.False(f.cancelPendingWeight(t, f.deployer).Receipt.ExitCode.IsSuccess(),
		"a non-owner must not reach SWA governance")
	kit.RequireMessageSuccess(t, f.cancelPendingWeight(t, f.owner1))

	unauthorized := f.sendRewardMessage(t, f.w3WriterKey.Address, builtin.MethodsReward.SetWeightRecordsExported,
		&reward19.SetWeightRecordsParams{Updates: updates})
	req.False(unauthorized.Receipt.ExitCode.IsSuccess(), "only the SWA may write weight records")

	// Advance only for message execution until the queued write is cancelled.
	// RPC and assertion work must not consume the pending write's timelock.
	req.Len(f.blockMiners, 1)
	blockMiner := f.blockMiners[0]
	blockMiner.Pause()
	paused := true
	defer func() {
		if paused {
			blockMiner.Restart()
		}
	}()
	mineReceipt := func(message cid.Cid) *lapi.MsgLookup {
		t.Helper()
		for {
			lookup, err := f.client.StateSearchMsg(f.ctx, types.EmptyTSK, message, lapi.LookbackNoLimit, false)
			req.NoError(err)
			if lookup != nil {
				return lookup
			}
			blockMiner.MineUntilBlock(f.ctx, f.client, nil)
		}
	}
	unanimousAndMine := func() (*lapi.MsgLookup, *lapi.MsgLookup) {
		t.Helper()
		selector := solsticeSetWeightRecords(t)
		input := solsticeWeightUpdatesInput(updates)
		first := solsticePush(f.ctx, t, f.client, f.owner1, f.swaAddr, selector, input)
		second := solsticePush(f.ctx, t, f.client, f.owner2, f.swaAddr, selector, input)
		return mineReceipt(first), mineReceipt(second)
	}
	firstQueued, secondQueued := unanimousAndMine()
	kit.RequireMessageSuccess(t, firstQueued)
	kit.RequireMessageSuccess(t, secondQueued)
	queuedLookup := solsticeLaterLookup(firstQueued, secondQueued)
	queuedActor, queued, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, queuedLookup.TipSet)
	f.requireAllocation(t, queuedActor, queued, queuedStreams)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.Equal(reward19.PendingWriteOpSetWeightRecords, pending.Op)

	firstCollision, secondCollision := unanimousAndMine()
	rejected := 0
	for _, lookup := range []*lapi.MsgLookup{firstCollision, secondCollision} {
		if !lookup.Receipt.ExitCode.IsSuccess() {
			rejected++
		}
	}
	req.Equal(1, rejected, "an occupied queue slot must reject the approval that reaches unanimity")
	_, _, collisionStreams := kit.LoadReward19(f.ctx, t, f.client, f.store,
		solsticeLaterLookup(firstCollision, secondCollision).TipSet)
	req.Len(collisionStreams.PendingWritesQueue, 1)
	req.Equal(pending, collisionStreams.PendingWritesQueue[0], "a rejected write leaves the queued one intact")

	cancel := mineReceipt(solsticePush(f.ctx, t, f.client, f.owner1, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "cancelPendingWeight(uint8)"),
		kit.EvmWordUint64(uint64(reward19.PendingWriteOpSetWeightRecords))))
	kit.RequireMessageSuccess(t, cancel)
	cancelReceiptTS, err := f.client.ChainGetTipSet(f.ctx, cancel.TipSet)
	req.NoError(err)
	cancelExecutionTS, err := f.client.ChainGetTipSet(f.ctx, cancelReceiptTS.Parents())
	req.NoError(err)
	req.Less(cancelExecutionTS.Height(), pending.EffectiveEpoch, "cancellation must execute before the queued write is due")
	cancelActor, cancelled, cancelledStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, cancel.TipSet)
	f.requireAllocation(t, cancelActor, cancelled, cancelledStreams)
	req.Empty(cancelledStreams.PendingWritesQueue)
	req.Equal(beforeWeights[0], cancelledStreams.Streams[0].Weight)
	req.Equal(beforeWeights[1], cancelledStreams.Streams[1].Weight)
	requireWriteQueuedEvent(f.ctx, t, f.client, pending)

	blockMiner.Restart()
	paused = false

	// The revert keeps the other owner's approval on record; a successful veto proves it is still
	// there.
	kit.RequireMessageSuccess(t, solsticeInvoke(f.ctx, t, f.client, f.owner1, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "veto(bytes32)"),
		solsticeTaskID(solsticeSetWeightRecords(t), solsticeWeightUpdatesInput(updates))))

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	_, _, afterStreams := f.stateAtOrAfter(t, pending.EffectiveEpoch+1)
	req.Empty(afterStreams.PendingWritesQueue)
	req.Equal(beforeWeights[0], afterStreams.Streams[0].Weight)
	req.Equal(beforeWeights[1], afterStreams.Streams[1].Weight)
}

// testDeferredWeightSchedule proves a queued schedule stays inert until due, then controls reward splits.
func (f *solsticeRewardLifecycle) testDeferredWeightSchedule(t *testing.T) {
	req := require.New(t)
	consensusWeight := flatWeight(70 * f.pct)
	serviceWeight := flatWeight(10 * f.pct)
	lookup := f.swaSetWeightRecords(t, []reward19.WeightRecordUpdate{
		{ID: 1, Weight: consensusWeight}, {ID: 2, Weight: serviceWeight},
	})
	kit.RequireMessageSuccess(t, lookup)

	receiptTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	parentTS, err := f.client.ChainGetTipSet(f.ctx, receiptTS.Parents())
	req.NoError(err)
	parentActor, parentState, parentStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, parentTS.Key())
	queuedActor, queuedState, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, receiptTS.Key())
	f.requireAllocation(t, parentActor, parentState, parentStreams)
	f.requireAllocation(t, queuedActor, queuedState, queuedStreams)
	req.Equal(parentStreams.Streams[0].Weight, queuedStreams.Streams[0].Weight)
	req.Equal(parentStreams.Streams[1].Weight, queuedStreams.Streams[1].Weight)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.Nil(pending.ID)
	req.Equal(reward19.PendingWriteOpSetWeightRecords, pending.Op)
	req.Equal(parentTS.Height()+f.timelock, pending.EffectiveEpoch)

	// Covers a due write on a non-null epoch only; the null-epoch case is untested.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+10))
	dueAwardTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch)
	dueActor, dueState, dueStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, dueAwardTS.Key())
	f.requireAllocation(t, dueActor, dueState, dueStreams)
	req.Len(dueStreams.PendingWritesQueue, 1)
	req.Equal(parentStreams.Streams[0].Weight, dueStreams.Streams[0].Weight)
	req.Equal(parentStreams.Streams[1].Weight, dueStreams.Streams[1].Weight)

	appliedTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, dueAwardTS.Height()+1)
	appliedActor, appliedState, appliedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, appliedActor, appliedState, appliedStreams)
	req.Empty(appliedStreams.PendingWritesQueue)
	req.Equal(consensusWeight, appliedStreams.Streams[0].Weight)
	req.Equal(serviceWeight, appliedStreams.Streams[1].Weight)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(appliedTS.Height()+20))
	endActor, endState, endStreams := f.stateAtOrAfter(t, appliedTS.Height()+10)
	f.requireAllocation(t, endActor, endState, endStreams)
	req.Equal(consensusWeight, endStreams.Streams[0].Weight)
	req.Equal(serviceWeight, endStreams.Streams[1].Weight)
	delta := rewardDeltas(appliedState, endState)
	awardUpperBound := int64(endState.Epoch - appliedState.Epoch)
	req.Positive(awardUpperBound)
	requireShareWithinAtto(t, delta.service, delta.total, 10*f.pct, awardUpperBound)
	requireShareWithinAtto(t, delta.burn, delta.total, 20*f.pct, 2*awardUpperBound)
	requireShareWithinAtto(t, delta.miner, delta.total, 70*f.pct, awardUpperBound)
}

// testQuarterlyGateAndShares covers volume posting, a passing weight step, share submission and
// payout.
func (f *solsticeRewardLifecycle) testQuarterlyGateAndShares(t *testing.T) {
	req := require.New(t)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.quarterStart))
	posted := solsticeInvoke(f.ctx, t, f.client, f.orchestrator, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "postVolume(uint64,uint256)"),
		kit.EvmWordUint64(f.gateQuarter), kit.EvmWordBytes(f.gateVolumeUSD.Int.Bytes()))
	kit.RequireMessageSuccess(t, posted)
	postedTS, err := f.client.ChainGetTipSet(f.ctx, posted.TipSet)
	req.NoError(err)
	req.Lessf(postedTS.Height(), f.postingEnd, "volume must land inside quarter %d's posting window", f.gateQuarter)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.bindingEpoch))
	gate := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "quarterlyGateCheck()"))
	kit.RequireMessageSuccess(t, gate)

	_, _, gateStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, gate.TipSet)
	req.Len(gateStreams.PendingWritesQueue, 1)
	pending := gateStreams.PendingWritesQueue[0]
	req.Nil(pending.ID)
	req.Equal(reward19.PendingWriteOpStepWeightRecords, pending.Op)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	appliedTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch+1)
	appliedActor, appliedState, appliedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, appliedActor, appliedState, appliedStreams)
	req.Empty(appliedStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, appliedStreams, 2).Weight)

	// Anyone may call submitShares once the quarter binds; the SRA writes stream 2's share map.
	submitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(f.gateQuarter))
	kit.RequireMessageSuccess(t, submitted)
	submittedTS, err := f.client.ChainGetTipSet(f.ctx, submitted.TipSet)
	req.NoError(err)
	_, _, nextBinds := f.quarterEpochs(f.gateQuarter + 1)
	req.Lessf(submittedTS.Height(), nextBinds,
		"quarter %d's shares must be submitted before the next quarter binds", f.gateQuarter)
	submittedActor, submittedState, submittedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, submitted.TipSet)
	f.requireAllocation(t, submittedActor, submittedState, submittedStreams)
	distribution := streamByID(t, submittedStreams, 2).Distribution
	req.Equal([]reward19.RecipientShare{{Recipient: f.orchestratorID, Share: reward19.Denom}}, distribution.Shares)
	req.Len(distribution.Payable, 1)
	req.Equal(f.orchestratorID, distribution.Payable[0].Recipient)
	settledPayable := distribution.Payable[0].Amount
	req.Positive(settledPayable.Sign())

	claim := f.claim(t, 2, []address.Address{f.orchestratorID})
	req.Len(claim.amounts, 1)
	req.Positive(big.Cmp(claim.amounts[0], settledPayable),
		"the claim carries the settled balance plus the current period's accrual")
	claimed := streamByID(t, claim.streams, 2).Distribution
	req.Empty(claimed.Payable)
	req.Len(claimed.ClaimedPeriod, 1)
	req.Equal(f.orchestratorID, claimed.ClaimedPeriod[0].Recipient)
	req.Equal(f.steppedWeight, streamByID(t, claim.streams, 2).Weight)
	f.requireServiceShareInForce(t, appliedState, claim.state, 15*f.pct)
}

// quarterEpochs returns a quarter's posting and binding epochs from the SRA's activation anchor.
func (f *solsticeRewardLifecycle) quarterEpochs(quarter uint64) (opens, postingEnds, binds abi.ChainEpoch) {
	offset := (abi.ChainEpoch(quarter) - abi.ChainEpoch(f.gateQuarter)) * f.epochsPerQuarter
	return f.quarterStart + offset, f.postingEnd + offset, f.bindingEpoch + offset
}

// testFailedGate checks that a below-threshold volume holds stream 2's weight but still submits
// shares.
func (f *solsticeRewardLifecycle) testFailedGate(t *testing.T) {
	req := require.New(t)
	quarter := f.gateQuarter + 1
	opens, postingEnds, binds := f.quarterEpochs(quarter)
	_, _, nextBinds := f.quarterEpochs(quarter + 1)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(opens))
	posted := solsticeInvoke(f.ctx, t, f.client, f.orchestrator, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "postVolume(uint64,uint256)"),
		kit.EvmWordUint64(quarter), kit.EvmWordBytes(f.belowThresholdUSD.Int.Bytes()))
	kit.RequireMessageSuccess(t, posted)
	postedTS, err := f.client.ChainGetTipSet(f.ctx, posted.TipSet)
	req.NoError(err)
	req.Lessf(postedTS.Height(), postingEnds, "volume must land inside quarter %d's posting window", quarter)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(binds))
	beforeTS, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, beforeState, beforeStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, beforeTS.Key())
	req.Empty(beforeStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, beforeStreams, 2).Weight)

	gate := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "quarterlyGateCheck()"))
	kit.RequireMessageSuccess(t, gate)
	gateActor, gateState, gateStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, gate.TipSet)
	f.requireAllocation(t, gateActor, gateState, gateStreams)
	req.Empty(gateStreams.PendingWritesQueue, "a volume below the threshold queues no weight step")
	req.Equal(f.steppedWeight, streamByID(t, gateStreams, 2).Weight)
	req.Len(streamByID(t, gateStreams, 2).Distribution.ClaimedPeriod, 1,
		"the passing quarter's claim is still the open period's only withdrawal")

	submitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(quarter))
	kit.RequireMessageSuccess(t, submitted)
	submittedTS, err := f.client.ChainGetTipSet(f.ctx, submitted.TipSet)
	req.NoError(err)
	req.Lessf(submittedTS.Height(), nextBinds,
		"quarter %d's shares must be submitted before the next quarter binds", quarter)
	submittedActor, submittedState, submittedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, submitted.TipSet)
	f.requireAllocation(t, submittedActor, submittedState, submittedStreams)
	req.Empty(submittedStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, submittedStreams, 2).Weight)
	// A failed gate leaves the volumes bound, so the map still reaches f02 and closes the period.
	distribution := streamByID(t, submittedStreams, 2).Distribution
	req.Equal([]reward19.RecipientShare{{Recipient: f.orchestratorID, Share: reward19.Denom}}, distribution.Shares)
	req.Empty(distribution.ClaimedPeriod, "the closed period carries no withdrawals into the new one")
	req.Len(distribution.Payable, 1)
	req.Equal(f.orchestratorID, distribution.Payable[0].Recipient)
	req.Positive(distribution.Payable[0].Amount.Sign())
	f.requireServiceShareInForce(t, beforeState, submittedState, 15*f.pct)
}

// testMissedVolumePost checks that an unposted quarter advances submission without touching f02.
func (f *solsticeRewardLifecycle) testMissedVolumePost(t *testing.T) {
	req := require.New(t)
	quarter := f.gateQuarter + 2
	_, _, binds := f.quarterEpochs(quarter)
	_, _, nextBinds := f.quarterEpochs(quarter + 1)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(binds))
	beforeTS, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	_, beforeState, beforeStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, beforeTS.Key())
	req.Empty(beforeStreams.PendingWritesQueue)
	req.Equal(f.steppedWeight, streamByID(t, beforeStreams, 2).Weight)

	gate := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "quarterlyGateCheck()"))
	kit.RequireMessageSuccess(t, gate)
	gateActor, gateState, gateStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, gate.TipSet)
	f.requireAllocation(t, gateActor, gateState, gateStreams)
	req.Empty(gateStreams.PendingWritesQueue, "an unposted quarter queues no weight step")
	req.Equal(f.steppedWeight, streamByID(t, gateStreams, 2).Weight)
	beforeDistribution := streamByID(t, gateStreams, 2).Distribution
	beforeAccrual := accrualOf(t, gateState, 2)

	submitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(quarter))
	kit.RequireMessageSuccess(t, submitted)
	submittedTS, err := f.client.ChainGetTipSet(f.ctx, submitted.TipSet)
	req.NoError(err)
	req.Lessf(submittedTS.Height(), nextBinds,
		"quarter %d's shares must be submitted before the next quarter binds", quarter)
	submittedActor, submittedState, submittedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, submitted.TipSet)
	f.requireAllocation(t, submittedActor, submittedState, submittedStreams)
	req.Equal(f.steppedWeight, streamByID(t, submittedStreams, 2).Weight)
	f.requireServiceShareInForce(t, beforeState, submittedState, 15*f.pct)
	req.Equal(beforeDistribution, streamByID(t, submittedStreams, 2).Distribution,
		"a quarter with no volume leaves the share map, payables and withdrawals untouched")
	req.Positive(big.Cmp(accrualOf(t, submittedState, 2), beforeAccrual),
		"awards keep accruing to the stream across the quarter")

	resubmitted := solsticeInvoke(f.ctx, t, f.client, f.deployer, f.sraAddr,
		solsticeSelector(t, "ServiceRewardsActor", "submitShares(uint64)"),
		kit.EvmWordUint64(quarter))
	req.False(resubmitted.Receipt.ExitCode.IsSuccess(), "a quarter cannot be submitted twice")
}

func (f *solsticeRewardLifecycle) testStreamRegistration(t *testing.T) {
	req := require.New(t)
	// Registration requires activation >= execution epoch + timelock; the slack covers inclusion
	// delay.
	const registerSlack = abi.ChainEpoch(20)
	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	activation := head.Height() + f.timelock + registerSlack
	weight := flatWeight(5 * f.pct)
	shares := []reward19.RecipientShare{{Recipient: f.recipients[0], Share: reward19.Denom}}

	lookup := f.requireUnanimous(t, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor",
			"registerStream(uint64,(int256,int256,uint64,int256,int256),address,(address,uint256)[],uint64)"),
		solsticeRegisterStreamInput(f.ctx, t, f.client, f.newStream, weight, f.w3WriterID, shares, activation))

	_, _, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.NotNil(pending.ID)
	req.Equal(f.newStream, *pending.ID)
	req.Equal(reward19.PendingWriteOpRegisterStream, pending.Op)
	req.Equal(activation, pending.EffectiveEpoch)
	req.Len(queuedStreams.Streams, 2, "a queued registration leaves the stream table alone")

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(activation+2))
	liveActor, liveState, liveStreams := f.stateAtOrAfter(t, activation+1)
	f.requireAllocation(t, liveActor, liveState, liveStreams)
	req.Empty(liveStreams.PendingWritesQueue)
	req.Len(liveStreams.Streams, 3)
	registered := streamByID(t, liveStreams, f.newStream)
	req.Equal(weight, registered.Weight)
	req.Equal(f.w3WriterID, registered.Distribution.Writer)
	req.Equal(shares, registered.Distribution.Shares)
	req.Positive(accrualOf(t, liveState, f.newStream).Sign())
}

// testShareSettlementAndWalletPayouts exercises share replacement, partial claims, and recipient balance changes.
func (f *solsticeRewardLifecycle) testShareSettlementAndWalletPayouts(t *testing.T) {
	req := require.New(t)
	lookup := f.sendRewardMessage(t, f.w3WriterKey.Address, builtin.MethodsReward.SetSharesExported, &reward19.SetSharesParams{
		ID: f.newStream,
		Shares: []reward19.RecipientShare{
			{Recipient: f.recipients[0], Share: 40 * f.pct},
			{Recipient: f.recipients[1], Share: 60 * f.pct},
		},
	})
	kit.RequireMessageSuccess(t, lookup)

	settledActor, settledState, settledStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	f.requireAllocation(t, settledActor, settledState, settledStreams)
	distribution := streamByID(t, settledStreams, f.newStream).Distribution
	req.NotNil(distribution)
	req.Equal([]reward19.RecipientShare{
		{Recipient: f.recipients[0], Share: 40 * f.pct},
		{Recipient: f.recipients[1], Share: 60 * f.pct},
	}, distribution.Shares)
	req.Len(distribution.Payable, 1)
	req.Equal(f.recipients[0], distribution.Payable[0].Recipient)
	settledPayable := distribution.Payable[0].Amount

	setSharesTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(setSharesTS.Height()+5))
	beforeClaimsTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, setSharesTS.Height()+5)
	beforeBalances := []abi.TokenAmount{
		kit.MustActor(f.ctx, t, f.client, f.recipients[0], beforeClaimsTS.Key()).Balance,
		kit.MustActor(f.ctx, t, f.client, f.recipients[1], beforeClaimsTS.Key()).Balance,
	}

	first := f.claim(t, f.newStream, f.recipients[:1])
	firstDistribution := streamByID(t, first.streams, f.newStream).Distribution
	req.Empty(firstDistribution.Payable)
	req.Len(firstDistribution.ClaimedPeriod, 1)
	req.Equal(f.recipients[0], firstDistribution.ClaimedPeriod[0].Recipient)
	firstCurrentClaim := firstDistribution.ClaimedPeriod[0].Amount
	req.Positive(firstCurrentClaim.Sign())
	req.Equal(big.Add(settledPayable, firstCurrentClaim), first.amounts[0])
	pendingRecipient2 := accruedShare(accrualOf(t, first.state, f.newStream), 60*f.pct)
	req.Positive(pendingRecipient2.Sign())

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(first.tipset.Height()+5))
	second := f.claim(t, f.newStream, f.recipients)
	secondDistribution := streamByID(t, second.streams, f.newStream).Distribution
	req.Empty(secondDistribution.Payable)
	req.Len(secondDistribution.ClaimedPeriod, 2)
	req.Equal(f.recipients[0], secondDistribution.ClaimedPeriod[0].Recipient)
	req.Equal(f.recipients[1], secondDistribution.ClaimedPeriod[1].Recipient)

	secondAccrual := accrualOf(t, second.state, f.newStream)
	currentRecipient1 := accruedShare(secondAccrual, 40*f.pct)
	currentRecipient2 := accruedShare(secondAccrual, 60*f.pct)
	req.Positive(big.Cmp(currentRecipient1, secondDistribution.ClaimedPeriod[0].Amount))
	req.Positive(big.Cmp(currentRecipient2, secondDistribution.ClaimedPeriod[1].Amount))
	req.Equal(big.Sub(secondDistribution.ClaimedPeriod[0].Amount, firstCurrentClaim), second.amounts[0])
	req.Equal(secondDistribution.ClaimedPeriod[1].Amount, second.amounts[1])
	req.Positive(big.Cmp(second.amounts[1], pendingRecipient2))

	cumulativePaid := []abi.TokenAmount{big.Add(first.amounts[0], second.amounts[0]), second.amounts[1]}
	req.Equal(big.Add(settledPayable, secondDistribution.ClaimedPeriod[0].Amount), cumulativePaid[0])
	req.Equal(secondDistribution.ClaimedPeriod[1].Amount, cumulativePaid[1])
	for i, recipient := range f.recipients {
		after := kit.MustActor(f.ctx, t, f.client, recipient, second.tipset.Key()).Balance
		req.Equal(cumulativePaid[i], big.Sub(after, beforeBalances[i]))
	}

	currentPeriodTotal := big.Add(secondDistribution.ClaimedPeriod[0].Amount, secondDistribution.ClaimedPeriod[1].Amount)
	requireShareWithinAtto(t, secondDistribution.ClaimedPeriod[0].Amount, currentPeriodTotal, 40*f.pct, 1)
	requireShareWithinAtto(t, secondDistribution.ClaimedPeriod[1].Amount, currentPeriodTotal, 60*f.pct, 1)
}

// testRemoveStreamTombstoneClaim exercises stream removal: queued, tombstoned, claimed, deleted.
func (f *solsticeRewardLifecycle) testRemoveStreamTombstoneClaim(t *testing.T) {
	req := require.New(t)
	setShares := f.sendRewardMessage(t, f.w3WriterKey.Address, builtin.MethodsReward.SetSharesExported, &reward19.SetSharesParams{
		ID: f.newStream,
		Shares: []reward19.RecipientShare{
			{Recipient: f.recipients[0], Share: 40 * f.pct},
			{Recipient: f.recipients[1], Share: 60 * f.pct},
		},
	})
	kit.RequireMessageSuccess(t, setShares)

	lookup := f.requireUnanimous(t, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "removeStream(uint64)"),
		kit.EvmWordUint64(uint64(f.newStream)))
	queuedTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	_, _, queuedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	req.Len(queuedStreams.PendingWritesQueue, 1)
	pending := queuedStreams.PendingWritesQueue[0]
	req.NotNil(pending.ID)
	req.Equal(f.newStream, *pending.ID)
	req.Equal(reward19.PendingWriteOpRemoveStream, pending.Op)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(pending.EffectiveEpoch+2))
	dueTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, pending.EffectiveEpoch)
	appliedTS := kit.TipsetAtOrAfter(f.ctx, t, f.client, dueTS.Height()+1)
	removedActor, removed, removedStreams := kit.LoadReward19(f.ctx, t, f.client, f.store, appliedTS.Key())
	f.requireAllocation(t, removedActor, removed, removedStreams)
	req.Len(removedStreams.Streams, 2)
	req.Equal(reward19.StreamID(1), removedStreams.Streams[0].ID)
	req.Equal(reward19.StreamID(2), removedStreams.Streams[1].ID)
	req.Len(removedStreams.Tombstones, 1)
	req.Equal(f.newStream, removedStreams.Tombstones[0].ID)
	req.Len(removedStreams.Tombstones[0].Payable, 2)
	payable := append([]reward19.RecipientAmount(nil), removedStreams.Tombstones[0].Payable...)

	// Stream 1 holds 70% from the deferred write and stream 2 15% from the gate, so while w3 waits
	// for removal the explicit share is 20% and the burn 10%. Each stream rounds its award down, an
	// atto per stream per epoch.
	_, liveStart, _ := f.stateAtOrAfter(t, queuedTS.Height())
	_, liveEnd, _ := f.stateAtOrAfter(t, pending.EffectiveEpoch)
	liveEpochs := int64(liveEnd.Epoch - liveStart.Epoch)
	req.Positive(liveEpochs)
	liveDelta := rewardDeltas(liveStart, liveEnd)
	requireShareWithinAtto(t, liveDelta.service, liveDelta.total, 20*f.pct, 2*liveEpochs)
	requireShareWithinAtto(t, liveDelta.burn, liveDelta.total, 10*f.pct, 3*liveEpochs)

	// Once the removal applies, w2's 15% is the whole explicit share and w3's 5% joins the burn.
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(appliedTS.Height()+6))
	_, goneStart, _ := f.stateAtOrAfter(t, appliedTS.Height()+1)
	_, goneEnd, _ := f.stateAtOrAfter(t, appliedTS.Height()+6)
	f.requireServiceShareInForce(t, goneStart, goneEnd, 15*f.pct)
	goneDelta := rewardDeltas(goneStart, goneEnd)
	requireShareWithinAtto(t, goneDelta.burn, goneDelta.total, 15*f.pct, 2*int64(goneEnd.Epoch-goneStart.Epoch))

	claim := f.claim(t, f.newStream, f.recipients)
	req.Len(claim.amounts, len(payable))
	for i, row := range payable {
		req.Equal(f.recipients[i], row.Recipient)
		req.Equal(row.Amount, claim.amounts[i])
		req.Equal(row.Amount, claim.deltas[i])
		req.Positive(claim.amounts[i].Sign())
	}
	req.Empty(claim.streams.Tombstones)
	req.Len(claim.streams.Streams, 2)

	// A claim on a stream that is neither live nor tombstoned returns zero per wallet.
	drained := f.claim(t, f.newStream, f.recipients)
	for i, amount := range drained.amounts {
		req.Zerof(amount.Sign(), "claiming the drained tombstone must pay recipient %d nothing", i)
	}
}

func (f *solsticeRewardLifecycle) sendRewardMessage(t *testing.T, from address.Address, method abi.MethodNum, params cbg.CBORMarshaler) *lapi.MsgLookup {
	t.Helper()
	req := require.New(t)
	serialized, serializeErr := actors.SerializeParams(params)
	req.NoError(serializeErr)
	nonce, err := f.client.MpoolGetNonce(f.ctx, from)
	req.NoError(err)
	message := &types.Message{
		To: builtin.RewardActorAddr, From: from, Nonce: nonce, Value: big.Zero(),
		Method: method, Params: serialized, GasLimit: 10_000_000,
		GasFeeCap: abi.NewTokenAmount(10_000), GasPremium: big.Zero(),
	}
	signed, err := f.client.WalletSignMessage(f.ctx, from, message)
	req.NoError(err)
	messageCID, err := f.client.MpoolPush(f.ctx, signed)
	req.NoError(err)
	lookup, err := f.client.StateWaitMsg(f.ctx, messageCID, 1, lapi.LookbackNoLimit, true)
	req.NoError(err)
	return lookup
}

// unanimousInvoke pushes both owners' approvals while mining is paused and returns both receipts.
// Either owner's approval may execute first, so callers read state at the later receipt.
func (f *solsticeRewardLifecycle) unanimousInvoke(t *testing.T, to address.Address, selector []byte, args ...[]byte) (*lapi.MsgLookup, *lapi.MsgLookup) {
	t.Helper()
	var first, second cid.Cid
	func() {
		for _, blockMiner := range f.blockMiners {
			blockMiner.Pause()
		}
		// A push that fails ends the subtest, and a chain left paused hangs every test after it.
		defer func() {
			for _, blockMiner := range f.blockMiners {
				blockMiner.Restart()
			}
		}()
		first = solsticePush(f.ctx, t, f.client, f.owner1, to, selector, args...)
		second = solsticePush(f.ctx, t, f.client, f.owner2, to, selector, args...)
	}()
	return solsticeWait(f.ctx, t, f.client, first), solsticeWait(f.ctx, t, f.client, second)
}

// requireUnanimous requires both approvals to succeed and returns the later receipt.
func (f *solsticeRewardLifecycle) requireUnanimous(t *testing.T, to address.Address, selector []byte, args ...[]byte) *lapi.MsgLookup {
	t.Helper()
	first, second := f.unanimousInvoke(t, to, selector, args...)
	kit.RequireMessageSuccess(t, first)
	kit.RequireMessageSuccess(t, second)
	return solsticeLaterLookup(first, second)
}

func (f *solsticeRewardLifecycle) swaSetWeightRecords(t *testing.T, updates []reward19.WeightRecordUpdate) *lapi.MsgLookup {
	t.Helper()
	return f.requireUnanimous(t, f.swaAddr, solsticeSetWeightRecords(t), solsticeWeightUpdatesInput(updates))
}

// cancelPendingWeight clears the discretionary weight slot, which any single owner may do.
func (f *solsticeRewardLifecycle) cancelPendingWeight(t *testing.T, from address.Address) *lapi.MsgLookup {
	t.Helper()
	return solsticeInvoke(f.ctx, t, f.client, from, f.swaAddr,
		solsticeSelector(t, "StreamWeightActor", "cancelPendingWeight(uint8)"),
		kit.EvmWordUint64(uint64(reward19.PendingWriteOpSetWeightRecords)))
}

func (f *solsticeRewardLifecycle) claim(t *testing.T, id reward19.StreamID, wallets []address.Address) solsticeClaimResult {
	t.Helper()
	req := require.New(t)
	lookup := f.sendRewardMessage(t, f.miner.OwnerKey.Address, builtin.MethodsReward.ClaimExported, &reward19.ClaimParams{ID: id, Wallets: wallets})
	kit.RequireMessageSuccess(t, lookup)
	var result reward19.ClaimReturn
	req.NoError(result.UnmarshalCBOR(bytes.NewReader(lookup.Receipt.Return)))
	req.Len(result.Amounts, len(wallets))

	claimTS, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
	req.NoError(err)
	beforeTS, err := f.client.ChainGetTipSet(f.ctx, claimTS.Parents())
	req.NoError(err)
	deltas := make([]abi.TokenAmount, len(wallets))
	for i, recipient := range wallets {
		before := kit.MustActor(f.ctx, t, f.client, recipient, beforeTS.Key()).Balance
		after := kit.MustActor(f.ctx, t, f.client, recipient, claimTS.Key()).Balance
		deltas[i] = big.Sub(after, before)
		// Equal big.Ints can differ in representation.
		req.Equalf(0, big.Cmp(result.Amounts[i], deltas[i]),
			"wallet %s balance moved by %s, not the claimed %s", recipient, deltas[i], result.Amounts[i])
	}
	actor, state, streams := kit.LoadReward19(f.ctx, t, f.client, f.store, lookup.TipSet)
	f.requireAllocation(t, actor, state, streams)
	return solsticeClaimResult{tipset: claimTS, amounts: result.Amounts, deltas: deltas, state: state, streams: streams}
}

func (f *solsticeRewardLifecycle) stateAtOrAfter(t *testing.T, epoch abi.ChainEpoch) (*types.Actor, *reward19.State, *reward19.StreamsState) {
	t.Helper()
	ts := kit.TipsetAtOrAfter(f.ctx, t, f.client, epoch)
	return kit.LoadReward19(f.ctx, t, f.client, f.store, ts.Key())
}

// requireServiceShareInForce checks the share f02 actually paid the explicit streams, not the
// stored weight. Stream 2 is the only explicit stream in the gate phases.
func (f *solsticeRewardLifecycle) requireServiceShareInForce(t *testing.T, start, end *reward19.State, share uint64) {
	t.Helper()
	delta := rewardDeltas(start, end)
	awardUpperBound := int64(end.Epoch - start.Epoch)
	require.Positive(t, awardUpperBound)
	requireShareWithinAtto(t, delta.service, delta.total, share, awardUpperBound)
}

func (f *solsticeRewardLifecycle) requireAllocation(t *testing.T, actor *types.Actor, state *reward19.State, streams *reward19.StreamsState) {
	t.Helper()
	require.Equal(t, f.initialAllocation, rewardAllocationAt(t, actor, state, streams))
}

func rewardDeltas(start, end *reward19.State) solsticeRewardDeltas {
	total := big.Sub(end.TotalMintedReward, start.TotalMintedReward)
	service := big.Sub(end.TotalExplicitMinted, start.TotalExplicitMinted)
	burn := big.Sub(end.TotalBurnMinted, start.TotalBurnMinted)
	return solsticeRewardDeltas{total: total, service: service, burn: burn, miner: big.Sub(big.Sub(total, service), burn)}
}

func flatWeight(weight uint64) reward19.WeightRecord {
	return reward19.WeightRecord{VStart: weight, Floor: weight, Cap: weight}
}

func accruedShare(amount abi.TokenAmount, share uint64) abi.TokenAmount {
	return big.Div(big.Mul(amount, big.NewInt(int64(share))), big.NewInt(int64(reward19.Denom)))
}

func requireWriteQueuedEvent(ctx context.Context, t *testing.T, node lapi.FullNode, pending reward19.PendingWrite) {
	t.Helper()
	req := require.New(t)
	var epochZero abi.ChainEpoch
	events, err := node.GetActorEventsRaw(ctx, &types.ActorEventFilter{
		Addresses:  []address.Address{builtin.RewardActorAddr},
		FromHeight: &epochZero,
	})
	req.NoError(err)
	expected := []types.EventEntry{
		eventEntry(t, 0x03, "$type", basicnode.NewString("write-queued")),
		eventEntry(t, 0x03, "op", basicnode.NewInt(int64(pending.Op))),
		eventEntry(t, 0x01, "effective-epoch", basicnode.NewInt(int64(pending.EffectiveEpoch))),
		eventEntry(t, 0x01, "payload", basicnode.NewBytes(pending.Payload)),
	}
	for _, event := range events {
		if len(event.Entries) > 2 &&
			event.Entries[0].Key == "$type" &&
			bytes.Equal(event.Entries[0].Value, expected[0].Value) &&
			event.Entries[2].Key == "effective-epoch" &&
			bytes.Equal(event.Entries[2].Value, expected[2].Value) {
			req.Equal(expected, event.Entries)
			return
		}
	}
	req.FailNow("write-queued event not found")
}

func eventEntry(t *testing.T, flags uint8, key string, value ipld.Node) types.EventEntry {
	t.Helper()
	encoded, err := ipld.Encode(value, dagcbor.Encode)
	require.NoError(t, err)
	return types.EventEntry{Flags: flags, Codec: uint64(multicodec.Cbor), Key: key, Value: encoded}
}

// solsticeInvoke sends an EVM call and returns its receipt without requiring a successful exit
// code.
func solsticeInvoke(
	ctx context.Context,
	t *testing.T,
	client *kit.TestFullNode,
	from, to address.Address,
	selector []byte,
	args ...[]byte,
) *lapi.MsgLookup {
	t.Helper()
	var input []byte
	for _, arg := range args {
		input = append(input, arg...)
	}
	lookup, err := client.EVM().InvokeSolidity(ctx, from, to, selector, input)
	require.NoError(t, err)
	return lookup
}

// solsticePush sends an EVM call without waiting for inclusion.
func solsticePush(
	ctx context.Context,
	t *testing.T,
	client *kit.TestFullNode,
	from, to address.Address,
	selector []byte,
	args ...[]byte,
) cid.Cid {
	t.Helper()
	var params bytes.Buffer
	require.NoError(t, cbg.WriteByteArray(&params, solsticeCalldata(selector, args...)))
	signed, err := client.MpoolPushMessage(ctx, &types.Message{
		To: to, From: from, Value: big.Zero(),
		Method: builtin.MethodsEVM.InvokeContract,
		// A quarter of the block leaves room for both approvals in one block.
		GasLimit: buildconstants.BlockGasLimit / 4,
		Params:   params.Bytes(),
	}, nil)
	require.NoError(t, err)
	return signed.Cid()
}

// solsticeLaterLookup returns the lookup whose tipset's parent state includes both approvals.
func solsticeLaterLookup(first, second *lapi.MsgLookup) *lapi.MsgLookup {
	if second.Height >= first.Height {
		return second
	}
	return first
}

// solsticeWait resolves a pushed call's receipt with the exit code left to the caller.
func solsticeWait(ctx context.Context, t *testing.T, client *kit.TestFullNode, message cid.Cid) *lapi.MsgLookup {
	t.Helper()
	lookup, err := client.StateWaitMsg(ctx, message, 1, lapi.LookbackNoLimit, false)
	require.NoError(t, err)
	return lookup
}

func solsticeCalldata(selector []byte, args ...[]byte) []byte {
	calldata := append([]byte(nil), selector...)
	for _, arg := range args {
		calldata = append(calldata, arg...)
	}
	return calldata
}

// solsticeTaskID is the identifier a unanimous call is recorded under, keccak256 of its calldata.
func solsticeTaskID(selector []byte, args ...[]byte) []byte {
	hasher := keccak.NewLegacyKeccak256()
	hasher.Write(solsticeCalldata(selector, args...))
	return hasher.Sum(nil)
}

func solsticeSetWeightRecords(t *testing.T) []byte {
	t.Helper()
	return solsticeSelector(t, "StreamWeightActor", "setWeightRecords((uint64,(int256,int256,uint64,int256,int256))[])")
}

// solsticeBool decodes a successful call's single ABI-encoded bool.
func solsticeBool(t *testing.T, lookup *lapi.MsgLookup) bool {
	t.Helper()
	kit.RequireMessageSuccess(t, lookup)
	out, err := cbg.ReadByteArray(bytes.NewBuffer(lookup.Receipt.Return), uint64(len(lookup.Receipt.Return)))
	require.NoError(t, err)
	require.Len(t, out, 32)
	return out[31] != 0
}

func solsticeIDAddress(t *testing.T, id uint64) address.Address {
	t.Helper()
	addr, err := address.NewIDAddress(id)
	require.NoError(t, err)
	return addr
}

// solsticeWeightWords encodes a WeightRecord as vStart, slope, tStart, floor, cap.
func solsticeWeightWords(record reward19.WeightRecord) []byte {
	words := kit.EvmWordUint64(record.VStart)
	words = append(words, kit.EvmWordInt64(record.Slope)...)
	words = append(words, kit.EvmWordUint64(uint64(record.TStart))...)
	words = append(words, kit.EvmWordUint64(record.Floor)...)
	return append(words, kit.EvmWordUint64(record.Cap)...)
}

// solsticeWeightUpdatesInput encodes an offset word, the array length and six-word (id, record)
// tuples.
func solsticeWeightUpdatesInput(updates []reward19.WeightRecordUpdate) []byte {
	input := kit.EvmWordUint64(32)
	input = append(input, kit.EvmWordUint64(uint64(len(updates)))...)
	for _, update := range updates {
		input = append(input, kit.EvmWordUint64(uint64(update.ID))...)
		input = append(input, solsticeWeightWords(update.Weight)...)
	}
	return input
}

// registerStream's nine-word head is id, WeightRecord, writer, shares offset and activation; the
// tail is the share count then (wallet, share) pairs.
func solsticeRegisterStreamInput(
	ctx context.Context,
	t *testing.T,
	client *kit.TestFullNode,
	id reward19.StreamID,
	weight reward19.WeightRecord,
	writer address.Address,
	shares []reward19.RecipientShare,
	activation abi.ChainEpoch,
) []byte {
	t.Helper()
	const headWords = 9
	input := kit.EvmWordUint64(uint64(id))
	input = append(input, solsticeWeightWords(weight)...)
	input = append(input, kit.EvmWordFromAddr(ctx, t, client, writer)...)
	input = append(input, kit.EvmWordUint64(headWords*32)...)
	input = append(input, kit.EvmWordUint64(uint64(activation))...)
	input = append(input, kit.EvmWordUint64(uint64(len(shares)))...)
	for _, share := range shares {
		input = append(input, kit.EvmWordFromAddr(ctx, t, client, share.Recipient)...)
		input = append(input, kit.EvmWordUint64(share.Share)...)
	}
	return input
}

const solsticeBundleDir = "contracts/solstice"

var solsticeUintType = regexp.MustCompile(`^uint(\d+)$`)

// solsticeArg pairs a constructor argument with its Solidity type for the manifest check.
type solsticeArg struct {
	typ string
	// data holds a static 32-byte word or an unpadded dynamic payload.
	data    []byte
	dynamic bool
}

func solsticeAddress(word []byte) solsticeArg {
	return solsticeArg{typ: "address", data: word}
}

func solsticeUint64(n uint64) solsticeArg {
	return solsticeArg{typ: "uint64", data: kit.EvmWordUint64(n)}
}

// solsticeBytes marks payload as a dynamic bytes constructor argument.
func solsticeBytes(payload []byte) solsticeArg {
	return solsticeArg{typ: "bytes", data: payload, dynamic: true}
}

type solsticeContract struct {
	CreationBytecode          string            `json:"creationBytecode"`
	ConstructorParameterTypes []string          `json:"constructorParameterTypes"`
	FunctionSelectors         map[string]string `json:"functionSelectors"`
}

func solsticeManifestEntry(t *testing.T, contract string) solsticeContract {
	t.Helper()

	var bundle struct {
		Contracts map[string]solsticeContract `json:"contracts"`
	}
	raw, err := os.ReadFile(filepath.Join(solsticeBundleDir, "manifest.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &bundle))

	entry, ok := bundle.Contracts[contract]
	require.Truef(t, ok, "%s missing from manifest.json", contract)
	return entry
}

// solsticeSelector computes sig's selector and checks the contract manifest declares it.
func solsticeSelector(t *testing.T, contract, sig string) []byte {
	t.Helper()

	declared, ok := solsticeManifestEntry(t, contract).FunctionSelectors[sig]
	require.Truef(t, ok, "%s does not declare %q in manifest.json", contract, sig)
	selector := kit.EthFunctionHash(sig)
	require.Equalf(t, declared, "0x"+hex.EncodeToString(selector),
		"%s selector for %q disagrees with manifest.json", contract, sig)
	return selector
}

// solsticeCreationCode appends the ABI-encoded args to the creation bytecode after checking
// their types.
func solsticeCreationCode(t *testing.T, contract string, args ...solsticeArg) []byte {
	t.Helper()

	entry := solsticeManifestEntry(t, contract)
	declared := entry.ConstructorParameterTypes
	encoded := make([]string, len(args))
	for i, arg := range args {
		encoded[i] = arg.typ
	}
	require.Equalf(t, declared, encoded,
		"%s constructor arguments do not match manifest.json's parameter types", contract)

	encodedFile, err := os.ReadFile(filepath.Join(solsticeBundleDir, entry.CreationBytecode))
	require.NoError(t, err)
	creationCode, err := hex.DecodeString(string(bytes.TrimSpace(encodedFile)))
	require.NoError(t, err)

	return append(creationCode, solsticeEncodeArgs(t, contract, args)...)
}

// solsticeEncodeArgs writes static words and dynamic offsets, then length-prefixed, 32-byte-padded
// tails.
func solsticeEncodeArgs(t *testing.T, contract string, args []solsticeArg) []byte {
	t.Helper()

	headSize := 32 * len(args)
	head := make([]byte, 0, headSize)
	var tail []byte
	for i, arg := range args {
		if arg.dynamic {
			head = append(head, kit.EvmWordUint64(uint64(headSize+len(tail)))...)
			tail = append(tail, kit.EvmWordUint64(uint64(len(arg.data)))...)
			tail = append(tail, arg.data...)
			if remainder := len(arg.data) % 32; remainder != 0 {
				tail = append(tail, make([]byte, 32-remainder)...)
			}
			continue
		}

		require.Lenf(t, arg.data, 32, "%s constructor argument %d is not a 32-byte word", contract, i)
		var usedBytes int
		switch {
		case arg.typ == "address":
			usedBytes = 20
		case solsticeUintType.MatchString(arg.typ):
			bits, err := strconv.Atoi(solsticeUintType.FindStringSubmatch(arg.typ)[1])
			require.NoError(t, err)
			usedBytes = bits / 8
		default:
			t.Fatalf("%s constructor argument %d has unhandled type %q", contract, i, arg.typ)
		}
		for _, b := range arg.data[:32-usedBytes] {
			require.Zerof(t, b, "%s constructor argument %d (%s) uses more than its declared %d bytes",
				contract, i, arg.typ, usedBytes)
		}
		head = append(head, arg.data...)
	}
	return append(head, tail...)
}
