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
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
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

// waitUsqdFaultedAndPastDeadline waits until target is faulted (QAP drops to want) and the current deadline has passed the target's.
func waitUsqdFaultedAndPastDeadline(ctx context.Context, t *testing.T, client *kit.TestFullNode, maddr address.Address, target abi.SectorNumber, targetDeadline uint64, want uint64, maxWait time.Duration) bool {
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

	faulted := waitUsqdFaultedAndPastDeadline(ctx, t, client, maddr, target, targetDeadline, baseQA-uint64(ssz), 4*time.Minute)
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
