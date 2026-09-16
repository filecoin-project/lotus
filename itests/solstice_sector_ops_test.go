package itests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
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

// TestMigrationNV29SolsticePrecommitDeposit checks native NV29 precommit reservation,
// deposit release on activation, and the resulting FULL_QA (10x) pledge tier.
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
	full, err := client.StateMinerInitialPledgeForSector(ctx, duration, defaultSectorSize, uint64(defaultSectorSize), head.Key())
	req.NoError(err)
	req.Greater(full.Uint64(), oneX.Uint64(),
		"FULL-QA pledge estimate must exceed the legacy-1x estimate for the same CC sector")
	req.Greater(info.InitialPledge.Uint64(), oneX.Uint64(),
		"the pledge funded by the released deposit must exceed the legacy-1x estimate (FULL-QA tier); on-chain=%d 1x-est=%d",
		info.InitialPledge.Uint64(), oneX.Uint64())
	// The estimate includes a safety margin and uses live reward state, so allow drift
	// while requiring the on-chain pledge to remain in the FULL-QA tier.
	req.GreaterOrEqual(info.InitialPledge.Uint64(), full.Uint64()/2,
		"chain must charge the FULL-QA pledge tier (on-chain %d, 1x-est %d, 10x-est %d)",
		info.InitialPledge.Uint64(), oneX.Uint64(), full.Uint64())

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
