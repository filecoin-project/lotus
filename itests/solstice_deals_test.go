package itests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

// TestSolsticeDealSmokeNoUpgrade verifies deal recipe on NV28-only chain: unverified sector 1x, verified sector 10x.
func TestSolsticeDealSmokeNoUpgrade(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB

	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	bal := types.MustParseFIL("100fil").Int64()

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{
		RootKey: rootKey, VerifierKey: verifierKey, VerifiedClientKey: verifiedClientKey, Bal: bal,
	})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "smoke test must run on NV28 to observe pre-upgrade semantics")

	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

	sUnver, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(sUnver, 1)

	sVer, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSector(
		kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{
			Client: clientId,
			ID:     verifreg14.AllocationId(allocationId),
		})),
	)
	req.Len(sVer, 1)

	all := append(sUnver, sVer...)
	um.WaitTillActivatedAndAssertPower(all,
		uint64(defaultSectorSize)*2,                            // raw power
		uint64(defaultSectorSize)+uint64(defaultSectorSize)*10, // QAP: 1x + 10x
	)

	unverInfo, err := client.StateSectorGetInfo(ctx, maddr, sUnver[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(unverInfo)
	req.Zero(unverInfo.Flags&miner.FULL_QA_POWER, "unverified deal sector must not carry FULL_QA_POWER on NV28")
	req.Zero(unverInfo.VerifiedDealWeight.Int64(), "unverified deal sector must have no verified weight")
	req.Positive(unverInfo.DealWeight.Int64(), "unverified deal sector must carry (1x) deal weight")

	verInfo, err := client.StateSectorGetInfo(ctx, maddr, sVer[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(verInfo)
	req.Zero(verInfo.Flags&miner.FULL_QA_POWER, "verified deal sector's 10x on NV28 must come from verified weight, not FULL_QA_POWER")
	req.Positive(verInfo.VerifiedDealWeight.Int64(), "verified deal sector must carry verified weight")

	head, err = client.ChainHead(ctx)
	req.NoError(err)
	nv, err = client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "chain must not have left NV28 by the end of the smoke test")
}

// TestMigrationNV29SolsticeDealVariants checks deal-content classes are non-retroactive across NV28→NV29: verified stays 10x by weight, unverified stays 1x.
func TestMigrationNV29SolsticeDealVariants(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	bal := types.MustParseFIL("100fil").Int64()

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{
		UpgradeEpoch: upgradeEpoch, RootKey: rootKey, VerifierKey: verifierKey,
		VerifiedClientKey: verifiedClientKey, Bal: bal,
	})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

	sUnver, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(sUnver, 1)

	sVer, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSector(
		kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{
			Client: clientId,
			ID:     verifreg14.AllocationId(allocationId),
		})),
	)
	req.Len(sVer, 1)

	all := append(sUnver, sVer...)
	um.WaitTillActivatedAndAssertPower(all,
		uint64(defaultSectorSize)*2,                            // raw power
		uint64(defaultSectorSize)+uint64(defaultSectorSize)*10, // QAP: 1x + 10x
	)

	verPre, err := client.StateSectorGetInfo(ctx, maddr, sVer[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(verPre)
	req.Less(verPre.Activation, upgradeEpoch, "verified sector must be activated before the NV29 upgrade")
	req.Zero(verPre.Flags&miner.FULL_QA_POWER, "pre-upgrade verified sector must not carry FULL_QA_POWER on NV28")
	req.Positive(verPre.VerifiedDealWeight.Int64(), "pre-upgrade verified sector must carry verified weight")

	unverPre, err := client.StateSectorGetInfo(ctx, maddr, sUnver[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(unverPre)
	req.Less(unverPre.Activation, upgradeEpoch, "unverified sector must be activated before the NV29 upgrade")
	req.Zero(unverPre.Flags&miner.FULL_QA_POWER, "pre-upgrade unverified sector must not carry FULL_QA_POWER on NV28")
	req.Zero(unverPre.VerifiedDealWeight.Int64(), "pre-upgrade unverified sector must carry no verified weight")
	req.Positive(unverPre.DealWeight.Int64(), "pre-upgrade unverified sector must carry (1x) deal weight")

	prePower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	verPost, err := client.StateSectorGetInfo(ctx, maddr, sVer[0], head.Key())
	req.NoError(err)
	req.NotNil(verPost)
	req.Equal(verPre.VerifiedDealWeight, verPost.VerifiedDealWeight, "legacy verified DealWeight must be preserved across migration")
	req.Equal(verPre.DealWeight, verPost.DealWeight)
	req.Equal(verPre.Flags, verPost.Flags, "legacy verified sector Flags must be preserved (never gains FULL_QA_POWER)")
	req.Zero(verPost.Flags&miner.FULL_QA_POWER, "legacy verified sector must stay 10x via verified weight, not FULL_QA_POWER")

	unverPost, err := client.StateSectorGetInfo(ctx, maddr, sUnver[0], head.Key())
	req.NoError(err)
	req.NotNil(unverPost)
	req.Equal(unverPre.VerifiedDealWeight, unverPost.VerifiedDealWeight, "legacy unverified VerifiedDealWeight must be preserved")
	req.Zero(unverPost.Flags&miner.FULL_QA_POWER, "legacy unverified sector must stay 1x, not gain FULL_QA_POWER")

	postPower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)
	req.Equal(prePower.MinerPower.QualityAdjPower, postPower.MinerPower.QualityAdjPower,
		"miner QAP must be unchanged across migration (legacy verified 10x + legacy unverified 1x)")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeDealOps drives USQ and termination against deal-content tiers post-NV29: unverified USQ→10x, verified USQ→no-op, terminations remove correct power.
func TestMigrationNV29SolsticeDealOps(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	bal := types.MustParseFIL("100fil").Int64()

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{
		UpgradeEpoch: upgradeEpoch, RootKey: rootKey, VerifierKey: verifierKey,
		VerifiedClientKey: verifiedClientKey, Bal: bal,
	})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

	uv, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(2))
	req.Len(uv, 2)
	uvA, uvB := uv[0], uv[1]

	ver, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSector(
		kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{
			Client: clientId,
			ID:     verifreg14.AllocationId(allocationId),
		})),
	)
	req.Len(ver, 1)

	um.WaitTillActivatedAndAssertPower([]abi.SectorNumber{uvA, uvB, ver[0]},
		uint64(defaultSectorSize)*3, // raw power
		uint64(defaultSectorSize)+uint64(defaultSectorSize)+uint64(defaultSectorSize)*10, // QAP 1x+1x+10x
	)

	for _, sn := range []abi.SectorNumber{uvA, uvB, ver[0]} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.Less(info.Activation, upgradeEpoch, "deal sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "pre-upgrade deal sector %d must not carry FULL_QA_POWER", sn)
	}

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	preOp, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*(1+1+10), preOp.MinerPower.QualityAdjPower.Uint64(),
		"deal variants must be non-retroactive across migration")

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{uvA}, nil)
	req.NoError(err, "USQ of a legacy unverified deal sector must succeed")
	uvAInfo, err := client.StateSectorGetInfo(ctx, maddr, uvA, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(uvAInfo.Flags&miner.FULL_QA_POWER, "USQ must set FULL_QA_POWER on an unverified deal sector")
	postUsqUnver, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*9,
		postUsqUnver.MinerPower.QualityAdjPower.Uint64()-preOp.MinerPower.QualityAdjPower.Uint64(),
		"USQ of an unverified 1x deal sector must raise it to 10x (+9x raw)")

	// USQ on an already-10x-by-weight sector is a no-op: flag stays clear, power unchanged.
	verPre, err := client.StateSectorGetInfo(ctx, maddr, ver[0], types.EmptyTSK)
	req.NoError(err)
	req.Zero(verPre.Flags&miner.FULL_QA_POWER, "precondition: verified deal sector is 10x by weight, not flag")
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{ver[0]}, nil)
	req.NoError(err, "USQ of a legacy verified deal sector must succeed (no-op)")
	verPost, err := client.StateSectorGetInfo(ctx, maddr, ver[0], types.EmptyTSK)
	req.NoError(err)
	req.Zero(verPost.Flags&miner.FULL_QA_POWER,
		"USQ on an already-10x-by-weight sector must not record FULL_QA_POWER (nothing to upgrade)")
	postUsqVer, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(postUsqUnver.MinerPower.QualityAdjPower.String(), postUsqVer.MinerPower.QualityAdjPower.String(),
		"USQ of an already-10x verified deal sector must not change QA power (stays 10x, not 100x)")

	// uvB(1x) terminated; uvA(10x)+ver(10x) remain.
	um.TerminateSectors([]abi.SectorNumber{uvB})
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*(10+10), // ver 10x + uvA 10x remain (uvB's 1x removed)
		2*time.Minute)

	// ver(10x) terminated; only uvA's 10x remains.
	um.TerminateSectors([]abi.SectorNumber{ver[0]})
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*10, // only uvA (USQ'd to 10x) remains
		2*time.Minute)

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticePostUpgradeDeal compares content-identical deal sectors by provenance: NV28 legacy stays 1x, NV29 native gets FULL_QA_POWER(10x).
func TestMigrationNV29SolsticePostUpgradeDeal(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy,
		uint64(defaultSectorSize), // raw
		uint64(defaultSectorSize), // QAP: 1x by DealWeight
	)
	legacyInfo, err := client.StateSectorGetInfo(ctx, maddr, legacy[0], types.EmptyTSK)
	req.NoError(err)
	req.Less(legacyInfo.Activation, upgradeEpoch, "legacy deal twin must be proven before the NV29 upgrade")
	req.Zero(legacyInfo.Flags&miner.FULL_QA_POWER, "legacy deal twin must start 1x, without FULL_QA_POWER")
	req.Zero(legacyInfo.VerifiedDealWeight.Int64(), "legacy deal twin is an unverified deal (no verified weight)")
	req.Positive(legacyInfo.DealWeight.Int64(), "legacy deal twin must carry (1x) unverified deal weight")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	legacyAfter, err := client.StateSectorGetInfo(ctx, maddr, legacy[0], head.Key())
	req.NoError(err)
	req.Zero(legacyAfter.Flags&miner.FULL_QA_POWER, "legacy deal twin must NOT gain FULL_QA_POWER from the migration")

	// Native twin: same piece but proven on NV29, gets FULL_QA_POWER(10x) regardless of content.
	native, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(native, 1)
	um.WaitTillActivatedAndAssertPower(native,
		uint64(defaultSectorSize)*2,                              // raw: legacy(1) + native(1)
		uint64(defaultSectorSize)*1+uint64(defaultSectorSize)*10, // QAP: legacy 1x + native 10x
	)
	nativeInfo, err := client.StateSectorGetInfo(ctx, maddr, native[0], types.EmptyTSK)
	req.NoError(err)
	req.Greater(nativeInfo.Activation, upgradeEpoch, "native deal twin must be proven after the NV29 upgrade")
	req.NotZero(nativeInfo.Flags&miner.FULL_QA_POWER,
		"post-upgrade deal sector must be FULL_QA (10x) regardless of content")

	// FULL_QA zeroes DealWeight and stores quality weight in VerifiedDealWeight.
	req.Zero(nativeInfo.DealWeight.Int64(),
		"FULL_QA must zero DealWeight on a post-upgrade deal sector (content no longer shapes power)")
	req.NotZero(nativeInfo.VerifiedDealWeight.Int64(),
		"FULL_QA must bake the full-sector quality weight into VerifiedDealWeight")
	req.NotZero(legacyAfter.DealWeight.Int64(),
		"the legacy twin must keep its 1x DealWeight (no FULL_QA relocation)")
	req.Zero(legacyAfter.VerifiedDealWeight.Int64(),
		"the legacy twin must hold its quality weight in DealWeight, not VerifiedDealWeight")

	req.NotEqual(legacyAfter.Flags&miner.FULL_QA_POWER, nativeInfo.Flags&miner.FULL_QA_POWER,
		"content-identical deal sectors must differ in FULL_QA_POWER by onboarding epoch alone")

	pw, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	fullSet := 1   // the native twin
	legacySet := 1 // the legacy twin
	req.Equal(uint64(legacySet)*uint64(defaultSectorSize)+uint64(fullSet)*uint64(defaultSectorSize)*10,
		pw.MinerPower.QualityAdjPower.Uint64(),
		"filter-classified QAP (10x per FULL_QA deal sector, 1x per legacy) must equal the real miner QAP")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeFullQaHelperSnapExtend verifies that miner.SectorIsFullQaPower
// correctly classifies legacy verified sectors whose PowerBaseEpoch has been reset by a snap
// or extension, and that its verdict matches on-chain QA power at every step.
func TestMigrationNV29SolsticeFullQaHelperSnapExtend(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	bal := types.MustParseFIL("100fil").Int64()

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{
		UpgradeEpoch: upgradeEpoch, RootKey: rootKey, VerifierKey: verifierKey,
		VerifiedClientKey: verifiedClientKey, Bal: bal,
	})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	size := abi.PaddedPieceSize(defaultSectorSize)

	// Allocations for: the directly-onboarded legacy verified sector, and the CC sector we snap into
	// a verified deal.
	pVer := abi.PieceInfo{Size: size, PieceCID: kit.BogusPieceCid2}
	verClient, verAlloc := kit.SetupAllocation(ctx, t, client, minerId, pVer, verifiedClientAddr, 0, 0)
	pSnap := abi.PieceInfo{Size: size, PieceCID: kit.BogusPieceCid1}
	snapClient, snapAlloc := kit.SetupAllocation(ctx, t, client, minerId, pSnap, verifiedClientAddr, 0, 0)

	ver, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSector(
		kit.SectorWithVerifiedPiece(pVer.PieceCID, &miner14.VerifiedAllocationKey{
			Client: verClient,
			ID:     verifreg14.AllocationId(verAlloc),
		})),
	)
	req.Len(ver, 1)

	cc, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2))
	req.Len(cc, 2)
	ccKeep, ccSnap := cc[0], cc[1]

	um.WaitTillActivatedAndAssertPower([]abi.SectorNumber{ver[0], ccKeep, ccSnap},
		uint64(defaultSectorSize)*3, // raw power
		uint64(defaultSectorSize)*10+uint64(defaultSectorSize)+uint64(defaultSectorSize), // QAP: verified 10x + 1x + 1x
	)

	// assertHelperMatchesPower sums helper verdicts (10x or 1x) and checks against on-chain QA power.
	assertHelperMatchesPower := func(tsk types.TipSetKey) {
		active, err := client.StateMinerActiveSectors(ctx, maddr, tsk)
		req.NoError(err)
		power, err := client.StateMinerPower(ctx, maddr, tsk)
		req.NoError(err)

		sum := big.Zero()
		for _, info := range active {
			req.Greater(int64(info.Expiration), int64(info.PowerBaseEpoch),
				"sector %d has non-positive duration (pbe=%d exp=%d)", info.SectorNumber, info.PowerBaseEpoch, info.Expiration)
			mult := int64(1)
			if miner.SectorIsFullQaPower(info) {
				mult = 10
			}
			sum = big.Add(sum, big.Mul(big.NewInt(int64(defaultSectorSize)), big.NewInt(mult)))
		}
		req.Equal(power.MinerPower.QualityAdjPower.String(), sum.String(),
			"miner.SectorIsFullQaPower classification must match on-chain QA power")
	}

	// Snap CC into a verified deal pre-upgrade; this resets its PowerBaseEpoch.
	um.SnapDeal(ccSnap, kit.SectorWithVerifiedPiece(pSnap.PieceCID, &miner14.VerifiedAllocationKey{
		Client: snapClient,
		ID:     verifreg14.AllocationId(snapAlloc),
	}))
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*10+uint64(defaultSectorSize)+uint64(defaultSectorSize)*10, // 10x + 1x + 10x
		2*time.Minute)

	head, err := client.ChainHead(ctx)
	req.NoError(err)

	verInfo, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	req.True(miner.SectorIsFullQaPower(verInfo), "legacy verified sector must classify as full-QA")
	req.Zero(verInfo.Flags&miner.FULL_QA_POWER, "precondition: legacy verified sector is 10x by weight, not flag")
	req.GreaterOrEqual(verInfo.PowerBaseEpoch, verInfo.Activation, "invariant: PowerBaseEpoch >= Activation")

	ccSnapInfo, err := client.StateSectorGetInfo(ctx, maddr, ccSnap, head.Key())
	req.NoError(err)
	req.True(miner.SectorIsFullQaPower(ccSnapInfo),
		"legacy CC sector snapped to a verified deal must classify as full-QA")

	ccKeepInfo, err := client.StateSectorGetInfo(ctx, maddr, ccKeep, head.Key())
	req.NoError(err)
	req.False(miner.SectorIsFullQaPower(ccKeepInfo), "legacy CC sector must not classify as full-QA")

	assertHelperMatchesPower(head.Key())

	// Extend the legacy verified sector across the NV29 boundary: the extension moves its power base
	// epoch/expiration, the case the helper's epoch choice must handle.
	verTarget := verInfo.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	um.ExtendSectorExpiration(ver[0], verTarget)

	head, err = client.ChainHead(ctx)
	req.NoError(err)
	verExtInfo, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	req.Equal(verTarget, verExtInfo.Expiration, "legacy verified sector must be extended")
	req.True(miner.SectorIsFullQaPower(verExtInfo), "extended legacy verified sector must still classify as full-QA")
	assertHelperMatchesPower(head.Key())

	// Cross the NV29 boundary: the helper's verdicts and the miner's real power must be preserved.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	assertHelperMatchesPower(head.Key())
	for _, sn := range []abi.SectorNumber{ver[0], ccSnap} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, head.Key())
		req.NoError(err)
		req.True(miner.SectorIsFullQaPower(info), "sector %d must remain full-QA after migration", sn)
	}
	ccKeepPost, err := client.StateSectorGetInfo(ctx, maddr, ccKeep, head.Key())
	req.NoError(err)
	req.False(miner.SectorIsFullQaPower(ccKeepPost), "legacy CC sector must remain 1x after migration")

	um.AssertNoWindowPostError()
}
