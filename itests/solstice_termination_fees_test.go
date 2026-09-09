package itests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMigrationNV29SolsticeUsqdTerminationFeeRealLedger verifies the termination fee of a sector that
// reached the FULL_QA(10x) tier via UpgradeSectorQuality on a legacy 1x CC sector, against the real
// FIL ledger. It asserts on the miner actor's on-chain Balance that terminating the USQ'd-to-10x sector
// debits strictly more real FIL than terminating an otherwise-identical legacy 1x sibling.
func TestMigrationNV29SolsticeUsqdTerminationFeeRealLedger(t *testing.T) {
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

	// minerBalance reads the miner actor's on-chain balance (the ledger the termination penalty debits).
	minerBalance := func() types.BigInt {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		return act.Balance
	}

	// settleAndRead advances a little past a QAP target so the deferred-termination cron has fully
	// burned the penalty, then returns the miner balance.
	settleAndRead := func(targetQA uint64) types.BigInt {
		kit.WaitForMinerQAP(ctx, t, client, maddr, targetQA, 2*time.Minute)
		head, herr := client.ChainHead(ctx)
		req.NoError(herr)
		client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+20))
		return minerBalance()
	}

	// ---- Two legacy CC sectors onboarded and activated on NV28, both 1x with no FULL_QA flag.
	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2))
	req.Len(legacy, 2)
	um.WaitTillActivatedAndAssertPower(legacy,
		uint64(defaultSectorSize)*2, uint64(defaultSectorSize)*2) // two legacy 1x CC
	usqSn, anchorSn := legacy[0], legacy[1]

	for _, sn := range legacy {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(err)
		req.Less(info.Activation, upgradeEpoch, "legacy sector %d must activate pre-upgrade (1x)", sn)
		req.Zero(info.Flags&miner.FULL_QA_POWER, "legacy sector %d must not carry FULL_QA_POWER pre-upgrade", sn)
	}

	// ---- Cross the migration (non-retroactive: both stay 1x).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// ---- USQ one legacy CC sector (usqSn) to FULL_QA(10x); the anchor stays legacy 1x. QAP = 11 units.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{usqSn}, nil)
	req.NoError(err, "USQ of a legacy CC sector must succeed")
	uInfo, err := client.StateSectorGetInfo(ctx, maddr, usqSn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(uInfo.Flags&miner.FULL_QA_POWER, "USQ'd sector must carry FULL_QA_POWER (10x)")
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*(1+10), power.MinerPower.QualityAdjPower.Uint64(),
		"USQ'd(10x) + anchor(1x) must sum to 11 units QAP")

	// ---- Terminate the untouched 1x anchor FIRST, then the USQ'd 10x sector, each as an isolated
	// Balance delta against a clean baseline.
	preAnchor := settleAndRead(uint64(defaultSectorSize) * 11)  // stable 11-unit baseline
	um.TerminateSectors([]abi.SectorNumber{anchorSn})           // anchor (1x) removed
	postAnchor := settleAndRead(uint64(defaultSectorSize) * 10) // USQ'd 10x remains
	fee1x := types.BigSub(preAnchor, postAnchor)
	req.True(fee1x.GreaterThan(types.NewInt(0)), "terminating the 1x anchor must debit some FIL; pre=%s post=%s", preAnchor, postAnchor)

	preUsq := settleAndRead(uint64(defaultSectorSize) * 10) // stable before the USQ'd-sector termination
	um.TerminateSectors([]abi.SectorNumber{usqSn})          // USQ'd 10x removed
	postUsq := settleAndRead(0)                             // all miner power gone
	feeUsqd10x := types.BigSub(preUsq, postUsq)
	t.Logf("termination fee: anchor(1x)=%s, USQ'd(10x)=%s", fee1x, feeUsqd10x)
	req.True(feeUsqd10x.GreaterThan(types.NewInt(0)), "terminating the USQ'd 10x sector must debit some FIL; pre=%s post=%s", preUsq, postUsq)
	req.True(feeUsqd10x.GreaterThan(fee1x),
		"termination penalty of a USQ'd-to-10x sector must strictly exceed that of a 1x sector; feeUsqd10x=%s fee1x=%s",
		feeUsqd10x, fee1x)

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeTerminationFeeRealLedger proves the real FIL ledger consequence of the
// FULL_QA(10x) tier on termination. It terminates, on a single unmanaged miner that earns no block
// rewards, one legacy 1x CC sector and one native NV29 10x CC sector, and asserts on the miner actor's
// on-chain Balance that the 10x sector's termination penalty strictly exceeds the 1x sector's.
func TestMigrationNV29SolsticeTerminationFeeRealLedger(t *testing.T) {
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

	// minerBalance reads the miner actor's on-chain balance (the ledger we assert the penalty against).
	minerBalance := func() types.BigInt {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		return act.Balance
	}

	// settleAndRead advances a little past a QAP target so the deferred-termination cron has fully
	// burned the penalty, then returns the miner balance.
	settleAndRead := func(targetQA uint64) types.BigInt {
		kit.WaitForMinerQAP(ctx, t, client, maddr, targetQA, 2*time.Minute)
		head, herr := client.ChainHead(ctx)
		req.NoError(herr)
		client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+20))
		return minerBalance()
	}

	// ---- Onboard a legacy CC sector on NV28; on activation it is 1x and carries no FULL_QA flag.
	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))

	lInfo, err := client.StateSectorGetInfo(ctx, maddr, legacy[0], types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER before upgrade")

	// ---- Cross the migration to NV29 (non-retroactive: the legacy sector stays 1x).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// ---- Onboard a native CC sector on NV29; on activation it is FULL_QA(10x). The helper asserts
	// the miner's *total* power, now the legacy 1x sector plus this native 10x sector (11x QA total).
	native, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(native, 1)
	um.WaitTillActivatedAndAssertPower(native, uint64(defaultSectorSize)*2, uint64(defaultSectorSize)*11)

	nInfo, err := client.StateSectorGetInfo(ctx, maddr, native[0], types.EmptyTSK)
	req.NoError(err)
	req.GreaterOrEqual(nInfo.Activation, upgradeEpoch, "native sector must activate on NV29 (10x)")
	req.NotZero(nInfo.Flags&miner.FULL_QA_POWER, "native NV29 CC sector must carry FULL_QA_POWER (10x)")

	// Sanity: both sectors are live -- QAP is 1x (legacy) + 10x (native).
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*11, power.MinerPower.QualityAdjPower.Uint64(), "1x legacy + 10x native")

	// ---- Terminate the legacy 1x sector; the deferred cron burns its penalty. Balance drop = fee1x.
	pre1 := settleAndRead(uint64(defaultSectorSize) * 11) // stable 11x baseline before termination
	um.TerminateSectors(legacy)
	post1 := settleAndRead(uint64(defaultSectorSize) * 10) // legacy removed, native 10x remains
	fee1x := types.BigSub(pre1, post1)
	req.True(fee1x.GreaterThan(types.NewInt(0)), "terminating the 1x sector must debit some FIL; pre=%s post=%s", pre1, post1)

	// ---- Terminate the native 10x sector; its penalty must exceed the 1x sector's on the real ledger.
	pre10 := settleAndRead(uint64(defaultSectorSize) * 10) // stable before the second termination
	um.TerminateSectors(native)
	post10 := settleAndRead(0) // all miner power gone
	fee10x := types.BigSub(pre10, post10)

	t.Logf("termination fee: legacy(1x)=%s, native(10x)=%s", fee1x, fee10x)
	req.True(fee10x.GreaterThan(types.NewInt(0)), "terminating the 10x sector must debit some FIL; pre=%s post=%s", pre10, post10)
	req.True(fee10x.GreaterThan(fee1x),
		"termination penalty of a FULL_QA(10x) sector must strictly exceed that of a 1x sector; fee10x=%s fee1x=%s",
		fee10x, fee1x)

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticePowerAndFees verifies read-state power/fee accounting on a miner holding a
// mix of 1x (legacy) and 10x (native and USQ'd) sectors post-upgrade: the sum of each partition's
// ActivePower().QA across all deadlines/partitions equals the miner's total QAP from StateMinerPower
// (and each partition's QA equals the sum of its member sectors' own tiers), and USQ re-derives a
// USQ'd sector's per-sector daily proof fee (SectorOnChainInfo.DailyFee) from the 1x rate to the 10x
// rate.
func TestMigrationNV29SolsticePowerAndFees(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	// The daily proof fee is proportional to circulating supply, which is ~0 in a fresh itest
	// ensemble (the genesis reserve actor still holds all of the initial FilReserved). Bump the
	// NV25+ reserve constant to 1B FIL exactly as daily_fees_test.go does so circulating supply is
	// ~700M and sector DailyFees are non-zero and scale with QA power (otherwise a FULL_QA 10x CC
	// sector and a legacy 1x CC sector would both read a DailyFee of 0 and the @10x comparison would
	// be vacuous).
	originalUpgradeTeepInitialFilReserved := buildconstants.UpgradeTeepInitialFilReserved
	buildconstants.UpgradeTeepInitialFilReserved = types.MustParseFIL("1000000000 FIL").Int
	t.Cleanup(func() {
		buildconstants.UpgradeTeepInitialFilReserved = originalUpgradeTeepInitialFilReserved
	})

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof
	defer um.Stop()

	// ---- Two legacy CC sectors (1x) pre-upgrade.
	legs, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(2))
	req.Len(legs, 2)
	um.WaitTillActivatedAndAssertPower(legs,
		uint64(defaultSectorSize)*2, uint64(defaultSectorSize)*2) // two 1x legacy CC

	// ---- Cross the migration.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// ---- One native NV29 CC sector (10x). Totals are cumulative over the miner: legs (2x 1x) + this.
	native, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(native, 1)
	um.WaitTillActivatedAndAssertPower(native,
		uint64(defaultSectorSize)*3,        // raw 3 sectors
		uint64(defaultSectorSize)*(1+1+10)) // QAP: legs 1x+1x + native 10x

	// dailyFee reads a sector's per-sector daily proof fee (SectorOnChainInfo.DailyFee, set at
	// activation and re-derived whenever the sector's QAP changes, e.g. on USQ).
	dailyFee := func(sn abi.SectorNumber) abi.TokenAmount {
		info, ierr := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(ierr)
		req.NotNil(info)
		return info.DailyFee
	}

	// ---- daily-fee re-derivation on USQ: capture legs[0]'s daily fee while it is still a legacy 1x
	// sector (activated on NV28), alongside its untouched 1x sibling legs[1] and the native NV29 10x
	// sector. legs[0]==legs[1] (same 1x tier) and both are below the native 10x fee.
	leg0Fee1x := dailyFee(legs[0])
	leg1Fee1x := dailyFee(legs[1])
	nativeFee := dailyFee(native[0])
	req.Equal(leg0Fee1x.String(), leg1Fee1x.String(),
		"the two same-batch legacy 1x sectors must carry the same daily fee (1x tier)")
	req.True(nativeFee.GreaterThan(leg0Fee1x), "a native FULL_QA(10x) sector's daily fee must exceed a legacy 1x sector's")
	t.Logf("daily fees before USQ: legs0(1x)=%s legs1(1x)=%s native(10x)=%s", leg0Fee1x, leg1Fee1x, nativeFee)

	// ---- USQ one legacy 1x CC sector to 10x, leaving the other legacy at 1x.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{legs[0]}, nil)
	req.NoError(err, "USQ of a legacy CC sector must succeed")

	// USQ raises the sector's QAP, so FIP-0118 re-derives its DailyFee to the 10x rate: legs[0] must now
	// charge strictly more than it did at 1x and more than its untouched 1x sibling legs[1], landing in
	// the FULL_QA(10x) fee band (native, 10x). This is the distinct USQ'd-sector semantic a native 10x
	// sector (born at 10x) cannot exercise.
	leg0Fee10x := dailyFee(legs[0])
	req.True(leg0Fee10x.GreaterThan(leg0Fee1x),
		"USQ must re-derive a legacy sector's daily fee from the 1x rate to a higher (10x) rate; before=%s after=%s",
		leg0Fee1x, leg0Fee10x)
	req.True(leg0Fee10x.GreaterThan(leg1Fee1x),
		"a USQ'd-to-10x sector's daily fee must exceed its untouched 1x sibling's (left the 1x band); usqd=%s sibling1x=%s",
		leg0Fee10x, leg1Fee1x)
	t.Logf("daily fee after USQ: legs0(USQ'd 10x)=%s legs1(still 1x)=%s native(10x)=%s", leg0Fee10x, leg1Fee1x, nativeFee)

	// Mixed end state: legs[0]=10x (USQ'd), legs[1]=1x (legacy), native=10x.
	total := uint64(defaultSectorSize) * (10 + 1 + 10)
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(total, power.MinerPower.QualityAdjPower.Uint64(), "mixed 1x+10x QAP must be as expected")

	// ---- partition/deadline power totals: sum ActivePower().QA across all partitions == miner QAP.
	blk := blockstore.NewAPIBlockstore(client)
	stor := gstStore.WrapBlockStore(ctx, blk)

	act, err := client.StateGetActor(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	var mst stminer.State
	req.NoError(stor.Get(ctx, act.Head, &mst))

	dls, err := mst.LoadDeadlines(stor)
	req.NoError(err)

	partitionQA := big.Zero()
	err = dls.ForEach(stor, func(dlIdx uint64, dl *stminer.Deadline) error {
		ps, err := dl.PartitionsArray(stor)
		if err != nil {
			return err
		}
		var part stminer.Partition
		return ps.ForEach(&part, func(partIdx int64) error {
			partitionQA = big.Add(partitionQA, part.ActivePower().QA)
			return nil
		})
	})
	req.NoError(err)

	req.Equal(power.MinerPower.QualityAdjPower.String(), partitionQA.String(),
		"sum of partition ActivePower().QA must equal the miner's total QAP (partition-level FULL_QA accounting)")

	// ---- Partition-subset accounting: the actor balances sectors across deadlines/partitions, so
	// whether the upgraded legs[0] (10x) and its untouched sibling legs[1] (1x) share a partition is not
	// fixed. Regardless of layout, each partition's QA must equal the sum of its members' OWN tiers --
	// a mixed 10x+1x partition reads 11 units, a pure 10x partition 10, a pure 1x partition 1 -- never a
	// whole-partition multiplier. Recompute each partition's QA from its member sectors' FULL_QA flags
	// and require it equals that partition's stored ActivePower().QA.
	err = dls.ForEach(stor, func(dlIdx uint64, dl *stminer.Deadline) error {
		ps, err := dl.PartitionsArray(stor)
		if err != nil {
			return err
		}
		var part stminer.Partition
		return ps.ForEach(&part, func(partIdx int64) error {
			sns, err := part.Sectors.All(1 << 20)
			if err != nil {
				return err
			}
			// Recompute this partition's QA from each live sector's own tier: a FULL_QA_POWER sector
			// contributes 10x, a legacy 1x sector contributes 1x (all of these are CC full-size sectors,
			// so the per-sector QA is exactly a multiple of defaultSectorSize).
			var perPart uint64
			for _, sn := range sns {
				info, ierr := client.StateSectorGetInfo(ctx, maddr, abi.SectorNumber(sn), types.EmptyTSK)
				if ierr != nil {
					return ierr
				}
				req.NotNil(info, "partition %d sector %d must exist (no termination in this test)", partIdx, sn)
				if info.Flags&miner.FULL_QA_POWER != 0 {
					perPart += uint64(defaultSectorSize) * 10
				} else {
					perPart += uint64(defaultSectorSize)
				}
			}
			req.Equal(perPart, part.ActivePower().QA.Uint64(),
				"partition (dl %d, part %d) ActivePower().QA must equal the sum of its members' own tiers (mixed 10x+1x subsets are not whole-partition-multiplied); stored=%d recomputed=%d",
				dlIdx, partIdx, part.ActivePower().QA.Uint64(), perPart)
			return nil
		})
	})
	req.NoError(err, "iterating deadlines/partitions to recompute partition QA must succeed")

	um.AssertNoWindowPostError()
}
