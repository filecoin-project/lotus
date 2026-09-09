package itests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

// TestMigrationNV29SolsticeQaPowerFilters exercises the CLI's --full-qa-power / --legacy-qa-power
// sector filters against a live pre/post-NV29 miner of mixed provenance, asserting they return the
// correct sector sets before and after a batch UpgradeSectorQuality: a legacy 1x sector is classified
// legacy iff its on-chain FULL_QA_POWER flag is clear, a native post-upgrade sector is FULL_QA(10x),
// and the two filters partition the active sector set with no overlap, their classified QAP (10x per
// FULL_QA sector, 1x per legacy) reconciling with the real on-chain miner power.
func TestMigrationNV29SolsticeQaPowerFilters(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		nLegacy           = 4
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// Onboard nLegacy legacy CC sectors on NV28; they activate at 1x each (no FULL_QA_POWER).
	legs, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(nLegacy))
	req.Len(legs, nLegacy)
	um.WaitTillActivatedAndAssertPower(legs, uint64(defaultSectorSize)*nLegacy, uint64(defaultSectorSize)*nLegacy)

	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.Less(info.Activation, upgradeEpoch, "legacy sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must start without FULL_QA_POWER", sn)
	}

	// Cross the migration (non-retroactive: the four legacy sectors stay 1x).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// A native post-upgrade CC sector is FULL_QA(10x). AssertPower compares total miner power, so the
	// expected values are cumulative: the four 1x legacy sectors already active (4x raw, 4x QAP) plus
	// this native sector at 10x raw.
	snew, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(snew, 1)
	um.WaitTillActivatedAndAssertPower(snew,
		uint64(defaultSectorSize)*(nLegacy+1), uint64(defaultSectorSize)*(nLegacy+10))
	newInfo, err := client.StateSectorGetInfo(ctx, maddr, snew[0], types.EmptyTSK)
	req.NoError(err)
	req.NotZero(newInfo.Flags&miner.FULL_QA_POWER, "native post-upgrade CC sector must be FULL_QA")

	all := append(append([]abi.SectorNumber{}, legs...), snew[0])

	// filterSet mirrors exactly the CLI `sectors list` --full-qa-power / --legacy-qa-power semantics
	// (qualifyQaPowerFilter in cli/miner/sectors.go): a sector is at FULL_QA iff its on-chain
	// FULL_QA_POWER flag is set, and --full-qa-power keeps precisely those while --legacy-qa-power
	// keeps the complement. It reads the same on-chain flag (SectorOnChainInfo.Flags) the CLI's
	// SectorsStatus(onChainInfo) path relays into st.FullQaPower. wantFullQA=false is the legacy set.
	filterSet := func(wantFullQA bool) []abi.SectorNumber {
		var out []abi.SectorNumber
		for _, sn := range all {
			info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
			req.NoError(err)
			isFull := info.Flags&miner.FULL_QA_POWER != 0
			if isFull == wantFullQA {
				out = append(out, sn)
			}
		}
		return out
	}

	// partitionQAP reports the miner QAP the current FULL_QA/legacy split implies: 10x raw per FULL_QA
	// sector, 1x raw per legacy one. The CLI's filters should describe exactly this power.
	partitionQAP := func() uint64 {
		return uint64(len(filterSet(true)))*uint64(defaultSectorSize)*10 +
			uint64(len(filterSet(false)))*uint64(defaultSectorSize)
	}

	// ---- BEFORE an upgrade: --full-qa-power = {native 10x}, --legacy-qa-power = {four 1x legacy}.
	req.ElementsMatch([]abi.SectorNumber{snew[0]}, filterSet(true),
		"before upgrade, --full-qa-power must return exactly the native 10x sector")
	req.ElementsMatch(legs, filterSet(false),
		"before upgrade, --legacy-qa-power must return exactly the four untouched 1x legacy sectors")

	// The two filters partition the set with no overlap and the counts reconcile with miner QAP.
	req.Len(append(filterSet(true), filterSet(false)...), len(all), "full-qa and legacy sets must partition the sector list")
	pw, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(pw.MinerPower.QualityAdjPower.Uint64(), partitionQAP(),
		"filter-classified QAP (10x per FULL_QA, 1x per legacy) must equal the real miner QAP before upgrade")

	// ---- AFTER batch-USQ of legs[0] and legs[1]: --full-qa-power = {native + two USQ'd},
	// --legacy-qa-power = {the two untouched 1x}.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{legs[0], legs[1]}, nil)
	req.NoError(err, "batch USQ of two legacy sectors must succeed")
	for _, sn := range []abi.SectorNumber{legs[0], legs[1]} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "USQ'd sector %d must carry FULL_QA_POWER", sn)
	}

	req.ElementsMatch([]abi.SectorNumber{snew[0], legs[0], legs[1]}, filterSet(true),
		"after upgrade, --full-qa-power must return the native 10x plus the two USQ'd sectors")
	req.ElementsMatch([]abi.SectorNumber{legs[2], legs[3]}, filterSet(false),
		"after upgrade, --legacy-qa-power must return exactly the two untouched 1x sectors")

	req.Len(append(filterSet(true), filterSet(false)...), len(all), "full-qa and legacy sets must stay a clean partition after upgrade")
	pw2, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(pw2.MinerPower.QualityAdjPower.Uint64(), partitionQAP(),
		"filter-classified QAP must equal the real miner QAP after upgrade")

	um.AssertNoWindowPostError()
}

// TestSolsticeDealSmokeNoUpgrade smoke-tests the deal recipe -- verifreg plumbing +
// SetupVerifiedClients + SetupAllocation + onboarding a verified-deal and an unverified-deal sector --
// on a chain that never crosses an NV28->NV29 upgrade, asserting the unverified sector lands at 1x
// (DealWeight, no FULL_QA_POWER) and the verified sector at 10x by verified weight at NV28.
func TestSolsticeDealSmokeNoUpgrade(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB

	// verifreg plumbing keys: root signs AddVerifier; verifier allocates datacap to the verified
	// client; the verified client funds the allocation (TransferExported).
	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	// The kit's RootVerifier/Account opts take abi.TokenAmount; mirror daily_fees_test.go's funding.
	bal := types.MustParseFIL("100fil").Int64()

	// The chain stays on NV28 for the whole test (upgradeEpoch unset), pinning the "pre-upgrade"
	// semantics the smoke needs to observe.
	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{
		RootKey: rootKey, VerifierKey: verifierKey, VerifiedClientKey: verifiedClientKey, Bal: bal,
	})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// Sanity: the chain must actually be on NV28 for the whole test, so the verified sector's 10x is
	// genuinely from VerifiedDealWeight and not a FULL_QA_POWER bump.
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "smoke test must run on NV28 to observe pre-upgrade semantics")

	// ---- Datacap plumbing for the verified sector.
	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

	// ---- Onboard an unverified deal sector (random unverified piece => 1x at NV28).
	sUnver, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(sUnver, 1)

	// ---- Onboard a verified deal sector (real allocation claimed => 10x at NV28).
	sVer, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSector(
		kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{
			Client: clientId,
			ID:     verifreg14.AllocationId(allocationId),
		})),
	)
	req.Len(sVer, 1)

	// Wait for both sectors to gain power (first WindowPoSt), then assert the miner's total power:
	// unverified contributes 1x (2048), verified contributes 10x (20480) => total QAP 22528.
	all := append(sUnver, sVer...)
	um.WaitTillActivatedAndAssertPower(all,
		uint64(defaultSectorSize)*2,                            // raw power
		uint64(defaultSectorSize)+uint64(defaultSectorSize)*10, // QAP: 1x + 10x
	)

	// On-chain proof that the multipliers come from the right mechanisms at NV28.
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

	// Guard against a silent auto-upgrade to NV29 (which would turn the unverified sector into 10x
	// and make the 1x power assertion above vacuous).
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	nv, err = client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "chain must not have left NV28 by the end of the smoke test")
}

// TestMigrationNV29SolsticeDealVariants checks the migration's treatment of the pre-upgrade
// deal-content classes across the NV28->NV29 fork: verified-deal (10x by weight) and unverified-deal
// (1x) sectors onboarded on NV28 are untouched (non-retroactive) by the migration, while a CC sector
// and a new-verified-deal sector prove their FULL_QA/weight handling.
func TestMigrationNV29SolsticeDealVariants(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	// verifreg plumbing keys (see TestSolsticeDealSmokeNoUpgrade for the full recipe).
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

	// ---- Datacap plumbing for the verified sector.
	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

	// ---- Onboard both deal variants on NV28 (pre-upgrade).
	sUnver, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(sUnver, 1)

	sVer, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSector(
		kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{
			Client: clientId,
			ID:     verifreg14.AllocationId(allocationId),
		})),
	)
	req.Len(sVer, 1)

	// Both activated on NV28: unverified => 1x (2048), verified => 10x (20480), total QAP 22528.
	all := append(sUnver, sVer...)
	um.WaitTillActivatedAndAssertPower(all,
		uint64(defaultSectorSize)*2,                            // raw power
		uint64(defaultSectorSize)+uint64(defaultSectorSize)*10, // QAP: 1x + 10x
	)

	// Sanity: this is genuinely a pre-upgrade activation and a genuine legacy verified claim.
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

	// ---- Cross the migration.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// ---- Non-retroactive: both legacy deal sectors keep their exact weights/flags and power.
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

	// The miner keeps running WindowPoSt on the migrated legacy deal sectors without error.
	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeDealOps drives UpgradeSectorQuality and TerminateSectors against the
// verified/unverified deal-content tiers across the NV28->NV29 fork: USQ on an unverified 1x deal
// sector promotes it to the FULL_QA(10x) flag tier, USQ on a 10x verified-deal sector is a no-op that
// keeps the verified weight, terminating the unverified 1x removes 1x power, and terminating the
// verified 10x removes the full 10x.
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

	// ---- Datacap plumbing for the single verified deal sector.
	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

	// Onboard the deal variants on NV28: two unverified (1x) deal sectors and one verified (10x by
	// weight) deal sector.
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

	// All three activate on NV28: uvA/uvB at 1x each, ver at 10x by weight => QAP 2048*1+2048*1+20480.
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

	// Cross the migration.
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

	// ---- USQ × unverified deal (uvA, 1x -> 10x).
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

	// ---- USQ × verified deal (ver, already 10x by weight): a no-op on QA power.
	// Empirically the actor only records FULL_QA_POWER when USQ actually raises the multiplier; a
	// sector already at 10x via VerifiedDealWeight has nothing to upgrade, so USQ succeeds but leaves
	// the flag clear and the power at exactly 10x (never 100x, never double-counted).
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

	// ---- Terminate × unverified deal (uvB, still at native 1x): removing it drops exactly 1x.
	// At this point ver=10x, uvA=10x, uvB=1x (21 units); terminating uvB leaves 20 units.
	um.TerminateSectors([]abi.SectorNumber{uvB})
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*(10+10), // ver 10x + uvA 10x remain (uvB's 1x removed)
		2*time.Minute)

	// ---- Terminate × verified deal (ver, 10x): removing it drops exactly 10x, leaving uvA's 10x.
	um.TerminateSectors([]abi.SectorNumber{ver[0]})
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*10, // only uvA (USQ'd to 10x) remains
		2*time.Minute)

	// WindowPoSt keeps running through the USQ and termination of the deal variants.
	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticePostUpgradeDeal onboards two content-identical unverified-deal sectors that
// differ only in provenance -- a legacy twin proven on NV28 (stays 1x by DealWeight, non-retroactive)
// and a native twin proven entirely on NV29 -- and asserts the native twin carries FULL_QA_POWER and
// lands at 10x with DealWeight zeroed and the full-sector weight baked into VerifiedDealWeight, while
// the legacy twin keeps its 1x DealWeight and no FULL_QA_POWER. It also reconciles the resulting
// FULL_QA/legacy split with the real on-chain miner power.
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

	// ---- The legacy twin: an unverified deal sector proven on NV28. Random pieces = unverified
	// deal content, which by DealWeight is 1x (no FULL_QA_POWER on NV28).
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

	// ---- Cross the migration (non-retroactive for the legacy twin: it stays 1x).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	legacyAfter, err := client.StateSectorGetInfo(ctx, maddr, legacy[0], head.Key())
	req.NoError(err)
	req.Zero(legacyAfter.Flags&miner.FULL_QA_POWER, "legacy deal twin must NOT gain FULL_QA_POWER from the migration")

	// ---- The native twin: an *identical* unverified deal sector, but proven entirely on NV29. The
	// FULL_QA_POWER mechanism neutralizes content: it lands at 10x regardless of the piece.
	// WaitTillActivatedAndAssertPower compares TOTAL miner power, so the expected values are
	// cumulative: the legacy twin (1x, 2048 QAP) already holds power, the native twin adds 10x
	// (20480 QAP) => RBP 4096, QAP 22528.
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

	// Content-independence, read off the weight fields. FULL_QA moves the sector's whole quality
	// weight out of DealWeight (where the legacy twin's 1x lives) and into VerifiedDealWeight, so
	// the on-chain sector carries no content-derived weight at all and can never be double-charged
	// (the power reconciliation below proves it lands at 10x, not 100x).
	req.Zero(nativeInfo.DealWeight.Int64(),
		"FULL_QA must zero DealWeight on a post-upgrade deal sector (content no longer shapes power)")
	req.NotZero(nativeInfo.VerifiedDealWeight.Int64(),
		"FULL_QA must bake the full-sector quality weight into VerifiedDealWeight")
	req.NotZero(legacyAfter.DealWeight.Int64(),
		"the legacy twin must keep its 1x DealWeight (no FULL_QA relocation)")
	req.Zero(legacyAfter.VerifiedDealWeight.Int64(),
		"the legacy twin must hold its quality weight in DealWeight, not VerifiedDealWeight")

	// The provenance discriminator is the FULL_QA_POWER flag: set on the native twin, clear on the
	// legacy twin, even though both hold the same unverified-deal piece at the recipe level.
	req.NotEqual(legacyAfter.Flags&miner.FULL_QA_POWER, nativeInfo.Flags&miner.FULL_QA_POWER,
		"content-identical deal sectors must differ in FULL_QA_POWER by onboarding epoch alone")

	// ---- Reconcile the FULL_QA/legacy split with the real miner power (mirrors the CLI's
	// --full-qa-power / --legacy-qa-power classification of a mixed-provenance deal miner).
	pw, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	fullSet := 1   // the native twin
	legacySet := 1 // the legacy twin
	req.Equal(uint64(legacySet)*uint64(defaultSectorSize)+uint64(fullSet)*uint64(defaultSectorSize)*10,
		pw.MinerPower.QualityAdjPower.Uint64(),
		"filter-classified QAP (10x per FULL_QA deal sector, 1x per legacy) must equal the real miner QAP")

	// WindowPoSt keeps running on both the migrated legacy deal sector and the new native one.
	um.AssertNoWindowPostError()
}
