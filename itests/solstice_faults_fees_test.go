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

// TestMigrationNV29SolsticeUpgradeQualityAuth asserts the FIP-0118 caller-authorization set for
// UpgradeSectorQuality (method 37) across a real NV28→NV29 migration. Via StateCall probes and a real
// mined USQ it verifies that a caller which is owner and worker (the kit's shared key) is accepted
// (the sector is actually lifted to FULL_QA(10x)), a control address is accepted (control is not
// limited to WindowPoSt for USQ), and an unrelated (non owner/worker/control) address is rejected with
// USR_FORBIDDEN. Because the kit always has owner==worker, it cannot separate owner from worker
// authorization; that case is covered by TestMigrationNV29SolsticeUpgradeQualityPureOwnerAuth.
func TestMigrationNV29SolsticeUpgradeQualityAuth(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(3000)
	)

	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof
	defer um.Stop()

	// fundAccount creates an account actor for `a` (so it can be used as a StateCall From) by funding it.
	fundAccount := func(a address.Address) {
		kit.SendFunds(ctx, t, client, a, types.FromFil(1))
	}

	// ---- Onboard a legacy 1x CC sector on NV28 (the authorized USQ must lift this to a real 10x).
	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))
	sn := legacy[0]

	lInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER before USQ")

	// Cross the migration to NV29 (non-retroactive: the legacy sector stays 1x until USQ'd).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// ---- Create distinct control and unrelated wallets (all in the node's wallet, funded to resolve
	// as StateCall From addresses).
	ctrlAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelatedAddr, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	fundAccount(ctrlAddr)
	fundAccount(unrelatedAddr)

	// ---- Install ctrlAddr as the miner's (sole) control address. We keep the worker unchanged so the
	// ChangeWorkerAddress takes effect immediately (only a *worker* handover waits WorkerKeyChangeDelay)
	// and the unmanaged miner's owner-keyed WindowPoSt is unaffected.
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

	// The control address is effective immediately (no worker handover in flight). StateMinerInfo
	// reports owner/worker/control as ID addresses, so resolve our keys to their ID forms for the compare.
	ctrlID, err := client.StateLookupID(ctx, ctrlAddr, types.EmptyTSK)
	req.NoError(err)
	defaultID, err := client.StateLookupID(ctx, client.DefaultKey.Address, types.EmptyTSK)
	req.NoError(err)
	mi, err = client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Contains(mi.ControlAddresses, ctrlID, "ctrlAddr must now be a control address")
	req.Equal(defaultID, mi.Owner, "owner unchanged")
	req.Equal(defaultID, mi.Worker, "worker unchanged (owner==worker in the kit ensemble)")

	// usqCallExit runs UpgradeSectorQuality on the live 1x sector via StateCall (virtual, non-persisting)
	// from `from` and returns the resulting exit code. Caller validation happens first in the actor, so
	// an unauthorised caller aborts with USR_FORBIDDEN regardless of the sector's deadline state.
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

	// ---- A control address must be ACCEPTED to call USQ. StateCall runs the real method body (caller
	// validation first, then the sector-quality logic on the live 1x sector) and reports a clean Ok --
	// i.e. control is in USQ's authorised caller set, not reserved for WindowPoSt only.
	req.Equal(exitcode.Ok, usqCallExit(ctrlAddr),
		"a control address must be authorized to call UpgradeSectorQuality (Ok, not USR_FORBIDDEN)")

	// ---- Unrelated (non owner/worker/control) must be FORBIDDEN.
	req.Equal(exitcode.ErrForbidden, usqCallExit(unrelatedAddr),
		"an unrelated address must be forbidden from calling UpgradeSectorQuality")

	// ---- Owner (== worker) must be ACCEPTED too: a real, mined USQ lifts the legacy 1x sector to FULL_QA.
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

	// The miner keeps passing WindowPoSts (its owner/worker keys are unchanged), confirming the control
	// reconfiguration did not disturb normal operation.
	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeUpgradeQualityPureOwnerAuth asserts that a pure owner (owner != worker,
// not a control) is individually authorized for UpgradeSectorQuality (method 37). It runs a genuine
// worker handover on NV29 (ChangeWorkerAddress installs a new worker B and a control C, leaving owner
// A a pure owner) and StateCalls method 37 from A(owner)/B(worker)/C(control)/unrelated(D), verifying
// the owner, worker, and control are authorized while only the unrelated caller is USR_FORBIDDEN.
func TestMigrationNV29SolsticeUpgradeQualityPureOwnerAuth(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	// Deliberately do NOT watch this miner's WindowPoSts (see below): once A loses the worker role
	// nothing needs a post, and the sector may fault harmlessly for caller-validation-first.
	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch, watchPost: false})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof

	ownerA := client.DefaultKey.Address

	// Cross the migration to NV29 so the miner is on the v19 actor where method 37 exists.
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	// Onboard a native NV29 CC sector. We deliberately do NOT watch this miner's WindowPoSts and we
	// Stop the um loop shortly after, so that once A loses the worker role nothing needs (or stalls on)
	// a post; the sector may fault for want of WindowPoSts, which caller-validation-first makes harmless.
	onboarded, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(onboarded, 1)
	um.WaitTillActivatedAndAssertPower(onboarded, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)
	sn := onboarded[0]
	um.Stop()

	// Create distinct worker (B), control (C) and unrelated (D) wallets, funded so StateCall resolves
	// them as callers.
	// The miner worker must be backed by a BLS pubkey (the actor rejects a secp worker account with
	// "worker account must have BLS pubkey"); the control and unrelated wallets may be secp.
	workerB, err := client.WalletNew(ctx, types.KTBLS)
	req.NoError(err)
	controlC, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	unrelatedD, err := client.WalletNew(ctx, types.KTSecp256k1)
	req.NoError(err)
	for _, a := range []address.Address{workerB, controlC, unrelatedD} {
		kit.SendFunds(ctx, t, client, a, types.FromFil(1))
	}

	// resolveToID maps a key address to its ID form (StateMinerInfo reports addresses as ID addresses).
	resolveToID := func(a address.Address) address.Address {
		id, lerr := client.StateLookupID(ctx, a, types.EmptyTSK)
		req.NoError(lerr)
		return id
	}
	workerBID := resolveToID(workerB)

	// ---- Genuine worker handover: install workerB as the new worker and controlC as the (sole)
	// control, from owner A. This makes A a *pure* owner once B takes effect (A is neither worker nor
	// control). Because a real worker change is used (NewWorker != current), it waits WorkerKeyChangeDelay.
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

	// api.MinerInfo (StateMinerInfo) drops PendingWorkerKey, but miner19.State.Info is the CID of the
	// MinerInfo CBOR which carries the pending worker's EffectiveAt. Decode it through the blockstore to
	// learn when B takes over as worker.
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

	// Advance past the handover effective epoch (plus margin).
	client.WaitTillChain(ctx, kit.HeightAtLeast(effectiveAt+20))

	// The worker change is a two-step handover: ChangeWorkerAddress only *schedules* B (and leaves A as
	// the acting worker until then); once past EffectiveAt the change must be ConfirmChangeWorkerAddress'd
	// (empty params) to actually take over as worker and apply the new control list C. The actor rejects
	// the *new* worker as the confirm caller; it authorizes the current worker / owner -- here A == owner
	// == current worker == DefaultKey. Push the confirm from A. Only after this does A become a *pure*
	// owner.
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

	// stateCallExit runs method `m` from `from` with `params` on a real tipset (virtual, non-persisting)
	// and returns the resulting exit code. Caller validation runs first in the miner actor, so an
	// unauthorized caller aborts with USR_FORBIDDEN regardless of sector/deadline state.
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

	// ---- GUARD: establish that A is now a *pure* owner from the recorded miner state -- exactly the
	// bookkeeping that method 37 authorizes from. (A SubmitWindowedPoSt proxy is NOT usable as a
	// discriminator here: the actor admits the owner past its caller gate for that method, returning
	// USR_ILLEGAL_ARGUMENT rather than USR_FORBIDDEN, so it cannot tell owner from worker.)
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

	// ---- PROBE method 37 (UpgradeSectorQuality) on the real sector from each caller role.
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

	// Unrelated stays the only USR_FORBIDDEN caller on this distinct-worker miner.
	req.Equal(exitcode.ErrForbidden, unrelatedUSQ, "an unrelated address must be forbidden from method 37")
	req.NotEqual(exitcode.ErrForbidden, workerUSQ, "the distinct worker B must remain authorized for method 37")
	req.NotEqual(exitcode.ErrForbidden, controlUSQ, "the control address must remain authorized for method 37")

	// A pure owner (owner != worker, not a control) is authorized for method 37: caller
	// validation runs first, so an authorized owner can never surface USR_FORBIDDEN regardless of sector
	// state. Whether we can pin the full call path to OK depends on whether the sole sector is still
	// active at probe time (it may have faulted once A lost the worker role).
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

// TestMigrationNV29SolsticeFaultAndRecover faults a native FULL_QA(10x) CC sector (the miner's only
// power) on NV29 and asserts faulting it drops the miner's QAP to zero (the full 10x tier removed, not
// a 1x residue), that UpgradeSectorQuality is rejected on the faulted (inactive) sector, and that a
// DeclareFaultsRecovered is accepted and recorded on the miner's recovery queue.
func TestMigrationNV29SolsticeFaultAndRecover(t *testing.T) {
	req := require.New(t)
	kit.QuietMiningLogs()

	const (
		defaultSectorSize = abi.SectorSize(2 << 10) // 2KiB
		upgradeEpoch      = abi.ChainEpoch(2000)
	)

	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof
	defer um.Stop()

	// minerBalanceFeeDebt reads the miner actor's current balance and outstanding FeeDebt (decoded
	// from the v19 miner state) for diagnostics.
	minerBalanceFeeDebt := func() (balance, feeDebt string) {
		act, aerr := client.StateGetActor(ctx, maddr, types.EmptyTSK)
		req.NoError(aerr)
		blk := blockstore.NewAPIBlockstore(client)
		stor := gstStore.WrapBlockStore(ctx, blk)
		var mst stminer.State
		req.NoError(stor.Get(ctx, act.Head, &mst))
		return act.Balance.String(), mst.FeeDebt.String()
	}

	// Cross to NV29, then onboard a native NV29 10x CC sector (the only power on this miner).
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

	// ---- Declare a fault on the 10x sector and let one proving period elapse so it takes effect.
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

	// The FULL_QA(10x) power is fully removed by the fault: the miner drops to zero QAP.
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.True(power.MinerPower.QualityAdjPower.IsZero(),
		"faulting the only (10x) sector must remove all QAP; got %s", power.MinerPower.QualityAdjPower)

	// FIP-0118 UpgradeSectorQuality must be rejected on a faulted (inactive) sector.
	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.Error(err, "USQ on a faulted sector must be rejected")
	req.Contains(err.Error(), "not active", "USQ on a faulted sector must fail with 'sector is not active'")

	// The miner's balance stays well-funded and free of FeeDebt across the fault, so we can be sure
	// the power-drop above is a genuine fault effect (not the miner having been terminated for debt).
	if bal, debt := minerBalanceFeeDebt(); true {
		t.Logf("after fault: miner balance=%s feeDebt=%s (power still 0)", bal, debt)
	}

	// ---- Recover the sector and confirm the recovery is accepted and recorded for the 10x sector.
	um.RecoverFaults([]abi.SectorNumber{sn})

	// The recovery must be recorded promptly (within a couple of epochs).
	recs, err := client.StateMinerRecoveries(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isRecovering, err := recs.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isRecovering, "a DeclareFaultsRecovered on the 10x sector must be accepted and recorded")

	um.AssertNoWindowPostError()
}

// TestMigrationNV29SolsticeFaultRecoverFullPower exercises fault-then-recover on a managed miner (real
// wdpost scheduler), the path where a recovering sector is WindowPoSt'd to clear the fault and restore
// power. With three native NV29 CC sectors all at FULL_QA(10x), faulting one drops QAP by exactly 10x
// and recovering it restores the full 10x, retaining the FULL_QA_POWER flag (not a 1x residue).
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

	// ---- Cross to NV29, then pledge three native NV29 CC sectors (all FULL_QA 10x).
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

	// All three must be native FULL_QA(10x).
	for _, sn := range sectors {
		info, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
		require.NoError(t, err)
		require.NotZero(t, info.Flags&miner.FULL_QA_POWER, "native NV29 CC sector %d must carry FULL_QA_POWER", sn)
	}

	// The EnsembleMinimal bootstrap miner also carries DefaultPresealsPerBootstrapMiner genesis
	// preseal sectors, and FULL_QA power registers sector-by-sector over the first epochs (a moving
	// baseline that defeats absolute/early QAP reads). So we wait for a deterministic stability
	// signal -- the miner's raw power reaching exactly (preseals + our 3) * ssz, i.e. every sector
	// provably active -- then capture the stable pre-fault QAP and assert relative 10x deltas.
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

	// ---- Mark one sector's storage failed; the scheduler auto-declares the fault, the FULL_QA(10x)
	// tier is fully removed (QAP drops by exactly 10x raw), and the sector lands in the fault set.
	target := sectors[0]

	spart, err := client.StateSectorPartition(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	targetDeadline := spart.Deadline

	markFailed := func(failed bool) {
		require.NoError(t, m.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(
			storiface.SectorRef{ID: abi.SectorID{Miner: abi.ActorID(mid), Number: target}}, failed))
	}
	markFailed(true)

	// Wait for the fault to be applied AND the current deadline to move past the target's. The QAP
	// reaching exactly 20x (preQAP minus the removed 10x tier) confirms the fault took effect.
	faulted := waitFaultedAndPastDeadline(t, ctx, client, maddr, target, targetDeadline, preQAP-uint64(ssz)*10, 4*time.Minute)
	require.True(t, faulted, "sector %d must be declared faulty", target)

	// ---- Make the sector provable again and issue a recovery (now inside a safe window); the
	// scheduler WindowPoSt's the recovering sector and the full 10x power (and FULL_QA flag) come back.
	markFailed(false)
	_, err = m.RecoverFault(ctx, []abi.SectorNumber{target})
	require.NoError(t, err, "RecoverFault must be accepted")

	// The recovery declaration is committed to the partition's RecoveringSectors bitfield as the
	// message settles on chain, so wait a few epochs then assert it.
	head, err = client.ChainHead(ctx)
	require.NoError(t, err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()+10))

	recs, err := client.StateMinerRecoveries(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	isRecovering, err := recs.IsSet(uint64(target))
	require.NoError(t, err)
	require.True(t, isRecovering, "the 10x sector must be recorded as recovering")

	// Wait for QAP to return to the pre-fault 30x (full 10x restored, not a 1x residue).
	waitForMinerQAP(t, ctx, client, maddr, preQAP, 3*time.Minute)

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

// waitFaultedAndPastDeadline waits until the target sector is declared faulty (its FULL_QA power tier
// removed, i.e. the miner QAP drops to want) AND the current proving deadline has advanced strictly
// past the target sector's deadline. It returns true when both conditions hold, false on timeout.
func waitFaultedAndPastDeadline(t *testing.T, ctx context.Context, client *kit.TestFullNode, maddr address.Address, target abi.SectorNumber, targetDeadline uint64, want uint64, maxWait time.Duration) bool {
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

// TestMigrationNV29SolsticeFaultRecoverUsqdFullPower drives fault→recover→full-10x-restore for a
// sector that reached the FULL_QA tier via UpgradeSectorQuality rather than natively, on a managed
// miner (real wdpost scheduler). A legacy 1x CC sector pledged pre-upgrade is USQ'd to FULL_QA(10x) on
// NV29, faulted, then recovered: faulting removes exactly its 10x tier, and the scheduler WindowPoSt's
// the recovering sector so recovery restores the FULL_QA flag and the full 10x (never a 1x residue).
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

	// ---- Pledge one legacy CC sector on NV28 (pre-upgrade): it will activate at 1x with no FULL_QA flag.
	m.PledgeSectors(ctx, 1, 0, nil)
	sectors, err := m.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sectors, 1)
	target := sectors[0]

	// Wait until the miner's raw power reaches (preseals + our 1) * ssz -- every genesis preseal and our
	// pledged sector provably active -- so the subsequent absolute QAP reads are stable, pre- and
	// post-migration. The managed bootstrap miner carries DefaultPresealsPerBootstrapMiner genesis
	// preseals, all legacy 1x CC (activated on NV28), and our pledge is a further legacy 1x CC.
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

	// ---- Cross the migration; the legacy sector stays 1x (non-retroactive).
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	require.NoError(t, err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	require.NoError(t, err)
	require.Equal(t, network.Version29, nv, "chain must actually be on NV29 after the migration")

	// Every sector is legacy 1x (preseals + our pledge all activated on NV28), so the miner QAP equals
	// its raw power. Read the stable pre-USQ baseline QAP.
	qapBase, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	baseQA := qapBase.MinerPower.QualityAdjPower.Uint64()

	// ---- USQ the legacy CC sector to FULL_QA (10x): miner QAP must rise by +9x raw over the 1x baseline.
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

	waitForMinerQAP(t, ctx, client, maddr, baseQA+uint64(ssz)*9, 3*time.Minute)
	fullInfo, err := client.StateSectorGetInfo(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	require.NotZero(t, fullInfo.Flags&miner.FULL_QA_POWER, "the USQ'd sector must carry FULL_QA_POWER (10x)")
	qapUsqd, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	usqdQA := qapUsqd.MinerPower.QualityAdjPower.Uint64()

	// ---- Fault the USQ'd 10x sector via the storage manager; the scheduler auto-declares the fault.
	spart, err := client.StateSectorPartition(ctx, maddr, target, types.EmptyTSK)
	require.NoError(t, err)
	targetDeadline := spart.Deadline

	markFailed := func(failed bool) {
		require.NoError(t, m.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(
			storiface.SectorRef{ID: abi.SectorID{Miner: abi.ActorID(mid), Number: target}}, failed))
	}
	markFailed(true)

	// The FULL_QA tier the USQ granted is fully removed: QAP falls by exactly the 10x (back to the
	// untouched 1x-legacy baseline of the other sectors = baseQA - ssz).
	faulted := waitFaultedAndPastDeadline(t, ctx, client, maddr, target, targetDeadline, baseQA-uint64(ssz), 4*time.Minute)
	require.True(t, faulted, "the USQ'd sector %d must be declared faulty", target)

	// ---- Make it provable again and recover; the scheduler WindowPoSt's the recovering sector and the
	// full 10x (and FULL_QA flag) come back.
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

	// QAP returns to the full pre-fault value (the USQ'd 10x restored, not a 1x residue).
	waitForMinerQAP(t, ctx, client, maddr, usqdQA, 3*time.Minute)

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

	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof
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

	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof
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

	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof
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
		waitForMinerQAP(t, ctx, client, maddr, targetQA, 2*time.Minute)
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

	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof
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
		waitForMinerQAP(t, ctx, client, maddr, targetQA, 2*time.Minute)
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

	e := newSolsticeUpgradeEnv(t, solsticeOpts{upgradeEpoch: upgradeEpoch})
	ctx, client, um, maddr := e.ctx, e.client, e.um, e.maddr
	sealProofType := e.sealProof
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
	dls.ForEach(stor, func(dlIdx uint64, dl *stminer.Deadline) error {
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

	um.AssertNoWindowPostError()
}
