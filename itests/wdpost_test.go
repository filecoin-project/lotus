// stm: #integration
package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func TestWindowedPost(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	kit.Expensive(t)

	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		nSectors  = 10
	)

	for _, height := range []abi.ChainEpoch{
		-1,   // before
		162,  // while sealing
		5000, // while proving
	} {
		height := height // copy to satisfy lints
		t.Run(fmt.Sprintf("upgrade-%d", height), func(t *testing.T) {
			testWindowPostUpgrade(t, blocktime, nSectors, height)
		})
	}
}

func testWindowPostUpgrade(t *testing.T, blocktime time.Duration, nSectors int, upgradeHeight abi.ChainEpoch) {

	/// XXX  TEMPORARILY DISABLED UNTIL NV18 MIGRATION IS IMPLEMENTED
	t.Skip("temporarily disabled as nv18 migration is not yet implemented")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.LatestActorsAt(upgradeHeight))
	ens.InterconnectAll().BeginMining(blocktime)

	miner.PledgeSectors(ctx, nSectors, 0, nil)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_MINER_CALCULATE_DEADLINE_001
	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	t.Log("Running one proving period")
	waitUntil := di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	//stm: @CHAIN_STATE_MINER_POWER_001
	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors+kit.DefaultPresealsPerBootstrapMiner)))

	t.Log("Drop some sectors")

	// Drop 2 sectors from deadline 2 partition 0 (full partition / deadline)
	{
		//stm: @CHAIN_STATE_MINER_GET_PARTITIONS_001
		parts, err := client.StateMinerPartitions(ctx, maddr, 2, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		secs := parts[0].AllSectors
		n, err := secs.Count()
		require.NoError(t, err)
		require.Equal(t, uint64(2), n)

		// Drop the partition
		err = secs.ForEach(func(sid uint64) error {
			return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkCorrupted(storiface.SectorRef{
				ID: abi.SectorID{
					Miner:  abi.ActorID(mid),
					Number: abi.SectorNumber(sid),
				},
			}, true)
		})
		require.NoError(t, err)
	}

	var s storiface.SectorRef

	// Drop 1 sectors from deadline 3 partition 0
	{
		//stm: @CHAIN_STATE_MINER_GET_PARTITIONS_001
		parts, err := client.StateMinerPartitions(ctx, maddr, 3, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		secs := parts[0].AllSectors
		n, err := secs.Count()
		require.NoError(t, err)
		require.Equal(t, uint64(2), n)

		// Drop the sector
		sn, err := secs.First()
		require.NoError(t, err)

		all, err := secs.All(2)
		require.NoError(t, err)
		t.Log("the sectors", all)

		s = storiface.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sn),
			},
		}

		err = miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(s, true)
		require.NoError(t, err)
	}

	//stm: @CHAIN_STATE_MINER_CALCULATE_DEADLINE_001
	di, err = client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	t.Log("Go through another PP, wait for sectors to become faulty")
	waitUntil = di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	//stm: @CHAIN_STATE_MINER_POWER_001
	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors := p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-3, int(sectors)) // -3 just removed sectors

	t.Log("Recover one sector")

	err = miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(s, false)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_MINER_CALCULATE_DEADLINE_001
	di, err = client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	waitUntil = di.Open + di.WPoStProvingPeriod
	t.Logf("End for head.Height > %d", waitUntil)

	ts = client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	//stm: @CHAIN_STATE_MINER_POWER_001
	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors = p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-2, int(sectors)) // -2 not recovered sectors

	// pledge a sector after recovery

	miner.PledgeSectors(ctx, 1, nSectors, nil)

	{
		// Wait until proven.
		//stm: @CHAIN_STATE_MINER_CALCULATE_DEADLINE_001
		di, err = client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)

		waitUntil := di.Open + di.WPoStProvingPeriod
		t.Logf("End for head.Height > %d\n", waitUntil)

		ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
		t.Logf("Now head.Height = %d", ts.Height())
	}

	//stm: @CHAIN_STATE_MINER_POWER_001
	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors = p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+kit.DefaultPresealsPerBootstrapMiner-2+1, int(sectors)) // -2 not recovered sectors + 1 just pledged
}

func TestWindowPostBaseFeeNoBurn(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	kit.Expensive(t)

	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		nSectors  = 10
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	och := build.UpgradeClausHeight
	build.UpgradeClausHeight = 0
	t.Cleanup(func() { build.UpgradeClausHeight = och })

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.GenesisNetworkVersion(network.Version9))
	ens.InterconnectAll().BeginMining(blocktime)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_MINER_INFO_001
	mi, err := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	miner.PledgeSectors(ctx, nSectors, 0, nil)
	//stm: @CHAIN_STATE_GET_ACTOR_001
	wact, err := client.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
	require.NoError(t, err)
	en := wact.Nonce

	// wait for a new message to be sent from worker address, it will be a PoSt

waitForProof:
	for {
		//stm: @CHAIN_STATE_GET_ACTOR_001
		wact, err := client.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
		require.NoError(t, err)
		if wact.Nonce > en {
			break waitForProof
		}

		build.Clock.Sleep(blocktime)
	}

	//stm: @CHAIN_STATE_LIST_MESSAGES_001
	slm, err := client.StateListMessages(ctx, &api.MessageMatch{To: maddr}, types.EmptyTSK, 0)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_REPLAY_001
	pmr, err := client.StateReplay(ctx, types.EmptyTSK, slm[0])
	require.NoError(t, err)

	require.Equal(t, pmr.GasCost.BaseFeeBurn, big.Zero())
}

func TestWindowPostBaseFeeBurn(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	kit.Expensive(t)

	kit.QuietMiningLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blocktime := 2 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(blocktime)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_MINER_INFO_001
	mi, err := client.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	miner.PledgeSectors(ctx, 10, 0, nil)
	//stm: @CHAIN_STATE_GET_ACTOR_001
	wact, err := client.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
	require.NoError(t, err)
	en := wact.Nonce

	// wait for a new message to be sent from worker address, it will be a PoSt

waitForProof:
	for {
		//stm: @CHAIN_STATE_GET_ACTOR_001
		wact, err := client.StateGetActor(ctx, mi.Worker, types.EmptyTSK)
		require.NoError(t, err)
		if wact.Nonce > en {
			break waitForProof
		}

		build.Clock.Sleep(blocktime)
	}

	//stm: @CHAIN_STATE_LIST_MESSAGES_001
	slm, err := client.StateListMessages(ctx, &api.MessageMatch{To: maddr}, types.EmptyTSK, 0)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_REPLAY_001
	pmr, err := client.StateReplay(ctx, types.EmptyTSK, slm[0])
	require.NoError(t, err)

	require.NotEqual(t, pmr.GasCost.BaseFeeBurn, big.Zero())
}
