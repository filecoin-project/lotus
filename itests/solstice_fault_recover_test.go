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
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

// solsticeRecover is the shared fixture: one managed miner that seals and proves through its own
// scheduler, holding a sector raised to 10x by the quality upgrade and three born at 10x on NV29.
type solsticeRecover struct {
	ctx          context.Context
	client       *kit.TestFullNode
	miner        *kit.TestMiner
	maddr        address.Address
	minerID      abi.ActorID
	sectorSize   abi.SectorSize
	upgradeEpoch abi.ChainEpoch

	upgraded abi.SectorNumber   // pledged before the fork, raised to 10x afterwards
	natives  []abi.SectorNumber // pledged after the fork, born at 10x
}

// TestSolsticeFaultRecover takes a sector of each provenance through a storage failure and back: the
// fault takes its whole 10x, the recovery is recorded, and proving it again restores the same power.
func TestSolsticeFaultRecover(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	// One sector recovers per WindowPoSt, so each cycle below is observed on its own.
	wasLimit := wdpost.RecoveringSectorLimit
	wdpost.RecoveringSectorLimit = 1
	t.Cleanup(func() { wdpost.RecoveringSectorLimit = wasLimit })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Nothing is read before the fork except the pledged sector's activation epoch, which is read
	// afterwards, so the chain only has to reach seal randomness.
	upgradeEpoch := miner14.ChainFinality + 4*miner.WPoStChallengeWindow()
	t.Logf("NV28 to NV29 at epoch %d", upgradeEpoch)

	client, managed, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1}, // genesis is NV28
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)
	// The faults here come from the miner's own storage, which the chain only learns about when a
	// WindowPoSt goes missing, so block production must not wait for one.
	ens.InterconnectAll().BeginMining(2 * time.Millisecond)

	maddr, err := managed.ActorAddress(ctx)
	req.NoError(err)
	minerID, err := address.IDFromAddress(maddr)
	req.NoError(err)
	sectorSize, err := managed.ActorSectorSize(ctx, maddr)
	req.NoError(err)

	f := &solsticeRecover{
		ctx: ctx, client: client, miner: managed, maddr: maddr,
		minerID: abi.ActorID(minerID), sectorSize: sectorSize, upgradeEpoch: upgradeEpoch,
	}

	t.Run("a sector pledged before the fork is legacy and upgrades to 10x", func(t *testing.T) {
		f.pledgeBeforeFork(t)
		f.upgradeLegacy(t)
	})
	t.Run("a storage failure takes an upgraded sector's whole 10x, and recovery restores it", func(t *testing.T) {
		f.failAndRecover(t, f.upgraded, "the upgraded sector")
	})
	t.Run("sectors pledged after the fork are born at 10x", func(t *testing.T) {
		f.pledgeAfterFork(t)
	})
	t.Run("a storage failure takes a native sector's whole 10x, and recovery restores it", func(t *testing.T) {
		f.failAndRecover(t, f.natives[0], "a native sector")
	})
}

// pledgeBeforeFork seals one sector on NV28 and crosses the fork. Its claim is on the activation
// epoch, read here after the fork, so its first proof may land on either side.
func (f *solsticeRecover) pledgeBeforeFork(t *testing.T) {
	req := require.New(t)

	f.miner.PledgeSectors(f.ctx, 1, 0, nil)
	sectors, err := f.miner.SectorsListNonGenesis(f.ctx)
	req.NoError(err)
	req.Len(sectors, 1, "miner %s must hold the one sector it pledged", f.maddr)
	f.upgraded = sectors[0]

	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(f.upgradeEpoch+5))
	f.waitRawPower(t, 1)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	nv, err := f.client.StateNetworkVersion(f.ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "the chain must be on NV29 after the migration")

	info := f.client.MustSectorInfo(f.ctx, f.maddr, f.upgraded, head.Key())
	req.Less(info.Activation, f.upgradeEpoch,
		"miner %s sector %d activated before the fork, so it is a legacy sector", f.maddr, f.upgraded)
	req.Zero(info.Flags&miner.FULL_QA_POWER,
		"miner %s sector %d must not carry FULL_QA_POWER", f.maddr, f.upgraded)
}

// upgradeLegacy raises the legacy sector to 10x, which is where the first fault cycle starts.
func (f *solsticeRecover) upgradeLegacy(t *testing.T) {
	req := require.New(t)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	before, _ := f.client.MinerQAP(f.ctx, f.maddr, head.Key())

	params := f.client.UpgradeSectorQualityParams(f.ctx, f.maddr, f.upgraded, head.Key())
	msg, err := f.client.MpoolPushMessage(f.ctx, &types.Message{
		From: f.miner.OwnerKey.Address, To: f.maddr,
		Method: builtin.MethodsMiner.UpgradeSectorQuality, Params: params, Value: big.Zero(),
	}, nil)
	req.NoError(err)
	lookup, err := f.client.StateWaitMsg(f.ctx, msg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err)
	req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "upgrading miner %s sector %d", f.maddr, f.upgraded)

	kit.WaitForMinerQAP(f.ctx, t, f.client, f.maddr, before+uint64(f.sectorSize)*9, 3*time.Minute)
	req.NotZero(f.client.MustSectorInfo(f.ctx, f.maddr, f.upgraded, lookup.TipSet).Flags&miner.FULL_QA_POWER,
		"miner %s sector %d must carry FULL_QA_POWER after the upgrade", f.maddr, f.upgraded)
}

// pledgeAfterFork seals three more sectors, which are born at 10x.
func (f *solsticeRecover) pledgeAfterFork(t *testing.T) {
	req := require.New(t)

	had := map[abi.SectorNumber]bool{f.upgraded: true}
	f.miner.PledgeSectors(f.ctx, 3, 1, nil)
	f.waitRawPower(t, 4)

	sectors, err := f.miner.SectorsListNonGenesis(f.ctx)
	req.NoError(err)
	req.Len(sectors, 4, "miner %s must hold all four sectors it pledged", f.maddr)
	for _, sn := range sectors {
		if !had[sn] {
			f.natives = append(f.natives, sn)
		}
	}
	req.Len(f.natives, 3, "three of miner %s's sectors must be new", f.maddr)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	for _, sn := range f.natives {
		info := f.client.MustSectorInfo(f.ctx, f.maddr, sn, head.Key())
		req.GreaterOrEqual(info.Activation, f.upgradeEpoch,
			"miner %s sector %d activated after the fork", f.maddr, sn)
		req.NotZero(info.Flags&miner.FULL_QA_POWER,
			"miner %s sector %d is born at 10x and must carry FULL_QA_POWER", f.maddr, sn)
	}
}

// failAndRecover marks one sector unreadable, waits for the chain to notice, then repairs it and
// declares it recovered. The declaration is read before the restoring proof clears it.
func (f *solsticeRecover) failAndRecover(t *testing.T, sector abi.SectorNumber, name string) {
	req := require.New(t)

	head, err := f.client.ChainHead(f.ctx)
	req.NoError(err)
	whole, _ := f.client.MinerQAP(f.ctx, f.maddr, head.Key())
	loc, err := f.client.StateSectorPartition(f.ctx, f.maddr, sector, head.Key())
	req.NoError(err)

	f.markFailed(t, sector, true)

	head = f.waitForFault(t, sector, loc.Deadline, name)
	faultedQAP, _ := f.client.MinerQAP(f.ctx, f.maddr, head.Key())
	req.Equal(whole-uint64(f.sectorSize)*10, faultedQAP,
		"%s must take its whole 10x with it, not a 1x residue", name)

	f.markFailed(t, sector, false)
	recover, err := f.miner.RecoverFault(f.ctx, []abi.SectorNumber{sector})
	req.NoError(err, "recovering %s must be accepted", name)
	req.NotEmpty(recover, "recovering %s must send a message", name)
	lookup, err := f.client.StateWaitMsg(f.ctx, recover[0], 2, lapi.LookbackNoLimit, true)
	req.NoError(err)
	req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "recovering %s", name)

	// StateWaitMsg returns the receipt child; its state includes the declaration executed in the parent.
	recoveries, err := f.client.StateMinerRecoveries(f.ctx, f.maddr, lookup.TipSet)
	req.NoError(err)
	isRecovering, err := recoveries.IsSet(uint64(sector))
	req.NoError(err)
	req.True(isRecovering, "the recovery of %s must be recorded in the receipt-child state", name)

	kit.WaitForMinerQAP(f.ctx, t, f.client, f.maddr, whole, 3*time.Minute)

	head, err = f.client.ChainHead(f.ctx)
	req.NoError(err)
	restored, _ := f.client.MinerQAP(f.ctx, f.maddr, head.Key())
	req.Equal(whole, restored, "%s must come back at its whole 10x", name)

	faults, err := f.client.StateMinerFaults(f.ctx, f.maddr, head.Key())
	req.NoError(err)
	stillFaulted, err := faults.IsSet(uint64(sector))
	req.NoError(err)
	req.False(stillFaulted, "%s must leave the fault set once it is proven again", name)
	req.NotZero(f.client.MustSectorInfo(f.ctx, f.maddr, sector, head.Key()).Flags&miner.FULL_QA_POWER,
		"%s must keep FULL_QA_POWER through the whole cycle", name)
}

// waitForFault waits until the chain has recorded the sector faulty and its deadline has closed, so
// the power the fault removes has been taken. It returns the tipset that was true at.
func (f *solsticeRecover) waitForFault(t *testing.T, sector abi.SectorNumber, dlIdx uint64, name string) *types.TipSet {
	req := require.New(t)

	di := f.client.DeadlineForHead(f.ctx, f.maddr)
	settled := kit.DeadlineCloseAfter(di, dlIdx, di.CurrentEpoch)
	f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(settled+5))

	end := time.Now().Add(4 * time.Minute)
	for {
		head, err := f.client.ChainHead(f.ctx)
		req.NoError(err)
		faults, err := f.client.StateMinerFaults(f.ctx, f.maddr, head.Key())
		req.NoError(err)
		isFaulted, err := faults.IsSet(uint64(sector))
		req.NoError(err)
		if isFaulted {
			return head
		}
		if time.Now().After(end) {
			req.FailNowf("fault wait timeout",
				"%s, miner %s sector %d, never entered the fault set", name, f.maddr, sector)
		}
		f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(head.Height()+di.WPoStChallengeWindow))
	}
}

// markFailed makes the miner's storage report the sector as unreadable, or readable again.
func (f *solsticeRecover) markFailed(t *testing.T, sector abi.SectorNumber, failed bool) {
	t.Helper()
	sealer := f.miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr)
	require.NoError(t, sealer.MarkFailed(storiface.SectorRef{
		ID: abi.SectorID{Miner: f.minerID, Number: sector},
	}, failed))
}

// waitRawPower waits until the miner's own sectors and its genesis preseals are all counted, so a
// later quality-adjusted reading is of the whole miner.
func (f *solsticeRecover) waitRawPower(t *testing.T, pledged int) {
	req := require.New(t)

	want := uint64(kit.DefaultPresealsPerBootstrapMiner+pledged) * uint64(f.sectorSize)
	end := time.Now().Add(4 * time.Minute)
	for {
		head, err := f.client.ChainHead(f.ctx)
		req.NoError(err)
		if f.client.MinerRawPower(f.ctx, f.maddr, head.Key()) == want {
			return
		}
		if time.Now().After(end) {
			req.FailNowf("raw power wait timeout",
				"miner %s never reached %d bytes of raw power", f.maddr, want)
		}
		f.client.WaitTillChain(f.ctx, kit.HeightAtLeast(head.Height()+40))
	}
}

// TestSolsticeWorkerHandover hands the worker role to a second address and leaves the owner holding
// nothing else, then asks the actor who can raise a sector's quality. Owner, worker and control all
// can; nobody else does.
func TestSolsticeWorkerHandover(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	// Nothing here is read before the fork, so the chain only has to reach seal randomness.
	upgradeEpoch := miner14.ChainFinality + 4*miner.WPoStChallengeWindow()
	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	defer um.Stop()
	owner := client.DefaultKey.Address

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
	req.NoError(err)
	req.Equal(network.Version29, nv, "the chain must be on NV29 after the migration")

	onboarded, _ := um.OnboardSectors(e.SealProof, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(onboarded, 1)
	um.WaitTillActivatedAndAssertPower(onboarded, uint64(e.Ssize), uint64(e.Ssize)*10)
	sector := onboarded[0]

	// Stop the post loop before handing over the worker. The owner remains authorized to submit
	// SubmitWindowedPoSt after the handover, but this loop signs with the owner's worker key.
	um.Stop()

	// The actor rejects a secp worker, and the other two roles are only ever message senders.
	worker, err := client.WalletNew(ctx, types.KTBLS)
	req.NoError(err)
	control, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelated, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	for _, a := range []address.Address{worker, control, unrelated} {
		kit.SendFunds(ctx, t, client, a, types.FromFil(1))
	}

	changed := must.One(actors.SerializeParams(&stminer.ChangeWorkerAddressParams{
		NewWorker:       worker,
		NewControlAddrs: []address.Address{control},
	}))
	msg, err := client.MpoolPushMessage(ctx, &types.Message{
		From: owner, To: maddr, Method: builtin.MethodsMiner.ChangeWorkerAddress,
		Params: changed, Value: big.Zero(),
	}, nil)
	req.NoError(err)
	lookup, err := client.StateWaitMsg(ctx, msg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err)
	req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "changing the worker")

	// A worker change is queued, not immediate: cron may apply it at a deadline after EffectiveAt,
	// without an owner confirmation.
	state := client.MinerState(ctx, maddr, lookup.TipSet)
	var info stminer.MinerInfo
	req.NoError(client.Store(ctx).Get(ctx, state.Info, &info))
	req.NotNil(info.PendingWorkerKey, "a worker change must be queued")
	effectiveAt := info.PendingWorkerKey.EffectiveAt
	t.Logf("worker change queued, effective at epoch %d", effectiveAt)

	queued, err := client.StateMinerInfo(ctx, maddr, lookup.TipSet)
	req.NoError(err)
	req.Equal(ownerAsWorker(ctx, t, client, maddr, lookup.TipSet), queued.Worker,
		"miner %s keeps its old worker until the change takes effect", maddr)

	// Cron applies a pending worker at the first deadline after EffectiveAt; confirming only forces
	// it sooner.

	client.WaitTillChain(ctx, kit.HeightAtLeast(effectiveAt+20))
	confirm, err := client.MpoolPushMessage(ctx, &types.Message{
		From: owner, To: maddr, Method: builtin.MethodsMiner.ConfirmChangeWorkerAddress, Value: big.Zero(),
	}, nil)
	req.NoError(err)
	lookup, err = client.StateWaitMsg(ctx, confirm.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err)
	req.Equal(exitcode.Ok, lookup.Receipt.ExitCode, "confirming the worker change")

	resolve := func(a address.Address) address.Address {
		id, err := client.StateLookupID(ctx, a, lookup.TipSet)
		req.NoError(err)
		return id
	}
	ownerID, workerID, controlID := resolve(owner), resolve(worker), resolve(control)

	mi, err := client.StateMinerInfo(ctx, maddr, lookup.TipSet)
	req.NoError(err)
	req.Equal(ownerID, mi.Owner, "the owner must be unchanged")
	req.Equal(workerID, mi.Worker, "the worker must be the new address")
	req.Contains(mi.ControlAddresses, controlID, "the control address must be installed")
	req.NotContains(mi.ControlAddresses, ownerID, "the owner must hold no other role")

	upgrade := client.UpgradeSectorQualityParams(ctx, maddr, sector, lookup.TipSet)
	callExit := func(tsk types.TipSetKey, from address.Address) exitcode.ExitCode {
		res, err := client.StateCall(ctx, &types.Message{
			From: from, To: maddr, Method: builtin.MethodsMiner.UpgradeSectorQuality,
			Params: upgrade, Value: big.Zero(),
		}, tsk)
		req.NoError(err)
		return res.MsgRct.ExitCode
	}

	probeAt, err := client.ChainHead(ctx)
	req.NoError(err)
	faults, err := client.StateMinerFaults(ctx, maddr, probeAt.Key())
	req.NoError(err)
	healthy, err := faults.IsSet(uint64(sector))
	req.NoError(err)
	healthy = !healthy

	req.Equal(exitcode.ErrForbidden, callExit(probeAt.Key(), unrelated),
		"an unrelated address may not upgrade miner %s sector %d", maddr, sector)

	ownerExit := callExit(probeAt.Key(), owner)
	workerExit := callExit(probeAt.Key(), worker)
	controlExit := callExit(probeAt.Key(), control)
	t.Logf("at epoch %d, sector %d healthy=%v: owner %d, worker %d, control %d",
		probeAt.Height(), sector, healthy, ownerExit, workerExit, controlExit)

	if healthy {
		// A healthy sector lets an authorised caller run the upgrade through to the end.
		req.Equal(exitcode.Ok, ownerExit, "an owner holding no other role may upgrade miner %s sector %d", maddr, sector)
		req.Equal(exitcode.Ok, workerExit, "the new worker may upgrade miner %s sector %d", maddr, sector)
		req.Equal(exitcode.Ok, controlExit, "the control address may upgrade miner %s sector %d", maddr, sector)
	} else {
		// A faulted sector fails on its state instead, which is not a refusal of the caller.
		req.NotEqual(exitcode.ErrForbidden, ownerExit, "an owner holding no other role is not refused as a caller")
		req.NotEqual(exitcode.ErrForbidden, workerExit, "the new worker is not refused as a caller")
		req.NotEqual(exitcode.ErrForbidden, controlExit, "the control address is not refused as a caller")
	}
	req.Equal(ownerExit, workerExit, "the three authorised callers must fare the same")
	req.Equal(ownerExit, controlExit, "the three authorised callers must fare the same")
}

// ownerAsWorker is the miner's worker before any handover, which is its owner in this fixture.
func ownerAsWorker(ctx context.Context, t *testing.T, client *kit.TestFullNode, maddr address.Address, tsk types.TipSetKey) address.Address {
	t.Helper()
	id, err := client.StateLookupID(ctx, client.DefaultKey.Address, tsk)
	require.NoError(t, err)
	return id
}
