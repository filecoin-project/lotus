package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
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
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

// Sector size for every miner here, and the unit power assertions count in.
const feesSectorSize = abi.SectorSize(2 << 10) // 2KiB

// solsticeFees is the shared fixture: one chain, four unmanaged miners.
//
//	ledger   a 1x sector, one upgraded to 10x and a native 10x sector, terminated one by one
//	fault1x  a single 1x sector, drained and faulted
//	fault10x a single native 10x sector, drained and faulted, then repaid
//	usqd     a single sector upgraded to 10x and faulted, funded throughout so it can recover
type solsticeFees struct {
	ctx          context.Context
	client       *kit.TestFullNode
	ledger       *kit.TestUnmanagedMiner
	fault1x      *kit.TestUnmanagedMiner
	fault10x     *kit.TestUnmanagedMiner
	usqd         *kit.TestUnmanagedMiner
	sealProof    abi.RegisteredSealProof
	upgradeEpoch abi.ChainEpoch

	anchor    abi.SectorNumber // stays 1x, terminated first
	upgraded  abi.SectorNumber // 1x then 10x, terminated second
	native    abi.SectorNumber // born 10x, terminated third
	faulted1x abi.SectorNumber
	faulted10 abi.SectorNumber
	faultedUp abi.SectorNumber

	declaredAt map[address.Address]abi.ChainEpoch // execution epoch of each miner's fault declaration
}

// TestSolsticeMinerFees drives what FIP-0118's tiers cost a miner: the fee a fault charges, the debt
// it builds on a drained miner, the repayment that clears it, and the fee a termination takes.
func TestSolsticeMinerFees(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upgradeEpoch := miner14.ChainFinality + miner.WPoStProvingPeriod() + 12*miner.WPoStChallengeWindow()
	t.Logf("NV28 to NV29 at epoch %d", upgradeEpoch)

	sealProof, err := miner.SealProofTypeFromSectorSize(feesSectorSize, network.Version28, miner.SealProofVariant_Standard)
	req.NoError(err)

	bal := types.FromFil(1000)
	ledgerKey := must.One(key.GenerateKey(types.KTBLS))
	fault1xKey := must.One(key.GenerateKey(types.KTBLS))
	fault10xKey := must.One(key.GenerateKey(types.KTBLS))
	usqdKey := must.One(key.GenerateKey(types.KTBLS))

	var client kit.TestFullNode
	var producer kit.TestMiner
	ens := kit.NewEnsemble(t,
		kit.MockProofs(),
		kit.Account(ledgerKey, bal),
		kit.Account(fault1xKey, bal),
		kit.Account(fault10xKey, bal),
		kit.Account(usqdKey, bal),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1}, // genesis is NV28
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	).
		FullNode(&client, kit.SectorSize(feesSectorSize), kit.ThroughRPC()).
		Miner(&producer, &client, kit.SectorSize(feesSectorSize), kit.WithAllSubsystems()).
		Start().
		InterconnectAll()

	blockMiners := ens.BeginMiningMustPost(5 * time.Millisecond)
	req.Len(blockMiners, 1)
	blockMiner := blockMiners[0]

	minerOpts := func(k *key.Key) []kit.NodeOpt {
		return []kit.NodeOpt{kit.SectorSize(feesSectorSize), kit.OwnerAddr(k)}
	}
	ledger, ens := ens.UnmanagedMiner(ctx, &client, minerOpts(ledgerKey)...)
	defer ledger.Stop()
	fault1x, ens := ens.UnmanagedMiner(ctx, &client, minerOpts(fault1xKey)...)
	defer fault1x.Stop()
	fault10x, ens := ens.UnmanagedMiner(ctx, &client, minerOpts(fault10xKey)...)
	defer fault10x.Stop()
	usqd, ens := ens.UnmanagedMiner(ctx, &client, minerOpts(usqdKey)...)
	defer usqd.Stop()
	ens.Start()

	f := &solsticeFees{
		ctx: ctx, client: &client,
		ledger: ledger, fault1x: fault1x, fault10x: fault10x, usqd: usqd,
		sealProof: sealProof, upgradeEpoch: upgradeEpoch,
		declaredAt: map[address.Address]abi.ChainEpoch{},
	}

	f.onboardBeforeFork(t, blockMiner)
	f.crossTheFork(t)
	f.onboardNative(t)
	f.upgradeBeforeFaulting(t)
	closes := f.drainAndFault(t)

	f.terminationFees(t)
	firstDebt10x := f.faultFees(t, closes)
	f.repayDebt(t, closes[f.fault10x.ActorAddr], firstDebt10x)
	f.declareRecoveries(t)
}

// onboardBeforeFork brings up the sectors that have to be legacy: the ledger's anchor and the sector
// it upgrades later, and one each for the miner that faults at 1x and the miner that faults upgraded.
func (f *solsticeFees) onboardBeforeFork(t *testing.T, blockMiner *kit.BlockMiner) {
	req := require.New(t)
	unit := uint64(feesSectorSize)

	var ledgerSectors, fault1xSectors, usqdSectors []abi.SectorNumber
	var eg errgroup.Group
	eg.Go(func() error {
		ledgerSectors, _ = f.ledger.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(2))
		return nil
	})
	eg.Go(func() error {
		fault1xSectors, _ = f.fault1x.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(1))
		return nil
	})
	eg.Go(func() error {
		usqdSectors, _ = f.usqd.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(1))
		return nil
	})
	req.NoError(eg.Wait())
	req.Len(ledgerSectors, 2, "the ledger miner did not finish onboarding")
	req.Len(fault1xSectors, 1, "the 1x fault miner did not finish onboarding")
	req.Len(usqdSectors, 1, "the usqd miner did not finish onboarding")

	f.anchor, f.upgraded = ledgerSectors[0], ledgerSectors[1]
	f.faulted1x = fault1xSectors[0]
	f.faultedUp = usqdSectors[0]

	for _, m := range []*kit.TestUnmanagedMiner{f.ledger, f.fault1x, f.fault10x, f.usqd} {
		blockMiner.WatchMinerForPost(m.ActorAddr)
	}

	f.ledger.WaitTillActivatedAndAssertPower(ledgerSectors, unit*2, unit*2)
	f.fault1x.WaitTillActivatedAndAssertPower(fault1xSectors, unit, unit)
	f.usqd.WaitTillActivatedAndAssertPower(usqdSectors, unit, unit)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	req.Less(head.Height(), f.upgradeEpoch,
		"onboarding must finish before the fork at %d; it ran to %d", f.upgradeEpoch, head.Height())

	for _, m := range []*kit.TestUnmanagedMiner{f.ledger, f.fault1x, f.usqd} {
		_, available, debt := f.minerLedger(t, m.ActorAddr, head.Key())
		req.True(debt.IsZero(), "miner %s must start free of fee debt, has %s", m.ActorAddr, debt)
		req.True(available.GreaterThan(big.Zero()),
			"miner %s must start with something to spend, has %s", m.ActorAddr, available)
	}
	nv, err := f.client.StateNetworkVersion(f.ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version28, nv, "every pre-fork assertion must be made on NV28")
}

func (f *solsticeFees) crossTheFork(t *testing.T) {
	req := require.New(t)

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.upgradeEpoch+5))
	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	nv, err := f.client.StateNetworkVersion(f.ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "the chain must be on NV29 after the migration")
}

// onboardNative adds the sectors that are born at 10x: the ledger's third sector and the one the
// drained miner faults.
func (f *solsticeFees) onboardNative(t *testing.T) {
	req := require.New(t)
	unit := uint64(feesSectorSize)

	var ledgerNative, faultNative []abi.SectorNumber
	var eg errgroup.Group
	eg.Go(func() error {
		ledgerNative, _ = f.ledger.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(1))
		return nil
	})
	eg.Go(func() error {
		faultNative, _ = f.fault10x.OnboardSectors(f.sealProof, kit.NewSectorBatch().AddEmptySectors(1))
		return nil
	})
	req.NoError(eg.Wait())
	req.Len(ledgerNative, 1, "the ledger miner did not finish native onboarding")
	req.Len(faultNative, 1, "the 10x fault miner did not finish native onboarding")

	f.native = ledgerNative[0]
	f.faulted10 = faultNative[0]

	kit.WaitForMinerQAP(f.ctx, t, f.client, f.ledger.ActorAddr, unit*(1+1+10), 5*time.Minute)
	kit.WaitForMinerQAP(f.ctx, t, f.client, f.fault10x.ActorAddr, unit*10, 5*time.Minute)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	for _, s := range []struct {
		maddr  address.Address
		sector abi.SectorNumber
	}{{f.ledger.ActorAddr, f.native}, {f.fault10x.ActorAddr, f.faulted10}} {
		info, err := f.client.StateSectorGetInfo(f.ctx, s.maddr, s.sector, head.Key())
		req.NoError(err)
		req.NotNil(info)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "a sector born on NV29 must carry FULL_QA_POWER")
	}
	_, available, debt := f.minerLedger(t, f.fault10x.ActorAddr, head.Key())
	req.True(debt.IsZero(), "the 10x fault miner must start free of fee debt, has %s", debt)
	req.True(available.GreaterThan(big.Zero()), "the 10x fault miner must start with something to spend")
}

// upgradeBeforeFaulting raises the two sectors that fault or terminate at 10x through the upgrade
// rather than by birth. Both happen before anything faults: an upgrade needs an active sector.
func (f *solsticeFees) upgradeBeforeFaulting(t *testing.T) {
	req := require.New(t)
	unit := uint64(feesSectorSize)

	beforeLedger := f.minerQAP(t, f.ledger.ActorAddr)
	_, err := f.ledger.UpgradeSectorQuality([]abi.SectorNumber{f.upgraded}, nil)
	req.NoError(err)
	req.Equal(beforeLedger+unit*9, f.minerQAP(t, f.ledger.ActorAddr), "the upgraded sector must be worth 9x more")

	_, err = f.usqd.UpgradeSectorQuality([]abi.SectorNumber{f.faultedUp}, nil)
	req.NoError(err)
	req.Equal(unit*10, f.minerQAP(t, f.usqd.ActorAddr),
		"the upgraded sector is this miner's only power, and it is now at 10x")

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	for _, s := range []struct {
		maddr  address.Address
		sector abi.SectorNumber
	}{{f.ledger.ActorAddr, f.upgraded}, {f.usqd.ActorAddr, f.faultedUp}} {
		info, err := f.client.StateSectorGetInfo(f.ctx, s.maddr, s.sector, head.Key())
		req.NoError(err)
		req.NotZero(info.Flags&miner.FULL_QA_POWER, "the upgraded sector %d must carry FULL_QA_POWER", s.sector)
	}
}

// drainAndFault empties the two drained miners and faults all three faulting sectors, and returns the
// epoch at which each fault's deadline next closes, which is when the actor charges for it.
func (f *solsticeFees) drainAndFault(t *testing.T) map[address.Address]abi.ChainEpoch {
	req := require.New(t)

	for _, m := range []*kit.TestUnmanagedMiner{f.fault1x, f.fault10x} {
		params := must.One(actors.SerializeParams(&stminer.WithdrawBalanceParams{AmountRequested: types.FromFil(1000)}))
		msg, err := f.client.MpoolPushMessage(f.ctx, &types.Message{
			From: m.OwnerKey.Address, To: m.ActorAddr, Value: big.Zero(),
			Method: builtin.MethodsMiner.WithdrawBalance, Params: params,
		}, nil)
		req.NoError(err)
		lookup, err := f.client.StateWaitMsg(f.ctx, msg.Cid(), 2, lapi.LookbackNoLimit, true)
		req.NoError(err)
		req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "draining miner %s", m.ActorAddr)

		_, available, debt := f.minerLedger(t, m.ActorAddr, lookup.TipSet)
		req.True(available.LessThan(types.NewInt(1e6)),
			"miner %s must be drained to nothing, has %s", m.ActorAddr, available)
		req.True(debt.IsZero(), "draining a miner must not put it in debt, %s has %s", m.ActorAddr, debt)
	}

	// The usqd miner keeps its balance: a recovery settles fee debt first, so a drained miner could
	// not declare one.
	_, usqdAvailable, _ := f.minerLedger(t, f.usqd.ActorAddr, types.EmptyTSK)
	req.True(usqdAvailable.GreaterThan(big.Zero()), "the usqd miner must stay funded for its recovery")

	// Each declaration waits for its own mutable window, so they land at different epochs. A miner's
	// first charge follows its own declaration, not the last one: taking one epoch for all three
	// would skip a charge that had already happened for an earlier miner.
	closes := make(map[address.Address]abi.ChainEpoch, 3)
	for _, s := range []struct {
		m      *kit.TestUnmanagedMiner
		sector abi.SectorNumber
	}{{f.fault1x, f.faulted1x}, {f.fault10x, f.faulted10}, {f.usqd, f.faultedUp}} {
		lookup := s.m.DeclareFaults([]abi.SectorNumber{s.sector})
		receiptTipSet, err := f.client.ChainGetTipSet(f.ctx, lookup.TipSet)
		req.NoError(err)
		declarationTipSet, err := f.client.ChainGetTipSet(f.ctx, receiptTipSet.Parents())
		req.NoError(err)
		declarationAt := declarationTipSet.Height()

		loc, err := f.client.StateSectorPartition(f.ctx, s.m.ActorAddr, s.sector, receiptTipSet.Key())
		req.NoError(err)
		di, err := f.client.StateMinerProvingDeadline(f.ctx, s.m.ActorAddr, receiptTipSet.Key())
		req.NoError(err)
		di = kit.DeadlineForHeight(di, declarationAt)
		closeAt := kit.DeadlineCloseAfter(di, loc.Deadline, declarationAt)
		closes[s.m.ActorAddr] = closeAt
		f.declaredAt[s.m.ActorAddr] = declarationAt
		t.Logf("miner %s faulted sector %d in deadline %d at execution epoch %d, first charge cron at epoch %d",
			s.m.ActorAddr, s.sector, loc.Deadline, declarationAt, closeAt-1)
	}
	return closes
}

// terminationFees terminates one sector of each tier and measures what each cost the miner.
func (f *solsticeFees) terminationFees(t *testing.T) {
	req := require.New(t)

	fee1x := f.terminateAndCharge(t, f.anchor, 1)
	feeUpgraded := f.terminateAndCharge(t, f.upgraded, 10)
	feeNative := f.terminateAndCharge(t, f.native, 10)

	t.Logf("termination fees: 1x %s, upgraded 10x %s, native 10x %s", fee1x, feeUpgraded, feeNative)
	req.True(fee1x.GreaterThan(big.Zero()), "terminating a 1x sector must cost something")
	req.True(feeUpgraded.GreaterThan(fee1x), "terminating an upgraded 10x sector must cost more than a 1x one")
	req.True(feeNative.GreaterThan(fee1x), "terminating a native 10x sector must cost more than a 1x one")
}

// terminateAndCharge terminates one of the ledger miner's sectors and returns what it took from the
// miner's balance.
func (f *solsticeFees) terminateAndCharge(t *testing.T, sector abi.SectorNumber, tier uint64) abi.TokenAmount {
	req := require.New(t)
	unit := uint64(feesSectorSize)

	start, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	before, _ := f.client.MinerQAP(f.ctx, f.ledger.ActorAddr, start.Key())
	balanceBefore, _, _ := f.minerLedger(t, f.ledger.ActorAddr, start.Key())

	f.ledger.TerminateSectors([]abi.SectorNumber{sector})
	want := before - unit*tier
	kit.WaitForMinerQAP(f.ctx, t, f.client, f.ledger.ActorAddr, want, 3*time.Minute)
	settling, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(settling.Height()+20))

	done, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	balanceAfter, _, debt := f.minerLedger(t, f.ledger.ActorAddr, done.Key())
	after, _ := f.client.MinerQAP(f.ctx, f.ledger.ActorAddr, done.Key())
	req.True(debt.IsZero(),
		"miner %s is funded, so a termination takes its fee from the balance rather than as debt",
		f.ledger.ActorAddr)
	req.Equal(want, after, "terminating miner %s sector %d must remove its own power and no more",
		f.ledger.ActorAddr, sector)
	// Nothing else moves this miner's balance, so the difference is what the termination cost.
	return big.Sub(balanceBefore, balanceAfter)
}

// faultFees reads each drained miner's fee debt at its own first charge, so the comparison is one
// charge against one charge whatever else the chain did in between.
func (f *solsticeFees) faultFees(t *testing.T, closes map[address.Address]abi.ChainEpoch) abi.TokenAmount {
	req := require.New(t)

	debt1x, at1x := f.debtAtFirstCharge(t, f.fault1x.ActorAddr, closes[f.fault1x.ActorAddr])
	debt10x, at10x := f.debtAtFirstCharge(t, f.fault10x.ActorAddr, closes[f.fault10x.ActorAddr])

	t.Logf("first charge: 1x %s at epoch %d (declared %d), 10x %s at epoch %d (declared %d)",
		debt1x, at1x, f.declaredAt[f.fault1x.ActorAddr],
		debt10x, at10x, f.declaredAt[f.fault10x.ActorAddr])
	req.True(debt1x.GreaterThan(big.Zero()),
		"miner %s, drained and holding a faulted 1x sector, must owe a fee at its first charge, epoch %d",
		f.fault1x.ActorAddr, at1x)
	req.True(debt10x.GreaterThan(big.Zero()),
		"miner %s, drained and holding a faulted 10x sector, must owe a fee at its first charge, epoch %d",
		f.fault10x.ActorAddr, at10x)
	req.True(debt10x.GreaterThan(debt1x),
		"a faulted 10x sector must cost more than a faulted 1x one, one charge each: "+
			"1x owes %s at epoch %d, 10x owes %s at epoch %d",
		debt1x, at1x, debt10x, at10x)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	for _, s := range []struct {
		m      *kit.TestUnmanagedMiner
		sector abi.SectorNumber
	}{{f.fault1x, f.faulted1x}, {f.fault10x, f.faulted10}, {f.usqd, f.faultedUp}} {
		faults, err := f.client.StateMinerFaults(f.ctx, s.m.ActorAddr, head.Key())
		req.NoError(err)
		isFaulted, err := faults.IsSet(uint64(s.sector))
		req.NoError(err)
		req.True(isFaulted, "miner %s sector %d must still be faulted", s.m.ActorAddr, s.sector)

		info, err := f.client.StateSectorGetInfo(f.ctx, s.m.ActorAddr, s.sector, head.Key())
		req.NoError(err)
		req.NotNil(info, "a fee must not terminate miner %s sector %d", s.m.ActorAddr, s.sector)
	}

	// TODO: Assert the legacy 1x sector also loses its QA power, and all three faulted
	// sectors lose their raw power, at this same tipset.
	usqdQAP, _ := f.client.MinerQAP(f.ctx, f.usqd.ActorAddr, head.Key())
	nativeQAP, _ := f.client.MinerQAP(f.ctx, f.fault10x.ActorAddr, head.Key())
	req.Zero(usqdQAP, "faulting miner %s's upgraded sector takes its whole 10x", f.usqd.ActorAddr)
	req.Zero(nativeQAP, "faulting miner %s's native sector takes its whole 10x", f.fault10x.ActorAddr)

	for _, s := range []struct {
		m      *kit.TestUnmanagedMiner
		sector abi.SectorNumber
	}{{f.usqd, f.faultedUp}, {f.fault10x, f.faulted10}} {
		_, err := s.m.UpgradeSectorQuality([]abi.SectorNumber{s.sector}, nil)
		req.Error(err, "miner %s sector %d is faulted and cannot be upgraded", s.m.ActorAddr, s.sector)
		req.Contains(err.Error(), "not active",
			"miner %s sector %d is faulted, so the upgrade must say so", s.m.ActorAddr, s.sector)
	}
	return debt10x
}

// debtAtFirstCharge returns what the miner owes at the state produced by its first fault charge, and
// the epoch that state belongs to. Wait for an observed anchor whose parent has reached the first
// charge, then select the first historical child from that anchor so late callers remain valid.
func (f *solsticeFees) debtAtFirstCharge(t *testing.T, maddr address.Address, closeAt abi.ChainEpoch) (abi.TokenAmount, abi.ChainEpoch) {
	req := require.New(t)

	period := miner.WPoStProvingPeriod()
	firstChargeAt := closeAt - 1
	secondChargeAt := firstChargeAt + period
	anchor := f.client.WaitTillChain(f.ctx, func(ts *types.TipSet) bool {
		parent, err := f.client.ChainGetTipSet(f.ctx, ts.Parents())
		req.NoError(err)
		return parent.Height() >= firstChargeAt
	})

	first, err := f.client.ChainGetTipSetAfterHeight(f.ctx, firstChargeAt, anchor.Key())
	req.NoError(err)
	selected, err := f.client.ChainGetTipSetAfterHeight(f.ctx, first.Height()+1, anchor.Key())
	req.NoError(err)
	req.Greater(selected.Height(), first.Height(),
		"miner %s: no child tipset after the first post-charge tipset at %d", maddr, first.Height())
	parent, err := f.client.ChainGetTipSet(f.ctx, selected.Parents())
	req.NoError(err)
	req.GreaterOrEqual(parent.Height(), firstChargeAt,
		"miner %s: no state at or after its first charge at %d", maddr, firstChargeAt)
	req.Less(parent.Height(), secondChargeAt,
		"miner %s: no state between its first charge at %d and its second at %d",
		maddr, firstChargeAt, secondChargeAt)

	_, _, debt := f.minerLedger(t, maddr, selected.Key())
	return debt, parent.Height()
}

// repayDebt funds the drained 10x miner and clears what it owes, then holds through its next fee
// deadline to see that the top-up covers the fault that is still running.
func (f *solsticeFees) repayDebt(t *testing.T, firstClose abi.ChainEpoch, firstDebt abi.TokenAmount) {
	req := require.New(t)

	f.sendFromOwner(t, f.fault10x, types.FromFil(10), builtin.MethodSend, nil)
	lookup := f.sendFromOwner(t, f.fault10x, big.Zero(), builtin.MethodsMiner.RepayDebt, nil)

	_, available, debt := f.minerLedger(t, f.fault10x.ActorAddr, lookup.TipSet)
	req.True(debt.IsZero(), "repaying must clear the debt, %s left", debt)
	req.True(available.GreaterThan(big.Zero()), "the top-up must leave the miner something to spend")
	f.requireFaulted(t, f.fault10x, f.faulted10, lookup.TipSet)

	repaidAt, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	loc, err := f.client.StateSectorPartition(f.ctx, f.fault10x.ActorAddr, f.faulted10, repaidAt.Key())
	req.NoError(err)
	di, err := f.client.StateMinerProvingDeadline(f.ctx, f.fault10x.ActorAddr, repaidAt.Key())
	req.NoError(err)
	di = kit.DeadlineForHeight(di, repaidAt.Height())
	nextCharge := kit.DeadlineCloseAfter(di, loc.Deadline, repaidAt.Height())

	debt, chargedAt := f.debtAtFirstCharge(t, f.fault10x.ActorAddr, nextCharge)
	req.True(debt.IsZero(),
		"the top-up must cover miner %s's continuing fault: %s owed at its next charge, epoch %d",
		f.fault10x.ActorAddr, debt, chargedAt)
	historicalDebt, historicalAt := f.debtAtFirstCharge(t, f.fault10x.ActorAddr, firstClose)
	req.Equal(firstDebt.String(), historicalDebt.String(),
		"the historical first-charge read must preserve the original debt after the next charge")
	t.Logf("historical first charge re-read: %s at epoch %d", historicalDebt, historicalAt)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	f.requireFaulted(t, f.fault10x, f.faulted10, head.Key())
}

// declareRecoveries declares both faulted 10x sectors recovered, reading the receipt child whose
// state includes execution in the parent.
func (f *solsticeFees) declareRecoveries(t *testing.T) {
	req := require.New(t)

	// TODO: Assert recovery declarations leave raw and QA power at zero, then wait for
	// successful WindowPoSt and assert restored raw power, full 10x QA power, and cleared
	// fault/recovery bits for both the upgraded and the repaid native sector.

	for _, s := range []struct {
		m      *kit.TestUnmanagedMiner
		sector abi.SectorNumber
		name   string
	}{
		{f.usqd, f.faultedUp, "an upgraded sector"},
		{f.fault10x, f.faulted10, "a native sector"},
	} {
		lookup := s.m.RecoverFaults([]abi.SectorNumber{s.sector})

		// StateWaitMsg returns the receipt child; a later deadline cron clears the recovering set.
		recoveries, err := f.client.StateMinerRecoveries(f.ctx, s.m.ActorAddr, lookup.TipSet)
		req.NoError(err)
		isRecovering, err := recoveries.IsSet(uint64(s.sector))
		req.NoError(err)
		req.True(isRecovering, "the recovery of %s must be recorded in the receipt-child state at epoch %d",
			s.name, lookup.Height)
	}

	for _, m := range []*kit.TestUnmanagedMiner{f.ledger, f.fault1x, f.fault10x, f.usqd} {
		m.AssertNoWindowPostError()
	}
}

func (f *solsticeFees) requireFaulted(t *testing.T, m *kit.TestUnmanagedMiner, sector abi.SectorNumber, tsk types.TipSetKey) {
	t.Helper()
	req := require.New(t)

	faults, err := f.client.StateMinerFaults(f.ctx, m.ActorAddr, tsk)
	req.NoError(err)
	isFaulted, err := faults.IsSet(uint64(sector))
	req.NoError(err)
	req.True(isFaulted, "sector %d must still be faulted", sector)
}

func (f *solsticeFees) sendFromOwner(t *testing.T, m *kit.TestUnmanagedMiner, value abi.TokenAmount, method abi.MethodNum, params []byte) *lapi.MsgLookup {
	t.Helper()
	req := require.New(t)

	msg, err := f.client.MpoolPushMessage(f.ctx, &types.Message{
		From: m.OwnerKey.Address, To: m.ActorAddr, Value: value, Method: method, Params: params,
	}, nil)
	req.NoError(err)
	lookup, err := f.client.StateWaitMsg(f.ctx, msg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err)
	req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "message to %s with method %d", m.ActorAddr, method)
	return lookup
}

func (f *solsticeFees) minerQAP(t *testing.T, maddr address.Address) uint64 {
	t.Helper()
	head, err := f.client.ChainHead(f.ctx)
	require.NoError(t, err)
	qap, _ := f.client.MinerQAP(f.ctx, maddr, head.Key())
	return qap
}

// minerLedger reads a miner's balance, what of it is free to spend, and what it owes.
func (f *solsticeFees) minerLedger(t *testing.T, maddr address.Address, tsk types.TipSetKey) (balance, available, feeDebt abi.TokenAmount) {
	t.Helper()
	req := require.New(t)

	act, err := f.client.StateGetActor(f.ctx, maddr, tsk)
	req.NoError(err)
	st := f.client.MinerState(f.ctx, maddr, tsk)

	available = big.Subtract(act.Balance, st.LockedFunds, st.PreCommitDeposits, st.InitialPledge, st.FeeDebt)
	if available.LessThan(big.Zero()) {
		available = big.Zero()
	}
	return act.Balance, available, st.FeeDebt
}
