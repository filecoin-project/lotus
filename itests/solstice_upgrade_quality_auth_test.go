package itests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
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
