package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	datacap19 "github.com/filecoin-project/go-state-types/builtin/v19/datacap"
	verifreg19 "github.com/filecoin-project/go-state-types/builtin/v19/verifreg"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/datacap"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

// TestMigrationNV29SolsticeDeals follows legacy verified and unverified deals through migration,
// native onboarding, quality upgrades, and termination, and checks the frozen datacap entry points.
func TestMigrationNV29SolsticeDeals(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	// Observe NV28 power after the first PoSt, which may need a full proving period after
	// seal randomness becomes available. Leave three more windows for onboarding and checks.
	upgradeEpoch := miner14.ChainFinality + miner.WPoStProvingPeriod() + 3*miner.WPoStChallengeWindow()

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

	t.Log("onboard legacy deal sectors and verify NV28 power")
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
		uint64(defaultSectorSize)*3,
		uint64(defaultSectorSize)*(1+1+10),
	)

	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "deal smoke assertions must run before the NV29 migration")

	legacyPre := make(map[abi.SectorNumber]*miner.SectorOnChainInfo)
	for _, sn := range []abi.SectorNumber{uvA, uvB, ver[0]} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, head.Key())
		req.NoError(err)
		req.NotNil(info)
		req.Less(info.Activation, upgradeEpoch, "deal sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy deal sector %d must not carry FULL_QA_POWER", sn)
		if sn == ver[0] {
			req.Positive(info.VerifiedDealWeight.Int64(), "legacy verified sector must be 10x by weight")
		} else {
			req.Zero(info.VerifiedDealWeight.Int64(), "unverified sector must have no verified weight")
			req.Positive(info.DealWeight.Int64(), "unverified sector must carry its 1x deal weight")
		}
		legacyPre[sn] = info
	}
	prePower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)

	t.Log("verify migration preserves legacy weights, flags, and power")
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	nv, err = client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	for _, sn := range []abi.SectorNumber{uvA, uvB, ver[0]} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, head.Key())
		req.NoError(err)
		req.NotNil(info)
		req.Equal(legacyPre[sn].VerifiedDealWeight, info.VerifiedDealWeight, "sector %d verified weight changed at migration", sn)
		req.Equal(legacyPre[sn].DealWeight, info.DealWeight, "sector %d deal weight changed at migration", sn)
		req.Equal(legacyPre[sn].Flags, info.Flags, "sector %d flags changed at migration", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must not gain FULL_QA_POWER", sn)
	}
	postPower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)
	req.Equal(prePower.MinerPower, postPower.MinerPower, "legacy raw and QA power must be unchanged across migration")

	// StateCall simulates each rejected mutation at the same migrated state; no call alters the fixture.
	assertSolsticeDatacapFrozen(ctx, t, client, head.Key())

	t.Log("compare a native unverified deal with its legacy twin")
	native, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(native, 1)
	um.WaitTillActivatedAndAssertPower(native,
		uint64(defaultSectorSize)*4,
		uint64(defaultSectorSize)*(1+1+10+10),
	)
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	nativeInfo, err := client.StateSectorGetInfo(ctx, maddr, native[0], head.Key())
	req.NoError(err)
	req.Greater(nativeInfo.Activation, upgradeEpoch, "native deal twin must be proven after the NV29 upgrade")
	req.NotZero(nativeInfo.Flags&miner.FULL_QA_POWER, "native deal sector must get FULL_QA_POWER regardless of content")
	req.Zero(nativeInfo.DealWeight.Int64(), "FULL_QA must zero the native sector's DealWeight")
	req.Positive(nativeInfo.VerifiedDealWeight.Int64(), "FULL_QA must store full-sector quality weight in VerifiedDealWeight")
	legacyInfo, err := client.StateSectorGetInfo(ctx, maddr, uvA, head.Key())
	req.NoError(err)
	req.Positive(legacyInfo.DealWeight.Int64(), "legacy twin must retain its 1x deal weight")
	req.Zero(legacyInfo.VerifiedDealWeight.Int64(), "legacy twin must not acquire verified weight")
	req.Zero(legacyInfo.Flags&miner.FULL_QA_POWER, "legacy twin must stay 1x while its native twin gets 10x")
	req.NotEqual(legacyInfo.Flags&miner.FULL_QA_POWER, nativeInfo.Flags&miner.FULL_QA_POWER,
		"content-identical sectors must differ in FULL_QA_POWER by onboarding epoch")

	t.Log("upgrade the legacy unverified deal and verify already-10x verified USQ is a no-op")
	preOp, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{uvA}, nil)
	req.NoError(err, "USQ of a legacy unverified deal sector must succeed")
	uvAInfo, err := client.StateSectorGetInfo(ctx, maddr, uvA, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(uvAInfo.Flags&miner.FULL_QA_POWER, "USQ must set FULL_QA_POWER on the unverified deal sector")
	postUsqUnver, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*9,
		postUsqUnver.MinerPower.QualityAdjPower.Uint64()-preOp.MinerPower.QualityAdjPower.Uint64(),
		"USQ of a 1x unverified deal sector must add 9x raw power")

	verPre, err := client.StateSectorGetInfo(ctx, maddr, ver[0], types.EmptyTSK)
	req.NoError(err)
	req.Zero(verPre.Flags&miner.FULL_QA_POWER, "verified deal sector must be 10x by weight, not flag")
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{ver[0]}, nil)
	req.NoError(err, "USQ of a legacy verified deal sector must succeed as a no-op")
	verPost, err := client.StateSectorGetInfo(ctx, maddr, ver[0], types.EmptyTSK)
	req.NoError(err)
	req.Zero(verPost.Flags&miner.FULL_QA_POWER, "USQ must not record FULL_QA_POWER on an already-10x-by-weight sector")
	postUsqVer, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(postUsqUnver.MinerPower.QualityAdjPower.String(), postUsqVer.MinerPower.QualityAdjPower.String(),
		"verified USQ must leave QA power at 10x, not 100x")

	t.Log("terminate 1x unverified and 10x verified deals and check each power removal")
	um.TerminateSectors([]abi.SectorNumber{uvB})
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*(10+10+10), // USQ'd legacy + verified + native remain.
		2*time.Minute)
	um.TerminateSectors([]abi.SectorNumber{ver[0]})
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*(10+10), // USQ'd legacy + native remain.
		2*time.Minute)

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeFullQaHelperSnapExtend verifies that miner.SectorIsFullQaPower
// correctly classifies legacy sectors after snap and extension, and that extensions on either
// side of the migration preserve verified 10x power and unverified/CC 1x power.
func TestMigrationNV29SolsticeFullQaHelperSnapExtend(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
	// Onboarding first needs seal randomness finality, and activation may then wait an entire
	// proving period. Allow three immutable windows for each of snap and the two extensions,
	// plus three windows for onboarding and message confirmations before crossing the fork.
	upgradeEpoch := miner14.ChainFinality + miner.WPoStProvingPeriod() + 12*miner.WPoStChallengeWindow()

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

	other, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2).AddSectorsWithRandomPieces(1))
	req.Len(other, 3)
	ccKeep, ccSnap, uv := other[0], other[1], other[2]

	um.WaitTillActivatedAndAssertPower([]abi.SectorNumber{ver[0], ccKeep, ccSnap, uv},
		uint64(defaultSectorSize)*4,
		uint64(defaultSectorSize)*(10+1+1+1),
	)
	for _, sn := range []abi.SectorNumber{ver[0], ccKeep, ccSnap, uv} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.Less(info.Activation, upgradeEpoch, "sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must not carry FULL_QA_POWER", sn)
	}

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
	t.Log("snap legacy CC to a verified deal and compare helper classifications to power")
	um.SnapDeal(ccSnap, kit.SectorWithVerifiedPiece(pSnap.PieceCID, &miner14.VerifiedAllocationKey{
		Client: snapClient,
		ID:     verifreg14.AllocationId(snapAlloc),
	}))
	kit.WaitForMinerQAP(ctx, t, client, maddr,
		uint64(defaultSectorSize)*(10+1+10+1),
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
	uvInfo, err := client.StateSectorGetInfo(ctx, maddr, uv, head.Key())
	req.NoError(err)
	req.False(miner.SectorIsFullQaPower(uvInfo), "legacy unverified deal must not classify as full-QA")
	req.Positive(uvInfo.DealWeight.Int64(), "unverified sector must carry deal weight")
	req.Zero(uvInfo.VerifiedDealWeight.Int64(), "unverified sector must have no verified weight")

	assertHelperMatchesPower(head.Key())

	t.Log("extend legacy CC and verified sectors before the migration")
	ccTarget := ccKeepInfo.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	req.Greater(ccTarget, upgradeEpoch, "the CC sector's new expiration must lie beyond the NV29 fork")
	um.ExtendSectorExpiration(ccKeep, ccTarget)
	ccMid, err := client.StateSectorGetInfo(ctx, maddr, ccKeep, types.EmptyTSK)
	req.NoError(err)
	req.Equal(ccTarget, ccMid.Expiration, "CC sector must be extended pre-migration")
	req.Zero(ccMid.Flags&miner.FULL_QA_POWER, "extend pre-migration must not promote the CC sector")
	req.False(miner.SectorIsFullQaPower(ccMid), "extended CC must still classify as 1x")
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	assertHelperMatchesPower(head.Key())

	// Extend the legacy verified sector across the NV29 boundary: the extension moves its power base
	// epoch/expiration, the case the helper's epoch choice must handle.
	claims, err := client.StateGetClaims(ctx, maddr, head.Key())
	req.NoError(err)
	var verClaims []verifreg14.ClaimId
	for id, claim := range claims {
		if claim.Sector == ver[0] {
			verClaims = append(verClaims, verifreg14.ClaimId(id))
		}
	}
	req.Len(verClaims, 1, "the directly onboarded verified piece must have a claim to maintain")
	verTarget := verInfo.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	um.ExtendSectorExpiration(ver[0], verTarget, verClaims...)

	head, err = client.ChainHead(ctx)
	req.NoError(err)
	verExtInfo, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	req.Equal(verTarget, verExtInfo.Expiration, "legacy verified sector must be extended")
	req.True(miner.SectorIsFullQaPower(verExtInfo), "extended legacy verified sector must still classify as full-QA")
	assertHelperMatchesPower(head.Key())
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "snap and the initial CC/verified extensions must complete on NV28")

	// Cross the NV29 boundary: the helper's verdicts and the miner's real power must be preserved.
	t.Log("check classifications and cross-fork CC extension survive migration")
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	nv, err = client.StateNetworkVersion(ctx, head.Key())
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
	req.Equal(ccMid.Expiration, ccKeepPost.Expiration, "CC expiration must be preserved across migration")
	req.Equal(ccMid.Flags, ccKeepPost.Flags, "CC flags must be preserved across migration")
	req.Zero(ccKeepPost.Flags&miner.FULL_QA_POWER, "cross-fork extended CC must not gain FULL_QA_POWER")
	req.False(miner.SectorIsFullQaPower(ccKeepPost), "legacy CC sector must remain 1x after migration")

	t.Log("extend legacy verified and unverified deals on NV29 without changing their power tiers")
	preExt, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	req.Positive(preExt.VerifiedDealWeight.Int64(), "verified sector must preserve verified weight across migration")
	prePower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)
	verTarget = preExt.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	um.ExtendSectorExpiration(ver[0], verTarget)
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	postExt, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	req.Equal(verTarget, postExt.Expiration, "verified deal sector must be extended on NV29")
	req.GreaterOrEqual(postExt.VerifiedDealWeight.Int64(), preExt.VerifiedDealWeight.Int64(),
		"no-drop-claims extend must keep (re-derive upward, never drop) the verified sector's weight")
	req.Positive(postExt.VerifiedDealWeight.Int64(), "verified sector must keep a verified weight after extend")
	req.Zero(postExt.Flags&miner.FULL_QA_POWER, "legacy verified sector must stay 10x via weight, not flag")
	req.True(miner.SectorIsFullQaPower(postExt), "verified extension must preserve full-QA classification")
	postPower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)
	req.Equal(prePower.MinerPower.QualityAdjPower.String(), postPower.MinerPower.QualityAdjPower.String(),
		"no-drop-claims extend must preserve verified 10x QA power")
	assertHelperMatchesPower(head.Key())

	uvInfo, err = client.StateSectorGetInfo(ctx, maddr, uv, head.Key())
	req.NoError(err)
	req.Zero(uvInfo.Flags&miner.FULL_QA_POWER, "unverified deal sector must remain 1x before its extension")
	uvTarget := uvInfo.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	um.ExtendSectorExpiration(uv, uvTarget)
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	uvPost, err := client.StateSectorGetInfo(ctx, maddr, uv, head.Key())
	req.NoError(err)
	req.Equal(uvTarget, uvPost.Expiration, "unverified deal sector must be extended")
	req.Zero(uvPost.VerifiedDealWeight.Int64(), "unverified deal sector must carry no verified weight")
	req.Zero(uvPost.Flags&miner.FULL_QA_POWER, "extend must not promote an unverified deal sector to FULL_QA_POWER")
	req.False(miner.SectorIsFullQaPower(uvPost), "extended unverified deal must still classify as 1x")
	uvPowerAfter, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)
	req.Equal(postPower.MinerPower.QualityAdjPower.String(), uvPowerAfter.MinerPower.QualityAdjPower.String(),
		"extend of a legacy unverified deal sector must preserve 1x QA power")
	assertHelperMatchesPower(head.Key())

	um.AssertNoWindowPostError()
}

// assertSolsticeDatacapFrozen checks FIP-0118's mutating entry points against migrated actors.
func assertSolsticeDatacapFrozen(ctx context.Context, t *testing.T, client api.FullNode, tsk types.TipSetKey) {
	t.Helper()
	from, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	var (
		verifregAddr = builtin.VerifiedRegistryActorAddr
		datacapAddr  = builtin.DatacapActorAddr
		amount       = big.Mul(big.NewInt(1<<30), builtin.TokenPrecision)
		allowance    = big.NewInt(1 << 30)
		// well-formed enough to deserialize; the actor never gets as far as verifying it
		signature = crypto.Signature{Type: crypto.SigTypeSecp256k1, Data: make([]byte, 65)}
		// FRC-46 token receiver hook payload type, per the frc46_token library
		frc46 = verifreg19.ReceiverType(builtin.MustGenerateFRCMethodNum("FRC46"))
	)

	type frozenMethod struct {
		name   string
		to     address.Address
		method abi.MethodNum
		params cbor.Marshaler
		// callerGated methods reject an account sender at caller validation, ahead of the
		// FIP-0118 refusal, so only the exit code is observable from a plain wallet.
		callerGated bool
	}

	cases := []frozenMethod{
		{
			name: "verifreg/AddVerifier", to: verifregAddr, method: verifreg.Methods.AddVerifier,
			params: &verifreg19.AddVerifierParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/RemoveVerifier", to: verifregAddr, method: verifreg.Methods.RemoveVerifier,
			params: &from,
		}, {
			name: "verifreg/AddVerifiedClient", to: verifregAddr, method: verifreg.Methods.AddVerifiedClient,
			params: &verifreg19.AddVerifiedClientParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/AddVerifiedClientExported", to: verifregAddr, method: verifreg.Methods.AddVerifiedClientExported,
			params: &verifreg19.AddVerifiedClientParams{Address: from, Allowance: allowance},
		}, {
			name: "verifreg/RemoveVerifiedClientDataCap", to: verifregAddr, method: verifreg.Methods.RemoveVerifiedClientDataCap,
			params: &verifreg19.RemoveDataCapParams{
				VerifiedClientToRemove: from,
				DataCapAmountToRemove:  allowance,
				VerifierRequest1:       verifreg19.RemoveDataCapRequest{Verifier: from, VerifierSignature: signature},
				VerifierRequest2:       verifreg19.RemoveDataCapRequest{Verifier: from, VerifierSignature: signature},
			},
		}, {
			name: "verifreg/RemoveExpiredAllocations", to: verifregAddr, method: verifreg.Methods.RemoveExpiredAllocations,
			params: &verifreg19.RemoveExpiredAllocationsParams{Client: abi.ActorID(100)},
		}, {
			name: "verifreg/RemoveExpiredAllocationsExported", to: verifregAddr, method: verifreg.Methods.RemoveExpiredAllocationsExported,
			params: &verifreg19.RemoveExpiredAllocationsParams{Client: abi.ActorID(100)},
		}, {
			name: "verifreg/ClaimAllocations", to: verifregAddr, method: verifreg.Methods.ClaimAllocations,
			params: &verifreg19.ClaimAllocationsParams{AllOrNothing: true}, callerGated: true,
		}, {
			name: "verifreg/ExtendClaimTerms", to: verifregAddr, method: verifreg.Methods.ExtendClaimTerms,
			params: &verifreg19.ExtendClaimTermsParams{},
		}, {
			name: "verifreg/ExtendClaimTermsExported", to: verifregAddr, method: verifreg.Methods.ExtendClaimTermsExported,
			params: &verifreg19.ExtendClaimTermsParams{},
		}, {
			name: "verifreg/RemoveExpiredClaims", to: verifregAddr, method: verifreg.Methods.RemoveExpiredClaims,
			params: &verifreg19.RemoveExpiredClaimsParams{Provider: abi.ActorID(1000)},
		}, {
			name: "verifreg/RemoveExpiredClaimsExported", to: verifregAddr, method: verifreg.Methods.RemoveExpiredClaimsExported,
			params: &verifreg19.RemoveExpiredClaimsParams{Provider: abi.ActorID(1000)},
		}, {
			// the allocation path: datacap tokens transferred to verifreg land here
			name: "verifreg/UniversalReceiverHook", to: verifregAddr, method: verifreg.Methods.UniversalReceiverHook,
			params: &verifreg19.UniversalReceiverParams{Type_: frc46}, callerGated: true,
		},

		{
			name: "datacap/Mint", to: datacapAddr, method: datacap.Methods.MintExported,
			params: &datacap19.MintParams{To: from, Amount: amount},
		}, {
			name: "datacap/Destroy", to: datacapAddr, method: datacap.Methods.DestroyExported,
			params: &datacap19.DestroyParams{Owner: from, Amount: amount},
		}, {
			name: "datacap/Transfer", to: datacapAddr, method: datacap.Methods.TransferExported,
			params: &datacap19.TransferParams{To: verifregAddr, Amount: amount},
		}, {
			name: "datacap/TransferFrom", to: datacapAddr, method: datacap.Methods.TransferFromExported,
			params: &datacap19.TransferFromParams{From: from, To: verifregAddr, Amount: amount},
		}, {
			name: "datacap/IncreaseAllowance", to: datacapAddr, method: datacap.Methods.IncreaseAllowanceExported,
			params: &datacap19.IncreaseAllowanceParams{Operator: from, Increase: amount},
		}, {
			name: "datacap/DecreaseAllowance", to: datacapAddr, method: datacap.Methods.DecreaseAllowanceExported,
			params: &datacap19.DecreaseAllowanceParams{Operator: from, Decrease: amount},
		}, {
			name: "datacap/RevokeAllowance", to: datacapAddr, method: datacap.Methods.RevokeAllowanceExported,
			params: &datacap19.RevokeAllowanceParams{Operator: from},
		}, {
			name: "datacap/Burn", to: datacapAddr, method: datacap.Methods.BurnExported,
			params: &datacap19.BurnParams{Amount: amount},
		}, {
			name: "datacap/BurnFrom", to: datacapAddr, method: datacap.Methods.BurnFromExported,
			params: &datacap19.BurnFromParams{Owner: from, Amount: amount},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, aerr := actors.SerializeParams(tc.params)
			require.NoError(t, aerr)

			res, err := client.StateCall(ctx, &types.Message{
				From:   from,
				To:     tc.to,
				Method: tc.method,
				Params: params,
				Value:  big.Zero(),
			}, tsk)
			require.NoError(t, err)

			require.Equal(t, exitcode.ErrForbidden, res.MsgRct.ExitCode, "error was: %s", res.Error)
			if !tc.callerGated {
				require.Contains(t, res.Error, "FIP-0118")
			}
		})
	}
}
