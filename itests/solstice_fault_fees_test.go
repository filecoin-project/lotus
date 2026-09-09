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
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMigrationNV29SolsticeUsqdSectorFault faults a sector that reached the FULL_QA(10x) tier via
// UpgradeSectorQuality: a legacy 1x CC sector USQ'd to FULL_QA(10x) on NV29 faults like a native 10x
// sector (the miner's QAP drops to zero, not to a 1x residue), USQ is rejected on the faulted sector,
// and a recovery declaration is recorded.
func TestMigrationNV29SolsticeUsqdSectorFault(t *testing.T) {
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

	// ---- A legacy CC sector onboarded and activated on NV28 is 1x with no FULL_QA flag.
	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))

	sn := legacy[0]
	lInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER pre-upgrade")

	// ---- Cross the migration (non-retroactive: the legacy sector stays 1x).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// ---- USQ the sole legacy CC sector to FULL_QA(10x): it becomes the miner's only (10x) power.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.NoError(err, "USQ of a legacy CC sector must succeed")
	uInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(uInfo.Flags&miner.FULL_QA_POWER, "USQ'd sector must carry FULL_QA_POWER (10x)")
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*10, power.MinerPower.QualityAdjPower.Uint64(),
		"USQ'd-to-10x sector must be the miner's sole 10x power")

	// ---- Declare a fault on the USQ'd 10x sector and let one proving period elapse so it takes effect.
	um.DeclareFaults([]abi.SectorNumber{sn})

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(di.Open+di.WPoStProvingPeriod+1))

	faults, err := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isFaulted, err := faults.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isFaulted, "USQ'd sector %d must be faulted after a proving period", sn)

	// The FULL_QA(10x) tier USQ granted is fully removed by the fault: the miner drops to zero QAP,
	// not to a 1x residue (the USQ'd tier is real and faults like a native 10x tier).
	fpower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.True(fpower.MinerPower.QualityAdjPower.IsZero(),
		"faulting the sole USQ'd-to-10x sector must remove all QAP (full 10x tier, not a 1x residue); got %s",
		fpower.MinerPower.QualityAdjPower)

	// UpgradeSectorQuality is rejected on the now faulted (inactive) USQ'd sector.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.Error(err, "USQ on a faulted USQ'd sector must be rejected")
	req.Contains(err.Error(), "not active", "USQ on a faulted USQ'd sector must fail with 'sector is not active'")

	// A recovery declaration on the USQ'd sector is accepted and recorded.
	um.RecoverFaults([]abi.SectorNumber{sn})

	recs, err := client.StateMinerRecoveries(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isRecovering, err := recs.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isRecovering, "a DeclareFaultsRecovered on the USQ'd sector must be accepted and recorded")

	// The unmanaged posting loop stayed clean through the USQ, fault, and recovery-declaration phases.
	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeFaultFee1xVs10x proves on the real ledger that the continued-fault penalty
// of the FULL_QA(10x) tier is QAP-proportional. It shows a legacy 1x sector faulted on a drained miner
// also accrues outstanding FeeDebt, and that an otherwise-identical native 10x sector's continued-fault
// penalty strictly exceeds the 1x sector's. It uses two unmanaged miners (no block rewards, gas paid
// from the shared owner wallet) on the same network/timeline, each holding one sector of a different
// tier.
func TestMigrationNV29SolsticeFaultFee1xVs10x(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
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

	// Two unmanaged miners on one network/timeline, each destined to hold exactly one CC sector of a
	// different QA tier: legacy 1x (um1x, onboarded on NV28) and native 10x (um10x, onboarded on NV29).
	um1x, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(defaultSectorSize),
		kit.OwnerAddr(client.DefaultKey),
	)
	defer um1x.Stop()
	um10x, ens := ens.UnmanagedMiner(ctx, client,
		kit.SectorSize(defaultSectorSize),
		kit.OwnerAddr(client.DefaultKey),
	)
	defer um10x.Stop()

	blockMiners := ens.InterconnectAll().BeginMining(5 * time.Millisecond)
	ens.Start()
	blockMiners[0].WatchMinerForPost(um1x.ActorAddr)
	blockMiners[0].WatchMinerForPost(um10x.ActorAddr)

	// ledger decodes the v19 miner state for a given miner and reports (balance, available, feeDebt).
	ledger := func(maddr address.Address, actType string) (balance, available, feeDebt abi.TokenAmount) {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		var mst stminer.State
		req.NoError(gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client)).Get(ctx, act.Head, &mst))
		avail := big.Subtract(act.Balance, mst.LockedFunds, mst.PreCommitDeposits, mst.InitialPledge, mst.FeeDebt)
		if avail.LessThan(big.Zero()) {
			avail = big.Zero()
		}
		return act.Balance, avail, mst.FeeDebt
	}

	// drainWithdrawBalance pushes a WithdrawBalance (owner method) from the shared owner key against
	// maddr and asserts the miner's available balance drops to ~0.
	drainWithdrawBalance := func(maddr address.Address, actType string) {
		params, perr := actors.SerializeParams(&stminer.WithdrawBalanceParams{AmountRequested: types.FromFil(1000)})
		req.NoError(perr)
		msg, merr := client.MpoolPushMessage(ctx, &types.Message{
			From:   client.DefaultKey.Address,
			To:     maddr,
			Value:  big.Zero(),
			Method: builtin.MethodsMiner.WithdrawBalance,
			Params: params,
		}, nil)
		req.NoError(merr)
		lookup, werr := client.StateWaitMsg(ctx, msg.Cid(), 2, -1, true)
		req.NoError(werr)
		req.True(lookup.Receipt.ExitCode.IsSuccess(), "WithdrawBalance on %s must succeed", actType)
		_, avail, _ := ledger(maddr, actType)
		req.True(avail.LessThan(types.NewInt(1e6)),
			"%s available must be drained to ~0 after WithdrawBalance; got %s", actType, avail)
	}

	// ---- Onboard um1x's legacy 1x CC sector on NV28 (before the fork). On activation it is 1x and
	// carries no FULL_QA flag.
	legacy, _ := um1x.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um1x.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))
	lInfo, err := client.StateSectorGetInfo(ctx, um1x.ActorAddr, legacy[0], types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER before upgrade")

	// ---- Cross the migration to NV29 (non-retroactive: um1x's legacy sector stays 1x).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// ---- Onboard um10x's native CC sector on NV29; on activation it is FULL_QA(10x).
	native, _ := um10x.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(native, 1)
	um10x.WaitTillActivatedAndAssertPower(native, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)
	nInfo, err := client.StateSectorGetInfo(ctx, um10x.ActorAddr, native[0], types.EmptyTSK)
	req.NoError(err)
	req.GreaterOrEqual(nInfo.Activation, upgradeEpoch, "native sector must activate on NV29 (10x)")
	req.NotZero(nInfo.Flags&miner.FULL_QA_POWER, "native NV29 CC sector must carry FULL_QA_POWER (10x)")

	// Sanity: each miner starts with a positive available balance (from precommit funding), no debt.
	for _, m := range []struct {
		actor string
		maddr address.Address
	}{
		{"legacy-1x", um1x.ActorAddr},
		{"native-10x", um10x.ActorAddr},
	} {
		_, avail, debt := ledger(m.maddr, m.actor)
		req.True(debt.IsZero(), "%s must start with no fee debt", m.actor)
		req.True(avail.GreaterThan(big.Zero()), "%s must start with positive available balance; got %s", m.actor, avail)
	}

	// ---- Drain both miners' available balances to ~0 so their fault penalties cannot be repaid and
	// instead accumulate as FeeDebt (the locked initial pledge cannot pay penalties).
	drainWithdrawBalance(um1x.ActorAddr, "legacy-1x")
	drainWithdrawBalance(um10x.ActorAddr, "native-10x")

	// ---- Declare a fault on each miner's sole sector.
	um1x.DeclareFaults([]abi.SectorNumber{legacy[0]})
	um10x.DeclareFaults([]abi.SectorNumber{native[0]})

	// ---- Poll until BOTH miners have accrued FeeDebt. Then the 10x continued-fault penalty must
	// strictly exceed the 1x one on the real ledger.
	end := time.Now().Add(4 * time.Minute)
	var debt1x, debt10x abi.TokenAmount
	for {
		_, _, d1 := ledger(um1x.ActorAddr, "legacy-1x")
		_, _, d10 := ledger(um10x.ActorAddr, "native-10x")
		if d1.GreaterThan(big.Zero()) && d10.GreaterThan(big.Zero()) {
			debt1x, debt10x = d1, d10
			break
		}
		if time.Now().After(end) {
			require.FailNowf(t, "FeeDebt accrual timeout",
				"both miners must accrue FeeDebt; legacy-1x=%s native-10x=%s", d1, d10)
		}
		h, herr := client.ChainHead(ctx)
		req.NoError(herr)
		client.WaitTillChain(ctx, kit.HeightAtLeast(h.Height()+40))
	}

	t.Logf("continued-fault FeeDebt accrued: legacy(1x)=%s, native(10x)=%s", debt1x, debt10x)
	req.True(debt1x.GreaterThan(big.Zero()), "a legacy 1x fault on a drained miner must accrue FeeDebt")
	req.True(debt10x.GreaterThan(big.Zero()), "a native 10x fault on a drained miner must accrue FeeDebt")
	req.True(debt10x.GreaterThan(debt1x),
		"continued-fault penalty of a FULL_QA(10x) sector must strictly exceed a legacy 1x sector's; fee10x=%s fee1x=%s",
		debt10x, debt1x)

	for _, m := range []struct {
		actor string
		maddr address.Address
		sn    abi.SectorNumber
	}{
		{"legacy-1x", um1x.ActorAddr, legacy[0]},
		{"native-10x", um10x.ActorAddr, native[0]},
	} {
		faults, ferr := client.StateMinerFaults(ctx, m.maddr, types.EmptyTSK)
		req.NoError(ferr)
		isFaulted, ierr := faults.IsSet(uint64(m.sn))
		req.NoError(ierr)
		req.True(isFaulted, "%s sector must be faulted while it accrues FeeDebt", m.actor)
		stillInfo, serr := client.StateSectorGetInfo(ctx, m.maddr, m.sn, types.EmptyTSK)
		req.NoError(serr)
		req.NotNil(stillInfo, "%s sector must still exist (miner not terminated for debt)", m.actor)
	}

	um1x.AssertNoWindowPostError()
	um10x.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeFaultFeeDebt exercises the full debt path of a continued-fault fee on the
// real ledger for a native FULL_QA(10x) sector, on a single unmanaged miner that earns no block
// rewards: it drains the available balance, faults the 10x sector so the proving-period cron parks the
// penalty as FeeDebt, tops the miner back up, and asserts RepayDebt clears the debt back to 0 (and
// that it stays 0 across another proving period, the top-up covering the continuing fee).
func TestMigrationNV29SolsticeFaultFeeDebt(t *testing.T) {
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

	// balanceOnly reads just the actor balance (handy for log messages).
	balanceOnly := func() string {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		return act.Balance.String()
	}

	// ledger decodes the v19 miner state and reports (balance, available, feeDebt).
	ledger := func() (balance, available, feeDebt abi.TokenAmount) {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		var mst stminer.State
		req.NoError(gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client)).Get(ctx, act.Head, &mst))
		avail := big.Subtract(act.Balance, mst.LockedFunds, mst.PreCommitDeposits, mst.InitialPledge, mst.FeeDebt)
		if avail.LessThan(big.Zero()) {
			avail = big.Zero()
		}
		return act.Balance, avail, mst.FeeDebt
	}

	// sendFromOwner pushes a message from the owner (== worker) key and asserts it lands successfully.
	sendFromOwner := func(value abi.TokenAmount, method abi.MethodNum, params []byte) {
		msg, merr := client.MpoolPushMessage(ctx, &types.Message{
			From:   client.DefaultKey.Address,
			To:     maddr,
			Value:  value,
			Method: method,
			Params: params,
		}, nil)
		req.NoError(merr)
		lookup, werr := client.StateWaitMsg(ctx, msg.Cid(), 2, lapi.LookbackNoLimit, true)
		req.NoError(werr)
		req.True(lookup.Receipt.ExitCode.IsSuccess(),
			"message (method %d) must succeed; exit=%d", method, lookup.Receipt.ExitCode)
	}

	// Cross the migration to NV29, then onboard a native NV29 10x CC sector (the miner's only power).
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

	info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(info.Flags&miner.FULL_QA_POWER, "native NV29 CC sector must carry FULL_QA_POWER (10x)")

	// Sanity: before draining, the miner holds a meaningful available balance (from the precommit funding).
	_, avail0, debt0 := ledger()
	req.True(debt0.IsZero(), "no fee debt before the fault; got %s", debt0)
	req.True(avail0.GreaterThan(big.Zero()), "miner must start with a positive available balance; got %s", avail0)

	// ---- Drain the available balance to ~0 via WithdrawBalance (owner method).
	withdrawParams, aerr := actors.SerializeParams(&stminer.WithdrawBalanceParams{AmountRequested: types.FromFil(1000)})
	req.NoError(aerr)
	sendFromOwner(big.Zero(), builtin.MethodsMiner.WithdrawBalance, withdrawParams)
	_, avail1, debt1 := ledger()
	req.True(debt1.IsZero(), "draining must not create fee debt; got %s", debt1)
	req.True(avail1.LessThan(types.NewInt(1e6)),
		"available balance must be drained to ~0 after WithdrawBalance; got %s (balance %s)", avail1, balanceOnly())

	// ---- Declare the 10x sector faulty. The proving-period cron then charges a continued-fault penalty
	// on its FULL_QA power; with the available balance drained to ~0 and the locked pledge unable to pay,
	// the penalty is parked as FeeDebt.
	um.DeclareFaults([]abi.SectorNumber{sn})

	endDebt := time.Now().Add(3 * time.Minute)
	var debtAccrued abi.TokenAmount
	for {
		_, _, feeDebt := ledger()
		if feeDebt.GreaterThan(big.Zero()) {
			debtAccrued = feeDebt
			break
		}
		if time.Now().After(endDebt) {
			require.FailNowf(t, "FeeDebt accrual timeout",
				"continued-fault penalty never produced FeeDebt with a drained available balance; balance=%s", balanceOnly())
		}
		h, herr := client.ChainHead(ctx)
		req.NoError(herr)
		client.WaitTillChain(ctx, kit.HeightAtLeast(h.Height()+40))
	}

	// The 10x sector is faulted but NOT terminated -- the miner is simply carrying outstanding FeeDebt.
	faulted, err := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isFaulted, err := faulted.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isFaulted, "the 10x sector must be declared faulty while it accrues FeeDebt")
	stillInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotNil(stillInfo, "the faulted 10x sector must still exist (miner not terminated for debt)")
	t.Logf("FeeDebt accrued on the drained, faulted 10x sector: %s", debtAccrued)

	// ---- Top the miner back up (plain value transfer) and call RepayDebt: the fresh available balance
	// is burned toward the debt until it is fully cleared.
	sendFromOwner(types.FromFil(10), builtin.MethodSend, nil) // plain transfer -> available balance
	// RepayDebt takes an empty payload; nil params is the correct empty value for this method.
	sendFromOwner(big.Zero(), builtin.MethodsMiner.RepayDebt, nil)

	// ---- FeeDebt must be back to 0 and the miner healthy.
	_, avail2, debt2 := ledger()
	req.True(debt2.IsZero(), "RepayDebt must clear the FeeDebt back to 0; remaining=%s", debt2)
	req.True(avail2.GreaterThan(big.Zero()), "the miner must hold positive available balance after the top-up")

	// Wait one full proving period and re-check: the (still faulted) sector keeps accruing a continued
	// fault fee each period, but the generous top-up means the cron repays each new fee out of the fresh
	// available balance, so FeeDebt stays at 0.
	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(di.Open+di.WPoStProvingPeriod+10))
	_, _, debt3 := ledger()
	req.True(debt3.IsZero(),
		"after one more proving period FeeDebt must still be 0 (top-up covers the continuing fault fee); remaining=%s", debt3)

	um.AssertNoWindowPostError()
}
