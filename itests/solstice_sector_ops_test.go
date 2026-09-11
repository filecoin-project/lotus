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
	"github.com/filecoin-project/lotus/lib/must"
)

// TestMigrationNV29SolsticePreCommitProve proves that a CC sector pre-committed on NV28 but activated on NV29 lands at FULL_QA(10x).
func TestMigrationNV29SolsticePreCommitProve(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const upgradeEpoch = abi.ChainEpoch(3000)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB

	sn, err := um.PreCommitSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.NoError(err)
	req.Len(sn, 1)

	preHead, err := client.ChainHead(ctx)
	req.NoError(err)
	req.Less(preHead.Height(), upgradeEpoch, "PreCommit must be submitted before the NV29 fork")
	preNv, err := client.StateNetworkVersion(ctx, preHead.Key())
	req.NoError(err)
	req.Equal(network.Version28, preNv, "PreCommit must be submitted on NV28")

	notYet, err := client.StateSectorGetInfo(ctx, maddr, sn[0], types.EmptyTSK)
	req.NoError(err)
	req.Nil(notYet, "a pre-committed-but-unproved sector must not yet be committed")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	committed, err := um.ProvePrecommittedSectors(sealProofType, sn)
	req.NoError(err)
	req.Equal(sn, committed, "the prepared sector must be successfully proved")

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

// TestMigrationNV29SolsticePrecommitDeposit asserts PreCommitDeposits is reserved while NV29 sector is unproven, then fully released on activation; resulting pledge exceeds 1x estimate.
func TestMigrationNV29SolsticePrecommitDeposit(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(1000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// preCommitDeposits reads the miner's reserved precommit deposit from v19 miner state.
	preCommitDeposits := func() abi.TokenAmount {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		var mst stminer.State
		req.NoError(gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client)).Get(ctx, act.Head, &mst))
		return mst.PreCommitDeposits
	}

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	sn, err := um.PreCommitSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.NoError(err)
	req.Len(sn, 1)

	reserved := preCommitDeposits()
	req.True(reserved.GreaterThan(big.Zero()),
		"a native NV29 precommit must reserve a nonzero PreCommitDeposits; got %s", reserved)

	notYet, err := client.StateSectorGetInfo(ctx, maddr, sn[0], types.EmptyTSK)
	req.NoError(err)
	req.Nil(notYet, "a pre-committed-but-unproved sector must not yet be committed")

	committed, err := um.ProvePrecommittedSectors(sealProofType, sn)
	req.NoError(err)
	req.Equal(sn, committed, "the prepared sector must be successfully proved")
	um.WaitTillActivatedAndAssertPower(committed, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)

	postActivation := preCommitDeposits()
	req.True(postActivation.IsZero(),
		"activating the FULL-QA sector must fully release the precommit deposit reserve back to 0; remaining=%s", postActivation)

	info, err := client.StateSectorGetInfo(ctx, maddr, sn[0], types.EmptyTSK)
	req.NoError(err)
	req.NotNil(info)
	req.NotZero(info.Flags&miner.FULL_QA_POWER, "native NV29 sector must be FULL_QA after prove")
	req.Greater(info.InitialPledge.Uint64(), uint64(0), "activated FULL-QA sector must carry a nonzero initial pledge")

	duration := info.Expiration - info.PowerBaseEpoch
	head, err = client.ChainHead(ctx)
	req.NoError(err)
	oneX, err := client.StateMinerInitialPledgeForSector(ctx, duration, defaultSectorSize, 0, head.Key())
	req.NoError(err)
	req.Greater(info.InitialPledge.Uint64(), oneX.Uint64(),
		"the pledge funded by the released deposit must exceed the legacy-1x estimate (FULL-QA tier); on-chain=%d 1x-est=%d",
		info.InitialPledge.Uint64(), oneX.Uint64())

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeSnapAndOrdering checks USQ→Snap and Snap→USQ both yield FULL_QA(10x), and a second USQ on an already-FULL_QA sector is a no-op.
func TestMigrationNV29SolsticeSnapAndOrdering(t *testing.T) {
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

	sns, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2))
	req.Len(sns, 2)
	sA, sB := sns[0], sns[1]
	um.WaitTillActivatedAndAssertPower(sns,
		uint64(defaultSectorSize)*2, uint64(defaultSectorSize)*2, // two legacy CC at 1x
	)

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

	// Path A: USQ then Snap — sA goes to FULL_QA(10x), Snap must not downgrade it.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sA}, nil)
	req.NoError(err)
	um.SnapDeal(sA, kit.SectorWithPiece(kit.BogusPieceCid2))

	// Path B: Snap then USQ — sB goes to FULL_QA(10x) via Snap; USQ is then a no-op.
	um.SnapDeal(sB, kit.SectorWithPiece(kit.BogusPieceCid2))
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sB}, nil)
	req.NoError(err)

	for _, sn := range []abi.SectorNumber{sA, sB} {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotNil(info)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "sector %d must carry FULL_QA_POWER", sn)
	}

	powerPost, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	delta := powerPost.MinerPower.QualityAdjPower.Int64() - powerPre.MinerPower.QualityAdjPower.Int64()
	req.Equal(int64(uint64(defaultSectorSize))*18, delta,
		"USQ↔Snap ordering must produce the same total QAP as Snap↔USQ (+9x per sector, no double charge)")

	// Path C: native NV29 CC sector (FULL_QA by origin) with a Snap must stay 10x.
	powerBeforeNative, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)

	native, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(native, 1)
	sC := native[0]

	nativeQA := powerBeforeNative.MinerPower.QualityAdjPower.Uint64() + uint64(defaultSectorSize)*10
	kit.WaitForMinerQAP(ctx, t, client, maddr, nativeQA, 2*time.Minute)

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

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeExtend checks ExtendSectorExpiration: verified 10x stays 10x by weight (no FULL_QA flag), CC/unverified-deal extended on NV29 stay at 1x.
func TestMigrationNV29SolsticeExtend(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const upgradeEpoch = abi.ChainEpoch(3000)
	const defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB

	rootKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifierKey := must.One(key.GenerateKey(types.KTSecp256k1))
	verifiedClientKey := must.One(key.GenerateKey(types.KTBLS))
	bal := types.MustParseFIL("100fil").Int64()

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{
		UpgradeEpoch:      upgradeEpoch,
		RootKey:           rootKey,
		VerifierKey:       verifierKey,
		VerifiedClientKey: verifiedClientKey,
		Bal:               bal,
	})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	_, vclients := kit.SetupVerifiedClients(ctx, t, client, rootKey, verifierKey, []*key.Key{verifiedClientKey})
	verifiedClientAddr := vclients[0]

	minerId := must.One(address.IDFromAddress(maddr))
	piece := abi.PieceInfo{Size: abi.PaddedPieceSize(defaultSectorSize), PieceCID: kit.BogusPieceCid2}
	clientId, allocationId := kit.SetupAllocation(ctx, t, client, minerId, piece, verifiedClientAddr, 0, 0)

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

	ccPre, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	ccTarget := ccPre.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	req.Greater(ccTarget, upgradeEpoch, "the CC sector's new expiration must lie beyond the NV29 fork")
	um.ExtendSectorExpiration(scc[0], ccTarget)

	ccMid, err := client.StateSectorGetInfo(ctx, maddr, scc[0], types.EmptyTSK)
	req.NoError(err)
	req.Equal(ccTarget, ccMid.Expiration, "CC sector must be extended pre-migration")
	req.Zero(ccMid.Flags&miner.FULL_QA_POWER, "extend pre-migration must not promote the CC sector")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	ccPost, err := client.StateSectorGetInfo(ctx, maddr, scc[0], head.Key())
	req.NoError(err)
	req.Equal(ccMid.Expiration, ccPost.Expiration, "CC expiration must be preserved across migration")
	req.Equal(ccMid.Flags, ccPost.Flags, "CC flags must be preserved across migration")
	req.Zero(ccPost.Flags&miner.FULL_QA_POWER, "cross-fork extended CC sector must stay 1x (not retroactively 10x)")

	verMid, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	req.Positive(verMid.VerifiedDealWeight.Int64(), "verified sector must keep its verified weight across migration")

	preExt, err := client.StateSectorGetInfo(ctx, maddr, ver[0], head.Key())
	req.NoError(err)
	prePower, err := client.StateMinerPower(ctx, maddr, head.Key())
	req.NoError(err)

	verTarget := preExt.Expiration + abi.ChainEpoch(builtin.EpochsInDay)
	um.ExtendSectorExpiration(ver[0], verTarget)

	postExt, err := client.StateSectorGetInfo(ctx, maddr, ver[0], types.EmptyTSK)
	req.NoError(err)
	req.Equal(verTarget, postExt.Expiration, "verified deal sector must be extended")
	req.GreaterOrEqual(postExt.VerifiedDealWeight.Int64(), preExt.VerifiedDealWeight.Int64(),
		"no-drop-claims extend must keep (re-derive upward, never drop) the verified sector's weight")
	req.Positive(postExt.VerifiedDealWeight.Int64(), "verified sector must keep a verified weight after extend")
	req.Zero(postExt.Flags&miner.FULL_QA_POWER, "legacy verified sector must stay 10x via weight, not flag")

	postPower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(prePower.MinerPower.QualityAdjPower.String(), postPower.MinerPower.QualityAdjPower.String(),
		"no-drop-claims extend of the verified deal sector must not change miner QA power (stays capped at 10x)")

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

// TestMigrationNV29SolsticeDeadlineImmutabilityWindow probes: Terminate rejected in current/next proving deadline, accepted once past; USQ accepted even in-window (not immutability-gated).
func TestMigrationNV29SolsticeDeadlineImmutabilityWindow(t *testing.T) {
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

	// stateCall probes method via StateCall; returns exit code without mutating state.
	stateCall := func(tsk types.TipSetKey, method abi.MethodNum, params []byte) exitcode.ExitCode {
		res, cerr := client.StateCall(ctx, &types.Message{
			From: client.DefaultKey.Address, To: maddr, Method: method,
			Params: params, Value: types.FromFil(0),
		}, tsk)
		req.NoError(cerr)
		return res.MsgRct.ExitCode
	}

	// waitForCurrentDeadline polls until the current proving deadline index equals want.
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

	head2, err := client.ChainHead(ctx)
	req.NoError(err)
	di0, err := client.StateMinerProvingDeadline(ctx, maddr, head2.Key())
	req.NoError(err)
	nd := di0.WPoStPeriodDeadlines

	// Position 1: sector in the NEXT (immutable) proving deadline.
	nextTs := waitForCurrentDeadline((sd + nd - 1) % nd)
	req.NotEqual(types.EmptyTSK, nextTs)
	t.Logf("sector deadline %d is the NEXT proving deadline at %s", sd, nextTs)
	req.Equal(exitcode.ErrIllegalArgument, stateCall(nextTs, builtin.MethodsMiner.TerminateSectors, termEnc),
		"Terminate of a sector in the next (immutable) proving deadline must be rejected")
	req.Equal(exitcode.Ok, stateCall(nextTs, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc),
		"USQ must be accepted even in the next proving deadline (USQ is not immutability-gated)")

	// Position 2: sector in the CURRENT (immutable) proving deadline.
	curTs := waitForCurrentDeadline(sd)
	req.NotEqual(types.EmptyTSK, curTs)
	t.Logf("sector deadline %d is the CURRENT proving deadline at %s", sd, curTs)
	req.Equal(exitcode.ErrIllegalArgument, stateCall(curTs, builtin.MethodsMiner.TerminateSectors, termEnc),
		"Terminate of a sector in the current (immutable) proving deadline must be rejected")
	req.Equal(exitcode.Ok, stateCall(curTs, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc),
		"USQ must be accepted even while the sector sits in the current proving deadline (USQ is not immutability-gated)")

	// Position 3: sector past the current deadline — mutable; Terminate now succeeds.
	mutTs := waitForCurrentDeadline((sd + 1) % nd)
	req.NotEqual(types.EmptyTSK, mutTs)
	t.Logf("sector deadline %d is just past the current deadline at %s", sd, mutTs)
	req.Equal(exitcode.Ok, stateCall(mutTs, builtin.MethodsMiner.TerminateSectors, termEnc),
		"Terminate must be accepted once the sector's deadline is outside the immutability window")
	req.Equal(exitcode.Ok, stateCall(mutTs, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc),
		"USQ must be accepted once the sector's deadline is outside the immutability window")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeMaxSectorsSplit drives the --max-sectors split path: 6 legacy sectors upgraded in batches of 2 (3 messages), all reach FULL_QA(10x), gas stays under block limit.
func TestMigrationNV29SolsticeMaxSectorsSplit(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		nSectors          = 6
		maxSectors        = 2
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	legs, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(nSectors))
	req.Len(legs, nSectors)
	um.WaitTillActivatedAndAssertPower(legs, uint64(defaultSectorSize)*nSectors, uint64(defaultSectorSize)*nSectors)

	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must start without FULL_QA_POWER", sn)
	}

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

	for _, sn := range legs {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "split USQ must leave every sector %d FULL_QA (no sector skipped)", sn)
	}

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
