package itests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/itests/solsticekit"
	"github.com/filecoin-project/lotus/lib/must"
)

// TestMigrationNV29SolsticePreCommitProve proves a CC sector that is pre-committed before the NV29
// upgrade lands at FULL_QA(10x) after being proved (activated) on NV29, since the QA tier is decided
// at prove time. It pauses between PreCommit and Prove (kit's split helpers) so the migration happens
// while the sector is still unproven.
func TestMigrationNV29SolsticePreCommitProve(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const upgradeEpoch = abi.ChainEpoch(3000)

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB

	// Pre-commit a CC sector on NV28, pausing BEFORE ProveCommit.
	sn, err := um.PreCommitSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.NoError(err)
	req.Len(sn, 1)

	// Guard: the PreCommit must have been submitted while the chain is still on NV28.
	preHead, err := client.ChainHead(ctx)
	req.NoError(err)
	req.Less(preHead.Height(), upgradeEpoch, "PreCommit must be submitted before the NV29 fork")
	preNv, err := client.StateNetworkVersion(ctx, preHead.Key())
	req.NoError(err)
	req.Equal(network.Version28, preNv, "PreCommit must be submitted on NV28")

	// The sector is pre-committed but not yet proved/activated.
	notYet, err := client.StateSectorGetInfo(ctx, maddr, sn[0], types.EmptyTSK)
	req.NoError(err)
	req.Nil(notYet, "a pre-committed-but-unproved sector must not yet be committed")

	// Cross the migration to NV29 while the sector sits un-proved.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// Prove (activate) the pre-committed CC sector on NV29.
	committed, err := um.ProvePrecommittedSectors(sealProofType, sn)
	req.NoError(err)
	req.Equal(sn, committed, "the prepared sector must be successfully proved")

	// It lands as a FULL_QA 10x sector even though its PreCommit was on NV28.
	um.WaitTillActivatedAndAssertPower(committed,
		uint64(defaultSectorSize), uint64(defaultSectorSize)*10)

	info, err := client.StateSectorGetInfo(ctx, maddr, sn[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(info)
	req.GreaterOrEqual(info.Activation, upgradeEpoch, "sector must be activated on NV29 (prove decides activation)")
	req.NotZero(info.Flags&miner.FULL_QA_POWER,
		"a CC sector precommitted NV28 but proved NV29 must carry FULL_QA_POWER (activation-time rule)")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticePrecommitDeposit pauses a native NV29 CC sector at its precommit boundary
// and asserts the miner's on-chain PreCommitDeposits field is reserved (>0) while the sector is
// unproven, then fully released back to 0 once the sector proves and its FULL-QA pledge locks, with
// the released deposit funding an initial pledge that exceeds the legacy-1x estimate.
func TestMigrationNV29SolsticePrecommitDeposit(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(1000)
	)

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// preCommitDeposits decodes the miner's on-chain reserved precommit deposit (v19 miner State).
	preCommitDeposits := func() abi.TokenAmount {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		var mst stminer.State
		req.NoError(gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client)).Get(ctx, act.Head, &mst))
		return mst.PreCommitDeposits
	}

	// Cross to NV29 first, so the precommit lands under FULL-QA (10x) rules.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// PreCommit a native NV29 CC sector and pause BEFORE ProveCommit.
	sn, err := um.PreCommitSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.NoError(err)
	req.Len(sn, 1)

	// The miner has reserved a nonzero deposit for the (unproven) FULL-QA sector.
	reserved := preCommitDeposits()
	req.True(reserved.GreaterThan(big.Zero()),
		"a native NV29 precommit must reserve a nonzero PreCommitDeposits; got %s", reserved)

	// The sector is precommitted but not yet committed.
	notYet, err := client.StateSectorGetInfo(ctx, maddr, sn[0], types.EmptyTSK)
	req.NoError(err)
	req.Nil(notYet, "a pre-committed-but-unproved sector must not yet be committed")

	// Prove (activate) it on NV29 -> FULL_QA 10x.
	committed, err := um.ProvePrecommittedSectors(sealProofType, sn)
	req.NoError(err)
	req.Equal(sn, committed, "the prepared sector must be successfully proved")
	um.WaitTillActivatedAndAssertPower(committed, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)

	// Activation fully consumes the reserved deposit into the sector's locked FULL-QA pledge.
	postActivation := preCommitDeposits()
	req.True(postActivation.IsZero(),
		"activating the FULL-QA sector must fully release the precommit deposit reserve back to 0; remaining=%s", postActivation)

	info, err := client.StateSectorGetInfo(ctx, maddr, sn[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(info)
	req.NotZero(info.Flags&miner.FULL_QA_POWER, "native NV29 sector must be FULL_QA after prove")
	req.Greater(info.InitialPledge.Uint64(), uint64(0), "activated FULL-QA sector must carry a nonzero initial pledge")

	// Tie the released reserve to the FULL-QA tier: the resulting on-chain pledge must exceed the
	// legacy-1x oracle figure the pre-nv29 / deprecated derivation would have charged for the same CC.
	duration := info.Expiration - info.Activation
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	oneX, err := client.StateMinerInitialPledgeForSector(ctx, duration, defaultSectorSize, 0, head.Key())
	req.NoError(err)
	req.Greater(info.InitialPledge.Uint64(), oneX.Uint64(),
		"the pledge funded by the released deposit must exceed the legacy-1x estimate (FULL-QA tier); on-chain=%d 1x-est=%d",
		info.InitialPledge.Uint64(), oneX.Uint64())

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeSnapAndOrdering checks that USQ-then-Snap and Snap-then-USQ end with
// identical FULL_QA(10x) power, and that a second USQ on an already-FULL_QA sector is a no-op.
func TestMigrationNV29SolsticeSnapAndOrdering(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// Two pre-upgrade CC sectors, both 1x on NV28. They are the Snap targets.
	sns, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2))
	req.Len(sns, 2)
	sA, sB := sns[0], sns[1]
	um.WaitTillActivatedAndAssertPower(sns,
		uint64(defaultSectorSize)*2, uint64(defaultSectorSize)*2, // two legacy CC at 1x
	)

	// Cross the migration; both sectors remain legacy 1x (non-retroactive).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	for _, sn := range []abi.SectorNumber{sA, sB} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, head.Key())
		req.NoError(err)
		req.NotNil(info)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "precondition: legacy CC sector is still 1x before ops")
	}

	powerPre, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)

	// ---- Path A: USQ then Snap.
	// USQ lifts sA to FULL_QA (10x) while it is still a CC sector; Snap then re-proves it with a deal
	// piece. FIP-0118 keeps it FULL_QA, so Snap must NOT downgrade it.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sA}, nil)
	req.NoError(err)
	um.SnapDeal(sA, kit.SectorWithPiece(kit.BogusPieceCid2))

	// ---- Path B: Snap then USQ.
	// Snap lifts sB to FULL_QA (10x); USQ on an already-FULL_QA sector is then a no-op, not a
	// rejection and not a double charge.
	um.SnapDeal(sB, kit.SectorWithPiece(kit.BogusPieceCid2))
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sB}, nil)
	req.NoError(err)

	// Both reach the same end state: FULL_QA_POWER set => 10x.
	for _, sn := range []abi.SectorNumber{sA, sB} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "sector %d must carry FULL_QA_POWER", sn)
	}

	// Power: both legacy 1x sectors went to 10x, so the miner's QAP rose by +9x raw each, i.e. 18x.
	powerPost, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	delta := powerPost.MinerPower.QualityAdjPower.Int64() - powerPre.MinerPower.QualityAdjPower.Int64()
	req.Equal(int64(uint64(defaultSectorSize))*18, delta,
		"USQ↔Snap ordering must produce the same total QAP as Snap↔USQ (+9x per sector, no double charge)")

	// ---- Path C: a brand-new native NV29 CC sector (born FULL_QA 10x) is onboarded straight on NV29
	// (FIP-0118 gives it FULL_QA_POWER = 10x), then a deal is snapped in. Paths A/B snap a pre-upgrade
	// legacy sector relative to a USQ; this snaps a sector that is FULL_QA by origin. Because every
	// re-activated sector stays FULL_QA regardless of content, the Snap must keep it at 10x -- neither
	// downgrading it back to 1x (Snap must not strip FULL_QA) nor double-counting it to 100x.
	// sA and sB are already at 10x each (powerPost = 2 x 10x), so adding the native sector must raise
	// miner QAP by exactly its 10x (FULL_QA) contribution -- AssertPower cannot be used here because it
	// compares TOTAL miner power, so we wait for the exact QAP delta instead.
	powerBeforeNative, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	native, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(native, 1)
	sC := native[0]

	// Wait until the native sector's first WindowPoSt is committed and its FULL_QA (10x) power is
	// on-chain: miner QAP must rise by exactly defaultSectorSize*10 over the pre-onboard total.
	nativeQA := powerBeforeNative.MinerPower.QualityAdjPower.Uint64() + uint64(defaultSectorSize)*10
	solsticekit.WaitForMinerQAP(t, ctx, client, maddr, nativeQA, 2*time.Minute)

	nInfo, err := client.StateSectorGetInfo(ctx, maddr, sC, types.EmptyTSK)
	req.NoError(err)
	req.NotNil(nInfo)
	req.NotZero(nInfo.Flags&miner.FULL_QA_POWER, "precondition: native NV29 CC sector must be born FULL_QA(10x)")

	powerPreSnap, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	um.SnapDeal(sC, kit.SectorWithPiece(kit.BogusPieceCid2))

	nAfter, err := client.StateSectorGetInfo(ctx, maddr, sC, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(nAfter.Flags&miner.FULL_QA_POWER, "Snap of a native FULL_QA CC sector must keep it FULL_QA(10x)")
	powerPostSnap, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(powerPreSnap.MinerPower.QualityAdjPower.String(), powerPostSnap.MinerPower.QualityAdjPower.String(),
		"Snap of a native FULL_QA sector must not change QA power (stays 10x: no downgrade to 1x, no double-count to 100x)")

	// WindowPoSt keeps running through all three paths.
	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeExtend checks how ExtendSectorExpiration treats each legacy sector class
// across a real NV28→NV29 migration:
//
//   - a no-drop-claims extend of a pre-upgrade verified-deal sector (10x by VerifiedDealWeight) keeps
//     its verified 10x: the allocation claim is not dropped, weight is preserved/re-derived upward,
//     QA power is unchanged, and it never gains the FULL_QA_POWER flag;
//   - a pre-upgrade CC sector whose lifetime (and a pre-migration extend) spans the fork stays at 1x
//     (the migration does not retroactively promote it);
//   - a pre-upgrade unverified-deal sector (1x) extended on NV29 without dropping its deals stays at
//     1x (extend does not promote it to the FULL_QA tier).
func TestMigrationNV29SolsticeExtend(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const upgradeEpoch = abi.ChainEpoch(3000)
	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB

	// verifreg plumbing keys (see the deal recipe below).
	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	bal := types.MustParseFIL("100fil").Int64()

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{
		UpgradeEpoch:      upgradeEpoch,
		RootKey:           rootKey,
		VerifierKey:       verifierKey,
		VerifiedClientKey: verifiedClientKey,
		Bal:               bal,
	})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// ---- Datacap plumbing for the verified deal sector.
	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

	// ---- Onboard the three pre-upgrade sectors: a verified deal (10x by weight), an unverified
	// deal (1x), and a CC (1x).
	ver, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSector(
		kit.SectorWithVerifiedPiece(piece.PieceCID, &miner14.VerifiedAllocationKey{
			Client: clientId,
			ID:     verifreg14.AllocationId(allocationId),
		})),
	)
	req.Len(ver, 1)

	uv, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddSectorsWithRandomPieces(1))
	req.Len(uv, 1)

	scc, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(scc, 1)

	um.WaitTillActivatedAndAssertPower([]abi.SectorNumber{ver[0], uv[0], scc[0]},
		uint64(defaultSectorSize)*3, // raw power
		uint64(defaultSectorSize)*10+uint64(defaultSectorSize)+uint64(defaultSectorSize), // QAP: verified 10x + unverified 1x + CC 1x
	)

	for _, sn := range []abi.SectorNumber{ver[0], uv[0], scc[0]} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.Less(info.Activation, upgradeEpoch, "sector %d must activate pre-upgrade", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "pre-upgrade sector %d must not carry FULL_QA_POWER", sn)
	}

	// ---- Cross-fork Extend: while still on NV28, extend the CC sector to a new expiration that lies
	// beyond the NV29 fork, so the sector's extended lifetime spans the upgrade boundary.
	ccPre, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	ccTarget := ccPre.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	req.Greater(ccTarget, upgradeEpoch, "the CC sector's new expiration must lie beyond the NV29 fork")
	um.ExtendSectorExpiration(scc[0], ccTarget)

	ccMid, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	req.Equal(ccTarget, ccMid.Expiration, "CC sector must be extended pre-migration")
	req.Zero(ccMid.Flags&miner.FULL_QA_POWER, "extend pre-migration must not promote the CC sector")

	// ---- Cross the migration.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// The CC sector whose lifetime now spans the fork is still 1x: the migration is non-retroactive
	// even for a sector extended (pre-upgrade) to live beyond the upgrade.
	ccPost, err := client.StateSectorGetInfo(ctx, maddr, scc[0], head.Key())
	req.NoError(err)
	req.Equal(ccMid.Expiration, ccPost.Expiration, "CC expiration must be preserved across migration")
	req.Equal(ccMid.Flags, ccPost.Flags, "CC flags must be preserved across migration")
	req.Zero(ccPost.Flags&miner.FULL_QA_POWER, "cross-fork extended CC sector must stay 1x (not retroactively 10x)")

	// The verified deal sector is still 10x by weight after migration.
	verMid, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	req.Positive(verMid.VerifiedDealWeight.Int64(), "verified sector must keep its verified weight across migration")

	// ---- No-drop-claims Extend of the verified deal sector on NV29: it must keep its verified 10x
	// (weight/flags/power preserved), because we are not dropping the underlying allocation claim.
	preExt, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	prePower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)

	verTarget := preExt.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	um.ExtendSectorExpiration(ver[0], verTarget)

	postExt, err := client.StateSectorGetInfo(ctx, maddr, ver[0], types.EmptyTSK)
	req.NoError(err)
	req.Equal(verTarget, postExt.Expiration, "verified deal sector must be extended")
	// No claims dropped => the verified allocation stays committed and its VerifiedDealWeight is
	// re-derived for the longer committed duration (it never drops to zero, and extends keep the
	// claim alive). QA power is capped at qa_power_max (10x), so the miner QAP is unchanged and the
	// sector never falls to 1x nor jumps to 100x; it also never acquires the FULL_QA_POWER flag.
	req.GreaterOrEqual(postExt.VerifiedDealWeight.Int64(), preExt.VerifiedDealWeight.Int64(),
		"no-drop-claims extend must keep (re-derive upward, never drop) the verified sector's weight")
	req.Positive(postExt.VerifiedDealWeight.Int64(), "verified sector must keep a verified weight after extend")
	req.Zero(postExt.Flags&miner.FULL_QA_POWER, "legacy verified sector must stay 10x via weight, not flag")

	postPower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(prePower.MinerPower.QualityAdjPower.String(), postPower.MinerPower.QualityAdjPower.String(),
		"no-drop-claims extend of the verified deal sector must not change miner QA power (stays capped at 10x)")

	// ---- No-drop-claims Extend of the unverified deal sector (uv, native 1x) on NV29: extending a
	// legacy 1x deal sector (without dropping its deals) must not promote it to the FULL_QA tier. It
	// keeps exactly 1x — no FULL_QA_POWER flag, no verified weight, and no change to miner QA power.
	uvInfo, err := client.StateSectorGetInfo(ctx, maddr, uv[0], types.EmptyTSK)
	req.NoError(err)
	req.Zero(uvInfo.Flags&miner.FULL_QA_POWER, "precondition: unverified deal sector must be at native 1x pre-extend")
	uvPowerBefore, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	uvTarget := uvInfo.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	um.ExtendSectorExpiration(uv[0], uvTarget)

	uvPost, err := client.StateSectorGetInfo(ctx, maddr, uv[0], types.EmptyTSK)
	req.NoError(err)
	req.Equal(uvTarget, uvPost.Expiration, "unverified deal sector must be extended")
	req.Zero(uvPost.VerifiedDealWeight.Int64(), "unverified deal sector must carry no verified weight")
	req.Zero(uvPost.Flags&miner.FULL_QA_POWER,
		"extend of a 1x unverified deal sector must not promote it to FULL_QA(10x)")
	uvPowerAfter, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uvPowerBefore.MinerPower.QualityAdjPower.String(), uvPowerAfter.MinerPower.QualityAdjPower.String(),
		"extend of a 1x unverified deal sector must not change QA power (stays 1x)")

	um.AssertNoWindowPostError()
}

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

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{UpgradeEpoch: upgradeEpoch})
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

// TestMigrationNV29SolsticeDeadlineImmutabilityWindow probes TerminateSectors and
// UpgradeSectorQuality against a sector whose deadline sits in the actor's immutability window on a
// real migrated NV29 miner. Via non-persisting StateCall probes at captured historical tipsets it
// asserts Terminate is rejected with ErrIllegalArgument while the sector is in the current or next
// proving deadline but accepted once the chain advances past them, and that UpgradeSectorQuality is
// accepted even while the sector is in-window (it is not immutability-gated).
func TestMigrationNV29SolsticeDeadlineImmutabilityWindow(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// ---- Onboard a legacy 1x CC sector on NV28, then cross to NV29 (it stays 1x, non-retroactive).
	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))
	sn := legacy[0]

	lInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	loc, err := client.StateSectorPartition(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	sd, part := loc.Deadline, loc.Partition

	// ---- Params targeting our single sector; identical for every probe so the deadline state is the
	// only variable between the rejected and accepted calls.
	termEnc, err := actors.SerializeParams(&stminer.TerminateSectorsParams{
		Terminations: []stminer.TerminationDeclaration{{
			Deadline: sd, Partition: part, Sectors: bitfield.NewFromSet([]uint64{uint64(sn)}),
		}},
	})
	req.NoError(err)
	usqEnc, err := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
		Upgrades: []stminer.UpgradeSectorQuality{{
			Deadline: sd, Partition: part, Sectors: bitfield.NewFromSet([]uint64{uint64(sn)}),
		}},
	})
	req.NoError(err)

	// stateCall runs `method` with the encoded params from the owner at `tsk` (virtual, non-persisting)
	// and returns the exit code, so we can probe historical tipsets without mutating the sector.
	stateCall := func(tsk types.TipSetKey, method abi.MethodNum, params []byte) exitcode.ExitCode {
		res, cerr := client.StateCall(ctx, &types.Message{
			From: client.DefaultKey.Address, To: maddr, Method: method,
			Params: params, Value: types.FromFil(0),
		}, tsk)
		req.NoError(cerr)
		return res.MsgRct.ExitCode
	}

	// waitForCurrentDeadline polls epoch-by-epoch until the current proving deadline index equals `want`,
	// returning that tipset key. Polling every epoch catches the deadline boundary the moment it crosses,
	// so the returned tipset is unambiguously inside the target window.
	//
	// The 4-minute ceiling is a hang-guard only: each poll exits as soon as the target index is current,
	// so a healthy chain reaches it well within a minute; the bound just catches a stalled clock.
	waitForCurrentDeadline := func(want uint64) types.TipSetKey {
		end := time.Now().Add(4 * time.Minute)
		for {
			ch, cerr := client.ChainHead(ctx)
			req.NoError(cerr)
			di, derr := client.StateMinerProvingDeadline(ctx, maddr, ch.Key())
			req.NoError(derr)
			if kit.CurrentDeadlineIndex(di) == want {
				return ch.Key()
			}
			if time.Now().After(end) {
				req.FailNowf("deadline wait timeout", "current proving deadline never reached %d", want)
			}
			client.WaitTillChain(ctx, kit.HeightAtLeast(ch.Height()+1))
		}
	}

	// The 48-deadline period length, read once.
	head2, err := client.ChainHead(ctx)
	req.NoError(err)
	di0, err := client.StateMinerProvingDeadline(ctx, maddr, head2.Key())
	req.NoError(err)
	nd := di0.WPoStPeriodDeadlines

	// relative offset of the sector's deadline w.r.t. the current one: rel = (sd - current) mod nd.
	// rel 0  => sector is the CURRENT deadline; rel 1 => sector is the NEXT deadline; rel >= 2 or rel
	// wrapping negative (e.g. 47) => sector is past / well-ahead and mutable. Only rel in {0,1} reject
	// Terminate (the immutable window); every other offset accepts it.

	// ---- Position 1: current = sd-1, so the sector sits in the NEXT (immutable) proving deadline.
	nextTs := waitForCurrentDeadline((sd + nd - 1) % nd)
	req.NotEqual(types.EmptyTSK, nextTs)
	t.Logf("sector deadline %d is the NEXT proving deadline at %s", sd, nextTs)
	req.Equal(exitcode.ErrIllegalArgument, stateCall(nextTs, builtin.MethodsMiner.TerminateSectors, termEnc),
		"Terminate of a sector in the next (immutable) proving deadline must be rejected")

	// ---- Position 2: current = sd, so the sector sits in the CURRENT (immutable) proving deadline.
	// Terminate is still rejected, but UpgradeSectorQuality is ACCEPTED -- documenting that USQ has no
	// immutability-window gate (only a faulted-sector gate).
	curTs := waitForCurrentDeadline(sd)
	req.NotEqual(types.EmptyTSK, curTs)
	t.Logf("sector deadline %d is the CURRENT proving deadline at %s", sd, curTs)
	req.Equal(exitcode.ErrIllegalArgument, stateCall(curTs, builtin.MethodsMiner.TerminateSectors, termEnc),
		"Terminate of a sector in the current (immutable) proving deadline must be rejected")
	req.Equal(exitcode.Ok, stateCall(curTs, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc),
		"USQ must be accepted even while the sector sits in the current proving deadline (USQ is not immutability-gated)")

	// ---- Position 3: advance one more window so current = sd+1 and the sector is just past the current
	// deadline (rel = 47, outside {0,1}), i.e. mutable. The identical Terminate now succeeds -- proving
	// the rejections above were purely the deadline gate.
	mutTs := waitForCurrentDeadline((sd + 1) % nd)
	req.NotEqual(types.EmptyTSK, mutTs)
	t.Logf("sector deadline %d is just past the current deadline at %s", sd, mutTs)
	req.Equal(exitcode.Ok, stateCall(mutTs, builtin.MethodsMiner.TerminateSectors, termEnc),
		"Terminate must be accepted once the sector's deadline is outside the immutability window")

	// No real termination / USQ was mined (all probes were virtual StateCalls), so the sector is still
	// active and the unmanaged WindowPoSt loop ran clean throughout.
	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeMaxSectorsSplit drives the CLI's --max-sectors split path end-to-end on a
// real NV28→NV29 chain: it onboard 6 legacy CC sectors, crosses the migration, then upgrades them as
// separate UpgradeSectorQuality messages in sub-batches of maxSectors=2 (ceil(6/2)=3 messages,
// mirroring cli/miner buildUpgradeQualityParams). It asserts every message commits and no sector is
// skipped (all reach FULL_QA(10x)) or double-counted, the network QAP delta equals the miner QAP
// delta, and each split message burns positive gas well under the block gas limit.
func TestMigrationNV29SolsticeMaxSectorsSplit(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		nSectors          = 6
		maxSectors        = 2
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// Onboard nSectors legacy CC sectors on NV28; they activate at 1x each.
	legs, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(nSectors))
	req.Len(legs, nSectors)
	um.WaitTillActivatedAndAssertPower(legs, uint64(defaultSectorSize)*nSectors, uint64(defaultSectorSize)*nSectors)

	// Every sector must start as a legacy 1x CC (no FULL_QA_POWER). The batch is precommitted in one
	// ProveCommitSectors call but the actor balances the activated sectors across partitions/deadlines,
	// so they may NOT all share one (deadline, partition) -- that is fine: the split below hands each
	// contiguous maxSectors-sub-batch to um.UpgradeSectorQuality, which groups by (deadline, partition)
	// internally and emits exactly one UpgradeSectorQuality message per call regardless of how many
	// deadlines the sub-batch straddles. So the on-chain message count is exactly nSectors/maxSectors,
	// mirroring the CLI's --max-sectors split path.
	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must start without FULL_QA_POWER", sn)
	}

	// Cross the migration (non-retroactive: all sectors stay 1x).
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

	// ---- Send the split messages: ceil(nSectors/maxSectors) separate UpgradeSectorQuality
	// messages, each covering exactly maxSectors co-located sectors -- mirroring what the CLI's
	// --max-sectors splitter would push for a single (deadline, partition) group.
	messages := 0
	var gasUsed []int64
	for i := 0; i < nSectors; i += maxSectors {
		end := i + maxSectors
		if end > nSectors {
			end = nSectors
		}
		lookup, err := um.UpgradeSectorQuality(legs[i:end], nil)
		req.NoError(err, "split USQ message covering sectors %v must succeed", legs[i:end])
		gasUsed = append(gasUsed, lookup.Receipt.GasUsed)
		messages++
	}
	expectedMessages := (nSectors + maxSectors - 1) / maxSectors // ceil division
	req.Equal(expectedMessages, messages, "splitting %d sectors at maxSectors=%d in one group must emit %d messages",
		nSectors, maxSectors, expectedMessages)

	// ---- gas for a large batch: every split message must stay under the block gas limit. Capture the
	// measured per-message gas so a future actor-cost regression -- a small batch approaching the block
	// limit, or a per-sector superlinear blowup -- surfaces as a hard failure here rather than a silent
	// over-limit push on a real cluster.
	req.Len(gasUsed, messages, "one gas sample per split message")
	var totalGas int64
	for _, g := range gasUsed {
		req.Greater(g, int64(0), "a split USQ message must burn positive gas")
		req.Less(g, buildconstants.BlockGasLimit, "a split USQ message must not approach the block gas limit")
		totalGas += g
	}
	req.Less(totalGas, buildconstants.BlockGasLimit, "the whole split batch's gas must stay far under the block gas limit (headroom for a larger group)")
	t.Logf("split USQ batch: %d messages, per-message gas %v, total %d (block gas limit %d)",
		messages, gasUsed, totalGas, buildconstants.BlockGasLimit)

	// No sector skipped: every one of the 6 must now carry FULL_QA_POWER.
	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "split USQ must leave every sector %d FULL_QA (no sector skipped)", sn)
	}

	// No sector double-counted and no accounting drift: miner QAP is exactly 6x10x over the 1x
	// baseline (+9x per sector), identical to a single-batch result; network QAP matches the miner.
	afterUSQ, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*nSectors*10, afterUSQ.MinerPower.QualityAdjPower.Uint64(),
		"split USQ of all %d sectors must yield 10x each", nSectors)
	perSectorMul := uint64(defaultSectorSize) * 9
	req.Equal(perSectorMul*nSectors, afterUSQ.MinerPower.QualityAdjPower.Uint64()-beforeUSQ.MinerPower.QualityAdjPower.Uint64(),
		"miner QAP delta over %d split messages must be +9x per sector (no double-count)", messages)
	req.Equal(perSectorMul*nSectors, afterUSQ.TotalPower.QualityAdjPower.Uint64()-beforeUSQ.TotalPower.QualityAdjPower.Uint64(),
		"network QAP delta must equal the miner QAP delta across split messages")

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
	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{
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

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{
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

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{
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
	solsticekit.WaitForMinerQAP(t, ctx, client, maddr,
		uint64(defaultSectorSize)*(10+10), // ver 10x + uvA 10x remain (uvB's 1x removed)
		2*time.Minute)

	// ---- Terminate × verified deal (ver, 10x): removing it drops exactly 10x, leaving uvA's 10x.
	um.TerminateSectors([]abi.SectorNumber{ver[0]})
	solsticekit.WaitForMinerQAP(t, ctx, client, maddr,
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

	e := solsticekit.NewUpgradeEnv(t, solsticekit.Opts{UpgradeEpoch: upgradeEpoch})
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
