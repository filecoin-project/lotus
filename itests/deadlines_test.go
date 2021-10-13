package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/mock"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"
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

	checkMiner := func(ma address.Address, power abi.StoragePower, active, activeIfCron bool, tsk types.TipSetKey) {
		p, err := client.StateMinerPower(ctx, ma, tsk)
		require.NoError(t, err)

		// make sure it has the expected power.
		require.Equal(t, p.MinerPower.RawBytePower, power)

		mact, err := client.StateGetActor(ctx, ma, tsk)
		require.NoError(t, err)

		mst, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client))), mact)
		require.NoError(t, err)

		act, err := mst.DeadlineCronActive()
		require.NoError(t, err)

		if tsk != types.EmptyTSK {
			ts, err := client.ChainGetTipSet(ctx, tsk)
			require.NoError(t, err)
			di, err := mst.DeadlineInfo(ts.Height())
			require.NoError(t, err)

			// cron happened on the same epoch some other condition would have happened
			if di.Open == ts.Height() {
				act, err := mst.DeadlineCronActive()
				require.NoError(t, err)
				require.Equal(t, activeIfCron, act)
				return
			}
		}

		require.Equal(t, active, act)
	}

	// check that just after the upgrade minerB was still active
	{
		uts, err := client.ChainGetTipSetByHeight(ctx, upgradeH+2, types.EmptyTSK)
		require.NoError(t, err)
		checkMiner(maddrB, types.NewInt(0), true, true, uts.Key())
	}

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
	checkMiner(maddrA, types.NewInt(uint64(ssz)*kit.DefaultPresealsPerBootstrapMiner), true, true, types.EmptyTSK)
	checkMiner(maddrC, types.NewInt(uint64(ssz)*sectorsC), true, true, types.EmptyTSK)

	checkMiner(maddrB, types.NewInt(0), false, false, types.EmptyTSK)
	checkMiner(maddrD, types.NewInt(0), false, false, types.EmptyTSK)
	checkMiner(maddrE, types.NewInt(0), false, false, types.EmptyTSK)

	// pledge sectors on minerB/minerD, stop post on minerC
	minerB.PledgeSectors(ctx, sectorsB, 0, nil)
	checkMiner(maddrB, types.NewInt(0), true, true, types.EmptyTSK)

	minerD.PledgeSectors(ctx, sectorsD, 0, nil)
	checkMiner(maddrD, types.NewInt(0), true, true, types.EmptyTSK)

	minerC.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).Fail()

	// precommit a sector on minerE
	{
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		cr, err := cid.Parse("bagboea4b5abcatlxechwbp7kjpjguna6r6q7ejrhe6mdp3lf34pmswn27pkkiekz")
		require.NoError(t, err)

		params := &miner.SectorPreCommitInfo{
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
			Method: miner.Methods.PreCommitSector,
			Params: enc.Bytes(),
		}, nil)
		require.NoError(t, err)

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

	checkMiner(maddrE, types.NewInt(0), true, true, types.EmptyTSK)

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
	checkMiner(maddrA, types.NewInt(uint64(ssz)*kit.DefaultPresealsPerBootstrapMiner), true, true, types.EmptyTSK)
	checkMiner(maddrC, types.NewInt(0), true, true, types.EmptyTSK)
	checkMiner(maddrB, types.NewInt(uint64(ssz)*sectorsB), true, true, types.EmptyTSK)
	checkMiner(maddrD, types.NewInt(uint64(ssz)*sectorsD), true, true, types.EmptyTSK)
	checkMiner(maddrE, types.NewInt(0), false, false, types.EmptyTSK)

	// disable post on minerB
	minerB.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).Fail()

	// terminate sectors on minerD
	{
		var terminationDeclarationParams []miner2.TerminationDeclaration
		secs, err := minerD.SectorsList(ctx)
		require.NoError(t, err)
		require.Len(t, secs, sectorsD)

		for _, sectorNum := range secs {
			sectorbit := bitfield.New()
			sectorbit.Set(uint64(sectorNum))

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
			Method: miner.Methods.TerminateSectors,

			Value:  big.Zero(),
			Params: sp,
		}, nil)
		require.NoError(t, err)

		t.Log("sent termination message:", smsg.Cid())

		r, err := client.StateWaitMsg(ctx, smsg.Cid(), 2, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

		// assert inactive if the message landed in the tipset we run cron in
		checkMiner(maddrD, types.NewInt(0), true, false, r.TipSet)
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

	checkMiner(maddrA, types.NewInt(uint64(ssz)*kit.DefaultPresealsPerBootstrapMiner), true, true, types.EmptyTSK)
	checkMiner(maddrC, types.NewInt(0), true, true, types.EmptyTSK)
	checkMiner(maddrB, types.NewInt(0), true, true, types.EmptyTSK)
	checkMiner(maddrD, types.NewInt(0), false, false, types.EmptyTSK)
}
