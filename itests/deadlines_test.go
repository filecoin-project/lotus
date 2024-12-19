package itests

import (
	"bytes"
	"context"
	"strings"
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
	"github.com/filecoin-project/go-state-types/exitcode"
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
func TestDeadlineToggling(t *testing.T) {

	kit.Expensive(t)

	kit.QuietMiningLogs()

	const sectorsC, sectorsD, sectorsB = 10, 9, 8

	var (
		provingPeriod abi.ChainEpoch = 2880
		blocktime                    = 2 * time.Millisecond
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		client kit.TestFullNode
		minerA kit.TestMiner // A has some genesis sector, just keeps power
		minerB kit.TestMiner // B pledges some sector, later fails some posts but stays alive
		minerC kit.TestMiner // C pledges sectors, gains power, and later stops its PoSTs, but stays alive
		minerD kit.TestMiner // D pledges sectors and later terminates them, losing all power, eventually deactivates cron
		minerE kit.TestMiner // E pre-commits a sector but doesn't advance beyond that, cron should become inactive
	)
	opts := []kit.NodeOpt{kit.WithAllSubsystems()}
	ens := kit.NewEnsemble(t, kit.MockProofs()).
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

	targetHeight := abi.ChainEpoch(0)

	// pledge sectors on C, go through a PP, check for power
	{
		minerC.PledgeSectors(ctx, sectorsC, 0, nil)

		di, err := client.StateMinerProvingDeadline(ctx, maddrC, types.EmptyTSK)
		require.NoError(t, err)

		t.Log("Running one proving period (miner C)")
		t.Logf("End for head.Height > %d", di.PeriodStart+di.WPoStProvingPeriod*2)

		targetHeight = di.PeriodStart + provingPeriod*2

		for {
			head, err := client.ChainHead(ctx)
			require.NoError(t, err)

			if head.Height() > targetHeight {
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

	checkMiner := func(ma address.Address, power abi.StoragePower, active bool, tsk types.TipSetKey) {
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

		require.Equal(t, active, act)
	}

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

		params := &miner.PreCommitSectorBatchParams2{
			Sectors: []miner.SectorPreCommitInfo{
				{
					Expiration:   2880 * 300,
					SectorNumber: 22,
					SealProof:    kit.TestSpt,

					SealedCID:     cr,
					SealRandEpoch: head.Height() - 200,
				},
			},
		}

		enc := new(bytes.Buffer)
		require.NoError(t, params.MarshalCBOR(enc))

		m, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     maddrE,
			From:   defaultFrom,
			Value:  types.FromFil(1),
			Method: builtin.MethodsMiner.PreCommitSectorBatch2,
			Params: enc.Bytes(),
		}, nil)
		require.NoError(t, err)

		r, err := client.StateWaitMsg(ctx, m.Cid(), 2, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)
	}

	targetHeight = targetHeight + (provingPeriod / 2)

	// go through 0.5 PP
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > targetHeight {
			t.Logf("Now head.Height = %d", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	checkMiner(maddrE, types.NewInt(0), true, types.EmptyTSK)

	targetHeight = targetHeight + (provingPeriod/2)*5

	// go through rest of the PP
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > targetHeight {
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

	// Note: in the older version of this test `active` would be set to false
	// this is now true because the time to commit a precommit a sector has
	// increased to 30 days. We could keep the original assert and increase the
	// wait above to 30 days, but that makes the test take 14 minutes to run..
	checkMiner(maddrE, types.NewInt(0), true, types.EmptyTSK)

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

		var smsg *types.SignedMessage
		require.Eventually(t, func() bool {
			smsg, err = client.MpoolPushMessage(ctx, &types.Message{
				From:   defaultFrom,
				To:     maddrD,
				Method: builtin.MethodsMiner.TerminateSectors,

				Value:  big.Zero(),
				Params: sp,
			}, nil)
			return err == nil || !strings.Contains(err.Error(), "cannot terminate sectors in immutable deadline")
		}, 60*time.Second, 100*time.Millisecond)
		require.NoError(t, err)

		t.Log("sent termination message:", smsg.Cid())

		r, err := client.StateWaitMsg(ctx, smsg.Cid(), 2, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, r.Receipt.ExitCode)

		// assert miner has no power
		p, err := client.StateMinerPower(ctx, maddrD, r.TipSet)
		require.NoError(t, err)
		require.True(t, p.MinerPower.RawBytePower.IsZero())
	}

	targetHeight = targetHeight + provingPeriod*2

	// go through another PP
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > targetHeight {
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
