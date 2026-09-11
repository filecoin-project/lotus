package itests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestMigrationNV29SolsticeUsqdSectorFault verifies USQ'd 10x sector faults correctly and USQ is rejected on it.
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

	legacy, _ := um.OnboardSectors(sealProofType, kit.NewSectorBatch().AddEmptySectors(1))
	req.Len(legacy, 1)
	um.WaitTillActivatedAndAssertPower(legacy, uint64(defaultSectorSize), uint64(defaultSectorSize))

	sn := legacy[0]
	lInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.Less(lInfo.Activation, upgradeEpoch, "legacy sector must activate pre-upgrade (1x)")
	req.Zero(lInfo.Flags&miner.FULL_QA_POWER, "legacy sector must not carry FULL_QA_POWER pre-upgrade")

	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	head, err := client.ChainHead(ctx)
	req.NoError(err)
	nv, err := client.StateNetworkVersion(ctx, head.Key())
	req.NoError(err)
	req.Equal(network.Version29, nv, "chain must actually be on NV29 after the migration")

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.NoError(err, "USQ of a legacy CC sector must succeed")
	uInfo, err := client.StateSectorGetInfo(ctx, maddr, sn, types.EmptyTSK)
	req.NoError(err)
	req.NotZero(uInfo.Flags&miner.FULL_QA_POWER, "USQ'd sector must carry FULL_QA_POWER (10x)")
	power, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.Equal(uint64(defaultSectorSize)*10, power.MinerPower.QualityAdjPower.Uint64(),
		"USQ'd-to-10x sector must be the miner's sole 10x power")

	um.DeclareFaults([]abi.SectorNumber{sn})

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	client.WaitTillChain(ctx, kit.HeightAtLeast(di.Open+di.WPoStProvingPeriod+1))

	faults, err := client.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isFaulted, err := faults.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isFaulted, "USQ'd sector %d must be faulted after a proving period", sn)

	fpower, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	req.True(fpower.MinerPower.QualityAdjPower.IsZero(),
		"faulting the sole USQ'd-to-10x sector must remove all QAP (full 10x tier, not a 1x residue); got %s",
		fpower.MinerPower.QualityAdjPower)

	_, err = um.UpgradeSectorQuality([]abi.SectorNumber{sn}, nil)
	req.Error(err, "USQ on a faulted USQ'd sector must be rejected")
	req.Contains(err.Error(), "not active", "USQ on a faulted USQ'd sector must fail with 'sector is not active'")

	um.RecoverFaults([]abi.SectorNumber{sn})

	recs, err := client.StateMinerRecoveries(ctx, maddr, types.EmptyTSK)
	req.NoError(err)
	isRecovering, err := recs.IsSet(uint64(sn))
	req.NoError(err)
	req.True(isRecovering, "a DeclareFaultsRecovered on the USQ'd sector must be accepted and recorded")

	um.AssertNoWindowPostError()
}
