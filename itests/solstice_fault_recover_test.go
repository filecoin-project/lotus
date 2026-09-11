package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/filecoin-project/lotus/storage/wdpost"

	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
)

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
