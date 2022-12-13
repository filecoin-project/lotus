// stm: #integration
package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/storage/sealer/mock"
)

// TestDeadlineToggling:
// * spins up a v3 network (miner A)
// * creates an inactive miner (miner B)
// * creates another miner, pledges a sector, waits for power (miner C)
//
// * goes through v4 upgrade
// * goes through PP
// * creates minerD, minerE
// * makes sure that miner B/D are inactive, A/C still are
// * pledges sectors on miner B/D
// * precommits a sector on minerE
// * disables post on miner C
// * goes through PP 0.5PP
// * asserts that minerE is active
// * goes through rest of PP (1.5)
// * asserts that miner C loses power
// * asserts that miner B/D is active and has power
// * asserts that minerE is inactive
// * disables post on miner B
// * terminates sectors on miner D
// * goes through another PP
// * asserts that miner B loses power
// * asserts that miner D loses power, is inactive
func TestDeadlineToggling(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @MINER_SECTOR_LIST_001
	kit.Expensive(t)

	kit.QuietMiningLogs()

	const sectorsC, sectorsD, sectorsB = 10, 9, 8

	var (
		upgradeH      abi.ChainEpoch = 4000
		provingPeriod abi.ChainEpoch = 2880
		blocktime                    = 2 * time.Millisecond
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		client kit.TestFullNode
		minerA kit.TestMiner
		minerB kit.TestMiner
		minerC kit.TestMiner
		minerD kit.TestMiner
		minerE kit.TestMiner
	)
	opts := []kit.NodeOpt{kit.WithAllSubsystems()}
	ens := kit.NewEnsemble(t, kit.MockProofs(), kit.TurboUpgradeAt(upgradeH)).
		FullNode(&client, opts...).
		Miner(&minerA, &client, opts...).
		Start().
		InterconnectAll()
	ens.BeginMining(blocktime)

	opts = append(opts, kit.OwnerAddr(client.DefaultKey))
	ens.Miner(&minerB, &client, opts...).
		Miner(&minerC, &client, opts...).
		Start()

	defaultFrom, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	maddrA, err := minerA.ActorAddress(ctx)
	require.NoError(t, err)

	build.Clock.Sleep(time.Second)

	maddrB, err := minerB.ActorAddress(ctx)
	require.NoError(t, err)
	maddrC, err := minerC.ActorAddress(ctx)
	require.NoError(t, err)

	ssz, err := minerC.ActorSectorSize(ctx, maddrC)
	require.NoError(t, err)

	// pledge sectors on C, go through a PP, check for power
	{
		minerC.PledgeSectors(ctx, sectorsC, 0, nil)

		//stm: @CHAIN_STATE_MINER_CALCULATE_DEADLINE_001
		di, err := client.StateMinerProvingDeadline(ctx, maddrC, types.EmptyTSK)
		require.NoError(t, err)

		t.Log("Running one proving period (miner C)")
		t.Logf("End for head.Height > %d", di.PeriodStart+di.WPoStProvingPeriod*2)

		for {
			head, err := client.ChainHead(ctx)
			require.NoError(t, err)

			if head.Height() > di.PeriodStart+provingPeriod*2 {
				t.Logf("Now head.Height = %d", head.Height())
				break
			}
			build.Clock.Sleep(blocktime)
		}

		expectedPower := types.NewInt(uint64(ssz) * sectorsC)

		//stm: @CHAIN_STATE_MINER_POWER_001
		p, err := client.StateMinerPower(ctx, maddrC, types.EmptyTSK)
		require.NoError(t, err)

		// make sure it has gained power.
		require.Equal(t, p.MinerPower.RawBytePower, expectedPower)
	}

	// go through upgrade + PP
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > upgradeH+provingPeriod {
			t.Logf("Now head.Height = %d", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	checkMiner := func(ma address.Address, power abi.StoragePower, active bool, tsk types.TipSetKey) {
		//stm: @CHAIN_STATE_MINER_POWER_001
		p, err := client.StateMinerPower(ctx, ma, tsk)
		require.NoError(t, err)

		// make sure it has the expected power.
		require.Equal(t, p.MinerPower.RawBytePower, power)

		//stm: @CHAIN_STATE_GET_ACTOR_001
		mact, err := client.StateGetActor(ctx, ma, tsk)
		require.NoError(t, err)

		mst, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client))), mact)
		require.NoError(t, err)

		act, err := mst.DeadlineCronActive()
		require.NoError(t, err)

		require.Equal(t, active, act)
	}

	// check that just after the upgrade minerB was still active
	{
		uts, err := client.ChainGetTipSetByHeight(ctx, upgradeH+2, types.EmptyTSK)
		require.NoError(t, err)
		checkMiner(maddrB, types.NewInt(0), true, uts.Key())
	}

	//stm: @CHAIN_STATE_NETWORK_VERSION_001
	nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.GreaterOrEqual(t, nv, network.Version12)

	ens.Miner(&minerD, &client, opts...).
		Miner(&minerE, &client, opts...).
		Start()

	maddrD, err := minerD.ActorAddress(ctx)
	require.NoError(t, err)
	maddrE, err := minerE.ActorAddress(ctx)
	require.NoError(t, err)

	// first round of miner checks
	checkMiner(maddrA, types.NewInt(uint64(ssz)*kit.DefaultPresealsPerBootstrapMiner), true, types.EmptyTSK)
	checkMiner(maddrC, types.NewInt(uint64(ssz)*sectorsC), true, types.EmptyTSK)

	checkMiner(maddrB, types.NewInt(0), false, types.EmptyTSK)
	checkMiner(maddrD, types.NewInt(0), false, types.EmptyTSK)
	checkMiner(maddrE, types.NewInt(0), false, types.EmptyTSK)

	// pledge sectors on minerB/minerD, stop post on minerC
	minerB.PledgeSectors(ctx, sectorsB, 0, nil)
	checkMiner(maddrB, types.NewInt(0), true, types.EmptyTSK)

	minerD.PledgeSectors(ctx, sectorsD, 0, nil)
	checkMiner(maddrD, types.NewInt(0), true, types.EmptyTSK)

	minerC.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).Fail()

	// precommit a sector on minerE
	{
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		cr, err := cid.Parse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")
		require.NoError(t, err)

		params := &minertypes.SectorPreCommitInfo{
			Expiration:   2880 * 300,
			SectorNumber: 22,
			SealProof:    kit.TestSpt,

			SealedCID:     cr,
			SealRandEpoch: head.Height() - 200,
		}

		enc := new(bytes.Buffer)
		require.NoError(t, params.MarshalCBOR(enc))

		m, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     maddrE,
			From:   defaultFrom,
			Value:  types.FromFil(1),
			Method: builtin.MethodsMiner.PreCommitSector,
			Params: enc.Bytes(),
		}, nil)
		require.NoError(t, err)

		//stm: @CHAIN_STATE_WAIT_MSG_001
		r, err := client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)
	}

	// go through 0.5 PP
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > upgradeH+provingPeriod+(provingPeriod/2) {
			t.Logf("Now head.Height = %d", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	checkMiner(maddrE, types.NewInt(0), true, types.EmptyTSK)

	// go through rest of the PP
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > upgradeH+(provingPeriod*3) {
			t.Logf("Now head.Height = %d", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	// second round of miner checks
	checkMiner(maddrA, types.NewInt(uint64(ssz)*kit.DefaultPresealsPerBootstrapMiner), true, types.EmptyTSK)
	checkMiner(maddrC, types.NewInt(0), true, types.EmptyTSK)
	checkMiner(maddrB, types.NewInt(uint64(ssz)*sectorsB), true, types.EmptyTSK)
	checkMiner(maddrD, types.NewInt(uint64(ssz)*sectorsD), true, types.EmptyTSK)
	checkMiner(maddrE, types.NewInt(0), false, types.EmptyTSK)

	// disable post on minerB
	minerB.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).Fail()

	// terminate sectors on minerD
	{
		var terminationDeclarationParams []miner2.TerminationDeclaration
		secs, err := minerD.SectorsListNonGenesis(ctx)
		require.NoError(t, err)
		require.Len(t, secs, sectorsD)

		for _, sectorNum := range secs {
			sectorbit := bitfield.New()
			sectorbit.Set(uint64(sectorNum))

			//stm: @CHAIN_STATE_SECTOR_PARTITION_001
			loca, err := client.StateSectorPartition(ctx, maddrD, sectorNum, types.EmptyTSK)
			require.NoError(t, err)

			para := miner2.TerminationDeclaration{
				Deadline:  loca.Deadline,
				Partition: loca.Partition,
				Sectors:   sectorbit,
			}

			terminationDeclarationParams = append(terminationDeclarationParams, para)
		}

		terminateSectorParams := &miner2.TerminateSectorsParams{
			Terminations: terminationDeclarationParams,
		}

		sp, aerr := actors.SerializeParams(terminateSectorParams)
		require.NoError(t, aerr)

		smsg, err := client.MpoolPushMessage(ctx, &types.Message{
			From:   defaultFrom,
			To:     maddrD,
			Method: builtin.MethodsMiner.TerminateSectors,

			Value:  big.Zero(),
			Params: sp,
		}, nil)
		require.NoError(t, err)

		t.Log("sent termination message:", smsg.Cid())

		//stm: @CHAIN_STATE_WAIT_MSG_001
		r, err := client.StateWaitMsg(ctx, smsg.Cid(), 2, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

		// assert miner has no power
		p, err := client.StateMinerPower(ctx, maddrD, r.TipSet)
		require.NoError(t, err)
		require.True(t, p.MinerPower.RawBytePower.IsZero())
	}

	// go through another PP
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > upgradeH+(provingPeriod*5) {
			t.Logf("Now head.Height = %d", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	checkMiner(maddrA, types.NewInt(uint64(ssz)*kit.DefaultPresealsPerBootstrapMiner), true, types.EmptyTSK)
	checkMiner(maddrC, types.NewInt(0), true, types.EmptyTSK)
	checkMiner(maddrB, types.NewInt(0), true, types.EmptyTSK)
	checkMiner(maddrD, types.NewInt(0), false, types.EmptyTSK)
}
