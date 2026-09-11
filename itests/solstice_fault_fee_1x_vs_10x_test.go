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

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMigrationNV29SolsticeFaultFee1xVs10x proves 10x continued-fault FeeDebt exceeds 1x on real ledger.
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

	// two miners: um1x holds legacy 1x CC (onboarded NV28), um10x holds native 10x CC (onboarded NV29).
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

	legacy, _ := um1x.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um1x.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))
	lInfo, err := client.StateSectorGetInfo(ctx, um1x.ActorAddr, legacy[0], types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER before upgrade")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	native, _ := um10x.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(native, 1)
	um10x.WaitTillActivatedAndAssertPower(native, uint64(defaultSectorSize), uint64(defaultSectorSize)*10)
	nInfo, err := client.StateSectorGetInfo(ctx, um10x.ActorAddr, native[0], types.EmptyTSK)
	req.NoError(err)
	req.GreaterOrEqual(nInfo.Activation, upgradeEpoch, "native sector must activate on NV29 (10x)")
	req.NotZero(nInfo.Flags&miner.FULL_QA_POWER, "native NV29 CC sector must carry FULL_QA_POWER (10x)")

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

	// drain available balance to ~0 so fault penalties accumulate as FeeDebt.
	drainWithdrawBalance(um1x.ActorAddr, "legacy-1x")
	drainWithdrawBalance(um10x.ActorAddr, "native-10x")

	um1x.DeclareFaults([]abi.SectorNumber{legacy[0]})
	um10x.DeclareFaults([]abi.SectorNumber{native[0]})

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
