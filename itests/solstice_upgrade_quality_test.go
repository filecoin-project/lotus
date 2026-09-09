package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
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
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

// TestMigrationNV29SolsticeUpgradeQualityAuth asserts USQ (method 37) caller authorization: owner/worker and control accepted, unrelated rejected with USR_FORBIDDEN.
func TestMigrationNV29SolsticeUpgradeQualityAuth(t *testing.T) {
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

	// fundAccount creates an account actor so `a` can be used as a StateCall From.
	fundAccount := func(a address.Address) {
		kit.SendFunds(ctx, t, client, a, types.FromFil(1))
	}

	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))
	sn := legacy[0]

	lInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER before USQ")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	ctrlAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelatedAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	fundAccount(ctrlAddr)
	fundAccount(unrelatedAddr)

	mi, err := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	cwp := &stminer.ChangeWorkerAddressParams{
		NewWorker:       mi.Worker, // unchanged -> no worker handover delay
		NewControlAddrs: []address.Address{ctrlAddr},
	}
	cwEnc, aerr := actors.SerializeParams(cwp)
	req.NoError(aerr)
	cwMsg, err := client.MpoolPushMessage(ctx, &types.Message{
		From:   client.DefaultKey.Address, // owner is the only authorised caller of ChangeWorkerAddress
		To:     maddr,
		Method: builtin.MethodsMiner.ChangeWorkerAddress,
		Params: cwEnc,
		Value:  types.FromFil(0),
	}, nil)
	req.NoError(err)
	_, err = client.StateWaitMsg(ctx, cwMsg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err, "ChangeWorkerAddress must be confirmed")

	ctrlID, err := client.StateLookupID(ctx, ctrlAddr, types.EmptyTSK)
	req.NoError(err)
	defaultID, err := client.StateLookupID(ctx, client.DefaultKey.Address, types.EmptyTSK)
	req.NoError(err)
	mi, err = client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Contains(mi.ControlAddresses, ctrlID, "ctrlAddr must now be a control address")
	req.Equal(defaultID, mi.Owner, "owner unchanged")
	req.Equal(defaultID, mi.Worker, "worker unchanged (owner==worker in the kit ensemble)")

	// usqCallExit probes USQ (method 37) via StateCall; returns the exit code.
	usqCallExit := func(from address.Address) exitcode.ExitCode {
		loc, lerr := client.StateSectorPartition(ctx, maddr, sn, types.EmptyTSK)
		req.NoError(lerr)
		enc, sErr := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
			Upgrades: []stminer.UpgradeSectorQuality{{
				Deadline:  loc.Deadline,
				Partition: loc.Partition,
				Sectors:   bitfield.NewFromSet([]uint64{uint64(sn)}),
			}},
		})
		req.NoError(sErr)
		res, cErr := client.StateCall(ctx, &types.Message{
			From:   from,
			To:     maddr,
			Method: builtin.MethodsMiner.UpgradeSectorQuality,
			Params: enc,
			Value:  types.FromFil(0),
		}, types.EmptyTSK)
		req.NoError(cErr)
		return res.MsgRct.ExitCode
	}

	req.Equal(exitcode.Ok, usqCallExit(ctrlAddr),
		"a control address must be authorized to call UpgradeSectorQuality (Ok, not USR_FORBIDDEN)")

	req.Equal(exitcode.ErrForbidden, usqCallExit(unrelatedAddr),
		"an unrelated address must be forbidden from calling UpgradeSectorQuality")

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.NoError(err, "owner/worker USQ must be accepted")

	info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(info.Flags&miner.FULL_QA_POWER,
		"the owner/worker USQ must actually raise the legacy sector to FULL_QA(10x)")
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*10, power.MinerPower.QualityAdjPower.Uint64(),
		"owner/worker USQ lifts the miner's only sector to 10x QAP")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeUpgradeQualityPureOwnerAuth asserts that a pure owner (owner != worker, not control) is authorized for USQ (method 37) while unrelated is USR_FORBIDDEN.
func TestMigrationNV29SolsticeUpgradeQualityPureOwnerAuth(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	e := kit.NewSolsticeUpgradeEnv(t, kit.SolsticeOpts{UpgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.Ctx, e.Client, e.Um, e.Maddr
	sealProofType := e.SealProof

	ownerA := client.DefaultKey.Address

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
	um.Stop()

	// workerB must be BLS (actor rejects secp worker); control and unrelated may be secp.
	workerB, err := client.WalletNew(ctx, types.KTBLS)
	req.NoError(err)
	controlC, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelatedD, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	for _, a := range []address.Address{workerB, controlC, unrelatedD} {
		kit.SendFunds(ctx, t, client, a, types.FromFil(1))
	}

	resolveToID := func(a address.Address) address.Address {
		id, lerr := client.StateLookupID(ctx, a, types.EmptyTSK)
		req.NoError(lerr)
		return id
	}
	workerBID := resolveToID(workerB)

	cwp := &stminer.ChangeWorkerAddressParams{
		NewWorker:       workerB,
		NewControlAddrs: []address.Address{controlC},
	}
	cwEnc, aerr := actors.SerializeParams(cwp)
	req.NoError(aerr)
	cwMsg, err := client.MpoolPushMessage(ctx, &types.Message{
		From:   ownerA,
		To:     maddr,
		Method: builtin.MethodsMiner.ChangeWorkerAddress,
		Params: cwEnc,
		Value:  types.FromFil(0),
	}, nil)
	req.NoError(err)
	_, err = client.StateWaitMsg(ctx, cwMsg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(err, "ChangeWorkerAddress must be confirmed")

	saAct, saErr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
	req.NoError(saErr)
	bs := gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(client))
	var mst stminer.State
	req.NoError(bs.Get(ctx, saAct.Head, &mst))
	var mInfo stminer.MinerInfo
	req.NoError(bs.Get(ctx, mst.Info, &mInfo))
	req.NotNil(mInfo.PendingWorkerKey, "a real worker change must register a pending worker key")
	effectiveAt := mInfo.PendingWorkerKey.EffectiveAt
	t.Logf("worker handover to %s pending, effective at epoch %d", workerBID, effectiveAt)

	client.WaitTillChain(ctx, kit.HeightAtLeast(effectiveAt+20))

	cfmMsg, cerr := client.MpoolPushMessage(ctx, &types.Message{
		From:   ownerA,
		To:     maddr,
		Method: builtin.MethodsMiner.ConfirmChangeWorkerAddress,
		Params: nil, // *abi.EmptyValue
		Value:  types.FromFil(0),
	}, nil)
	req.NoError(cerr)
	_, cerr = client.StateWaitMsg(ctx, cfmMsg.Cid(), 2, lapi.LookbackNoLimit, true)
	req.NoError(cerr, "ConfirmChangeWorkerAddress must be confirmed")

	// stateCallExit probes method m via StateCall; returns the exit code.
	stateCallExit := func(from address.Address, m abi.MethodNum, params []byte) exitcode.ExitCode {
		res, cerr := client.StateCall(ctx, &types.Message{
			From:   from,
			To:     maddr,
			Method: m,
			Params: params,
			Value:  types.FromFil(0),
		}, types.EmptyTSK)
		req.NoError(cerr)
		return res.MsgRct.ExitCode
	}

	mi, merr := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	req.NoError(merr)
	ownerAID := resolveToID(ownerA)
	controlCID := resolveToID(controlC)
	aIsControl := false
	for _, c := range mi.ControlAddresses {
		if c == ownerAID {
			aIsControl = true
			break
		}
	}
	req.True(mi.Owner == ownerAID, "owner must be A after the handover; got %s want %s", mi.Owner, ownerAID)
	req.True(mi.Worker == workerBID, "worker must be B after the handover (A is no longer worker); got %s want %s", mi.Worker, workerBID)
	req.Contains(mi.ControlAddresses, controlCID, "control C must be installed after the handover")
	req.False(aIsControl, "A must not be a control address (pure owner); controls=%v", mi.ControlAddresses)
	req.True(mi.Owner == ownerAID && mi.Worker != ownerAID && !aIsControl,
		"guard: A must be owner-only (Owner=A, Worker=B, A not in controls)")
	t.Logf("guard ok: after handover Owner=%s Worker=%s Controls=%v -> A is a pure owner", mi.Owner, mi.Worker, mi.ControlAddresses)

	loc, lerr := client.StateSectorPartition(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(lerr)
	usqEnc, sErr := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
		Upgrades: []stminer.UpgradeSectorQuality{{
			Deadline:  loc.Deadline,
			Partition: loc.Partition,
			Sectors:   bitfield.NewFromSet([]uint64{uint64(sn)}),
		}},
	})
	req.NoError(sErr)

	ownerUSQ := stateCallExit(ownerA, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	workerUSQ := stateCallExit(workerB, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	controlUSQ := stateCallExit(controlC, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	unrelatedUSQ := stateCallExit(unrelatedD, builtin.MethodsMiner.UpgradeSectorQuality, usqEnc)
	t.Logf("UpgradeSectorQuality exit codes: owner(A)=%d worker(B)=%d control(C)=%d unrelated(D)=%d",
		ownerUSQ, workerUSQ, controlUSQ, unrelatedUSQ)

	req.Equal(exitcode.ErrForbidden, unrelatedUSQ, "an unrelated address must be forbidden from method 37")
	req.NotEqual(exitcode.ErrForbidden, workerUSQ, "the distinct worker B must remain authorized for method 37")
	req.NotEqual(exitcode.ErrForbidden, controlUSQ, "the control address must remain authorized for method 37")

	// A pure owner cannot surface USR_FORBIDDEN; sector may fault after A lost worker role.
	faults, ferr := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	req.NoError(ferr)
	ownerSectorFaulted, fserr := faults.IsSet(uint64(sn))
	req.NoError(fserr)
	if ownerSectorFaulted {
		req.NotEqual(exitcode.ErrForbidden, ownerUSQ,
			"a pure owner (owner != worker, not a control) must be authorized for method 37; got %d (sector faulted: authorized owner surfaces only a sector-state error, not USR_FORBIDDEN)", ownerUSQ)
		t.Logf("sector %d is faulted at probe time; pure-owner authorization asserted at the caller gate (ownerUSQ=%d)", sn, ownerUSQ)
	} else {
		req.Equal(exitcode.Ok, ownerUSQ,
			"a pure owner (owner != worker, not a control) must be authorized for method 37 AND run it to completion on an active sector; got %d", ownerUSQ)
		t.Logf("sector %d active at probe time; pure owner ran method 37 to OK on an active sector", sn)
	}
}

// TestMigrationNV29SolsticeFaultAndRecover faults a native FULL_QA(10x) sector: QAP drops to zero, USQ on faulted sector rejected, recovery recorded.
func TestMigrationNV29SolsticeFaultAndRecover(t *testing.T) {
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

	// minerBalanceFeeDebt reads actor balance and FeeDebt for diagnostics.
	minerBalanceFeeDebt := func() (balance, feeDebt string) {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		blk := blockstore.NewAPIBlockstore(client)
		stor := gstStore.WrapBlockStore(ctx, blk)
		var mst stminer.State
		req.NoError(stor.Get(ctx, act.Head, &mst))
		return act.Balance.String(), mst.FeeDebt.String()
	}

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	onboarded, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(onboarded, 1)
	um.WaitTillActivatedAndAssertPower(onboarded,
		uint64(defaultSectorSize), uint64(defaultSectorSize)*10)
	sn := onboarded[0]

	info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(info.Flags&miner.FULL_QA_POWER, "native NV29 CC sector must carry FULL_QA_POWER (10x)")

	um.DeclareFaults([]abi.SectorNumber{sn})

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(di.Open+di.WPoStProvingPeriod+1))

	faults, err := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isFaulted, err := faults.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isFaulted, "sector %d must be faulted after a proving period", sn)
	if bal, debt := minerBalanceFeeDebt(); true {
		t.Logf("after fault effective: miner balance=%s feeDebt=%s", bal, debt)
	}

	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.True(power.MinerPower.QualityAdjPower.IsZero(),
		"faulting the only (10x) sector must remove all QAP; got %s", power.MinerPower.QualityAdjPower)

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.Error(err, "USQ on a faulted sector must be rejected")
	req.Contains(err.Error(), "not active", "USQ on a faulted sector must fail with 'sector is not active'")

	if bal, debt := minerBalanceFeeDebt(); true {
		t.Logf("after fault: miner balance=%s feeDebt=%s (power still 0)", bal, debt)
	}

	um.RecoverFaults([]abi.SectorNumber{sn})

	recs, err := client.StateMinerRecoveries(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isRecovering, err := recs.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isRecovering, "a DeclareFaultsRecovered on the 10x sector must be accepted and recorded")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeFaultRecoverFullPower exercises fault→recover on a managed miner: faulting one of three FULL_QA(10x) sectors drops QAP by 10x, recovery restores the full 10x.
func TestMigrationNV29SolsticeFaultRecoverFullPower(t *testing.T) {
	kit.QuietMiningLogs()

	oldVal := wdpost.RecoveringSectorLimit
	defer func() { wdpost.RecoveringSectorLimit = oldVal }()
	wdpost.RecoveringSectorLimit = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const upgradeEpoch = abi.ChainEpoch(1500)
	blocktime := 2 * time.Millisecond

	client, m, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	maddr, err := m.ActorAddress(ctx)
	require.NoError(t, err)
	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	ssz, err := m.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	require.NoError(t, err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	require.NoError(t, err)
	require.Equal(t, network.Version29, nv, "chain must actually be on NV29 after the migration")

	m.PledgeSectors(ctx, 3, 0, nil)

	sectors, err := m.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sectors, 3)

	for _, sn := range sectors {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		require.NoError(t, err)
		require.NotZero(t, info.Flags&miner.FULL_QA_POWER, "native NV29 CC sector %d must carry FULL_QA_POWER", sn)
	}

	// Wait for raw power to stabilize (preseals + our 3 sectors all active), then read stable QAP.
	rawWant := uint64(kit.DefaultPresealsPerBootstrapMiner+3) * uint64(ssz)
	endRaw := time.Now().Add(4 * time.Minute)
	for {
		pw, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		if pw.MinerPower.RawBytePower.Uint64() == rawWant {
			break
		}
		if time.Now().After(endRaw) {
			require.FailNowf(t, "raw power wait timeout",
				"miner raw power did not reach %d in time; last=%d", rawWant, pw.MinerPower.RawBytePower.Uint64())
		}
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)
		client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+40))
	}
	pre, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	preQAP := pre.MinerPower.QualityAdjPower.Uint64()

	target := sectors[0]

	spart, err := client.StateSectorPartition(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	targetDeadline := spart.Deadline

	markFailed := func(failed bool) {
		require.NoError(t, m.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(
			storiface.SectorRef{ID: abi.SectorID{Miner: abi.ActorID(mid), Number: target}}, failed))
	}
	markFailed(true)

	faulted := waitFaultedAndPastDeadline(ctx, t, client, maddr, target, targetDeadline, preQAP-uint64(ssz)*10, 4*time.Minute)
	require.True(t, faulted, "sector %d must be declared faulty", target)

	markFailed(false)
	_, err = m.RecoverFault(ctx, []abi.SectorNumber{target})
	require.NoError(t, err, "RecoverFault must be accepted")

	head, err = client.ChainHead(ctx)
	require.NoError(t, err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+10))

	recs, err := client.StateMinerRecoveries(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	isRecovering, err := recs.IsSet(uint64(target))
	require.NoError(t, err)
	require.True(t, isRecovering, "the 10x sector must be recorded as recovering")

	kit.WaitForMinerQAP(ctx, t, client, maddr, preQAP, 3*time.Minute)

	faultsAfter, err := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	isFaultedAfter, err := faultsAfter.IsSet(uint64(target))
	require.NoError(t, err)
	require.False(t, isFaultedAfter, "the recovered 10x sector must leave the fault set")

	info, err := client.StateSectorGetInfo(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	require.NotZero(t, info.Flags&miner.FULL_QA_POWER,
		"a recovered native NV29 sector must keep its FULL_QA_POWER flag (10x restored, not 1x)")
}

// waitFaultedAndPastDeadline waits until target is faulted (QAP drops to want) and the current deadline has passed the target's.
func waitFaultedAndPastDeadline(ctx context.Context, t *testing.T, client *kit.TestFullNode, maddr address.Address, target abi.SectorNumber, targetDeadline uint64, want uint64, maxWait time.Duration) bool {
	t.Helper()
	endBy := time.Now().Add(maxWait)
	for {
		pw, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		if pw.MinerPower.QualityAdjPower.Uint64() == want {
			di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
			require.NoError(t, err)
			if di.Index > targetDeadline {
				return true
			}
		}
		if time.Now().After(endBy) {
			require.FailNowf(t, "fault wait timeout",
				"target %d never became faulted with the deadline past %d", target, targetDeadline)
		}
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)
		client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+40))
	}
}

// TestMigrationNV29SolsticeFaultRecoverUsqdFullPower drives fault→recover for a USQ'd legacy sector: fault removes its 10x, recovery restores FULL_QA flag and full 10x.
func TestMigrationNV29SolsticeFaultRecoverUsqdFullPower(t *testing.T) {
	kit.QuietMiningLogs()

	oldVal := wdpost.RecoveringSectorLimit
	defer func() { wdpost.RecoveringSectorLimit = oldVal }()
	wdpost.RecoveringSectorLimit = 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		ssz          = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch = abi.ChainEpoch(2000)
	)
	blocktime := 2 * time.Millisecond

	client, m, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version28, Height: -1},
			stmgr.Upgrade{
				Network:   network.Version29,
				Height:    upgradeEpoch,
				Migration: filcns.UpgradeActorsV19With(buildconstants.NeutralSolsticeRewardBootstrapParams),
			},
		),
	)
	ens.InterconnectAll().BeginMining(blocktime)

	maddr, err := m.ActorAddress(ctx)
	require.NoError(t, err)
	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	m.PledgeSectors(ctx, 1, 0, nil)
	sectors, err := m.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sectors, 1)
	target := sectors[0]

	// Wait for raw power to stabilize (preseals + our 1 sector all active) before reading QAP.
	rawWant := uint64(kit.DefaultPresealsPerBootstrapMiner+1) * uint64(ssz)
	waitRaw := func() {
		end := time.Now().Add(4 * time.Minute)
		for {
			pw, perr := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
			require.NoError(t, perr)
			if pw.MinerPower.RawBytePower.Uint64() == rawWant {
				return
			}
			if time.Now().After(end) {
				require.FailNowf(t, "raw power wait timeout",
					"miner raw power did not reach %d in time; last=%d", rawWant, pw.MinerPower.RawBytePower.Uint64())
			}
			h, herr := client.ChainHead(ctx)
			require.NoError(t, herr)
			client.WaitTillChain(ctx, kit.HeightAtLeast(h.Height()+40))
		}
	}
	waitRaw()

	lInfo, err := client.StateSectorGetInfo(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	require.NotNil(t, lInfo)
	require.Less(t, lInfo.Activation, upgradeEpoch, "the pledged sector must activate pre-upgrade (legacy 1x)")
	require.Zero(t, lInfo.Flags&miner.FULL_QA_POWER, "the pre-upgrade sector must start at 1x without FULL_QA_POWER")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	require.NoError(t, err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	require.NoError(t, err)
	require.Equal(t, network.Version29, nv, "chain must actually be on NV29 after the migration")

	qapBase, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	baseQA := qapBase.MinerPower.QualityAdjPower.Uint64()

	loc, lerr := client.StateSectorPartition(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, lerr)
	usqEnc, serr := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
		Upgrades: []stminer.UpgradeSectorQuality{{
			Deadline:  loc.Deadline,
			Partition: loc.Partition,
			Sectors:   bitfield.NewFromSet([]uint64{uint64(target)}),
		}},
	})
	require.NoError(t, serr)
	usqMsg, merr := client.MpoolPushMessage(ctx, &types.Message{
		From:   m.OwnerKey.Address,
		To:     maddr,
		Method: builtin.MethodsMiner.UpgradeSectorQuality,
		Params: usqEnc,
		Value:  big.Zero(),
	}, nil)
	require.NoError(t, merr)
	_, werr := client.StateWaitMsg(ctx, usqMsg.Cid(), 2, lapi.LookbackNoLimit, true)
	require.NoError(t, werr, "USQ must be confirmed")

	kit.WaitForMinerQAP(ctx, t, client, maddr, baseQA+uint64(ssz)*9, 3*time.Minute)
	fullInfo, err := client.StateSectorGetInfo(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	require.NotZero(t, fullInfo.Flags&miner.FULL_QA_POWER, "the USQ'd sector must carry FULL_QA_POWER (10x)")
	qapUsqd, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	usqdQA := qapUsqd.MinerPower.QualityAdjPower.Uint64()

	spart, err := client.StateSectorPartition(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	targetDeadline := spart.Deadline

	markFailed := func(failed bool) {
		require.NoError(t, m.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(
			storiface.SectorRef{ID: abi.SectorID{Miner: abi.ActorID(mid), Number: target}}, failed))
	}
	markFailed(true)

	faulted := waitFaultedAndPastDeadline(ctx, t, client, maddr, target, targetDeadline, baseQA-uint64(ssz), 4*time.Minute)
	require.True(t, faulted, "the USQ'd sector %d must be declared faulty", target)

	markFailed(false)
	_, err = m.RecoverFault(ctx, []abi.SectorNumber{target})
	require.NoError(t, err, "RecoverFault must be accepted")

	head, err = client.ChainHead(ctx)
	require.NoError(t, err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+10))

	recs, err := client.StateMinerRecoveries(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	isRecovering, err := recs.IsSet(uint64(target))
	require.NoError(t, err)
	require.True(t, isRecovering, "the USQ'd 10x sector must be recorded as recovering")

	kit.WaitForMinerQAP(ctx, t, client, maddr, usqdQA, 3*time.Minute)

	faultsAfter, err := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	isFaultedAfter, err := faultsAfter.IsSet(uint64(target))
	require.NoError(t, err)
	require.False(t, isFaultedAfter, "the recovered USQ'd sector must leave the fault set")

	info, err := client.StateSectorGetInfo(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	require.NotZero(t, info.Flags&miner.FULL_QA_POWER,
		"a recovered USQ'd sector must keep its FULL_QA_POWER flag (full 10x restored, not a 1x residue)")
}
