package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner19 "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	power19 "github.com/filecoin-project/go-state-types/builtin/v19/power"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	smoothing19 "github.com/filecoin-project/go-state-types/builtin/v19/util/smoothing"
	reward20 "github.com/filecoin-project/go-state-types/builtin/v20/reward"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestNV30WindowPostDisputePenaltyWeight(t *testing.T) {
	kit.QuietMiningLogs()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	swa, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)
	var client kit.TestFullNode
	var chainMiner, targetMiner kit.TestMiner
	ens := kit.NewEnsemble(t, kit.MockProofs(), kit.GenesisNetworkVersion(network.Version30), kit.RewardSWA(swa, 0)).
		FullNode(&client, kit.ThroughRPC()).
		Miner(&chainMiner, &client, kit.WithAllSubsystems()).
		Miner(&targetMiner, &client, kit.WithAllSubsystems(), kit.PresealSectors(1)).
		Start()
	require.NoError(t, targetMiner.Stop(ctx))
	ens.InterconnectAll().BeginMiningMustPost(5*time.Millisecond, &chainMiner)
	_, err = client.WalletImport(ctx, &swa.KeyInfo)
	require.NoError(t, err)
	kit.SendFunds(ctx, t, &client, targetMiner.ActorAddr, types.FromFil(1000))
	sectors, err := client.StateMinerSectors(ctx, targetMiner.ActorAddr, nil, types.EmptyTSK)
	require.NoError(t, err)
	require.Len(t, sectors, 1)
	location, err := client.StateSectorPartition(ctx, targetMiner.ActorAddr, sectors[0].SectorNumber, types.EmptyTSK)
	require.NoError(t, err)
	for {
		dl := client.CurrentProvingDeadline(ctx, targetMiner.ActorAddr)
		if dl.Index == location.Deadline && dl.CurrentEpoch > dl.Open+1 {
			require.NoError(t, submitBadProof(ctx, client, targetMiner.OwnerKey.Address, targetMiner.ActorAddr, dl, location.Deadline, location.Partition))
			client.WaitTillChain(ctx, kit.HeightAtLeast(dl.Close+3))
			break
		}
		build.Clock.Sleep(5 * time.Millisecond)
	}
	params, err := actors.SerializeParams(&minertypes.DisputeWindowedPoStParams{Deadline: location.Deadline, PoStIndex: 0})
	require.NoError(t, err)
	const fullWeight uint64 = 1_000_000_000_000_000_000
	for _, tc := range []struct {
		name   string
		weight uint64
	}{{"full", fullWeight}, {"half", fullWeight / 2}, {"zero", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			weights, encodeErr := actors.SerializeParams(&reward19.SetWeightRecordsParams{Updates: []reward19.WeightRecordUpdate{{
				ID: 1, Weight: reward19.WeightRecord{VStart: tc.weight, Floor: tc.weight, Cap: tc.weight},
			}}})
			require.NoError(t, encodeErr)
			msg, err := client.MpoolPushMessage(ctx, &types.Message{
				From: swa.Address, To: builtin.RewardActorAddr, Method: builtin.MethodsReward.SetWeightRecordsExported,
				Value: big.Zero(), Params: weights,
			}, nil)
			require.NoError(t, err)
			lookup, err := client.WaitMsgResult(ctx, msg.Cid(), 3)
			require.NoError(t, err)
			kit.RequireMessageSuccess(t, lookup)
			head, err := client.ChainHead(ctx)
			require.NoError(t, err)
			result, err := client.StateCall(ctx, &types.Message{
				From: client.DefaultKey.Address, To: targetMiner.ActorAddr,
				Method: builtin.MethodsMiner.DisputeWindowedPoSt, Params: params, Value: big.Zero(),
			}, head.Key())
			require.NoError(t, err)
			require.Equal(t, exitcode.Ok, result.MsgRct.ExitCode, result.Error)
			var reward reward20.ThisEpochRewardReturn
			var power power19.CurrentTotalPowerReturn
			var haveReward, havePower bool
			burned, rewarded := big.Zero(), big.Zero()
			for _, call := range result.ExecutionTrace.Subcalls {
				switch {
				case call.Msg.To == builtin.RewardActorAddr && call.Msg.Method == builtin.MethodsReward.ThisEpochReward:
					require.NoError(t, reward.UnmarshalCBOR(bytes.NewReader(call.MsgRct.Return)))
					haveReward = true
				case call.Msg.To == builtin.StoragePowerActorAddr && call.Msg.Method == builtin.MethodsPower.CurrentTotalPower:
					require.NoError(t, power.UnmarshalCBOR(bytes.NewReader(call.MsgRct.Return)))
					havePower = true
				case call.Msg.To == builtin.BurntFundsActorAddr:
					burned = big.Add(burned, call.Msg.Value)
				case call.Msg.Method == builtin.MethodSend:
					rewarded = big.Add(rewarded, call.Msg.Value)
				}
			}
			require.True(t, haveReward)
			require.True(t, havePower)
			require.Equal(t, tc.weight, reward.ThisEpochImplicitStreamWeight)
			qaPower := big.NewInt(10 * 2 << 10)
			projected := miner19.ExpectedRewardForPower(
				smoothing19.FilterEstimate(reward.ThisEpochRewardSmoothed), power.QualityAdjPowerSmoothed,
				qaPower, miner19.ContinuedFaultProjectionPeriod+2*abi.ChainEpoch(builtin.EpochsInDay))
			require.True(t, projected.GreaterThan(big.Zero()))
			scaled := big.Div(big.Mul(projected, big.NewIntUnsigned(tc.weight)), big.NewIntUnsigned(fullWeight))
			require.Equal(t, big.Add(types.FromFil(20), scaled), burned)
			require.Equal(t, types.FromFil(4), rewarded)
		})
	}
}

func TestWindowPostDispute(t *testing.T) {

	kit.Expensive(t)

	kit.QuietMiningLogs()

	blocktime := 2 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		client     kit.TestFullNode
		chainMiner kit.TestMiner
		evilMiner  kit.TestMiner
	)

	// First, we configure two miners. After sealing, we're going to turn off the first miner so
	// it doesn't submit proofs.
	//
	// Then we're going to manually submit bad proofs.
	opts := []kit.NodeOpt{kit.WithAllSubsystems()}
	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&client, opts...).
		Miner(&chainMiner, &client, opts...).
		Miner(&evilMiner, &client, append(opts, kit.PresealSectors(0))...).
		Start()

	defaultFrom, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// Mine with the _second_ node (the good one).
	ens.InterconnectAll().BeginMining(blocktime, &chainMiner)

	// Give the chain miner enough sectors to win every block.
	chainMiner.PledgeSectors(ctx, 10, 0, nil)
	// And the evil one 1 sector. No cookie for you.
	evilMiner.PledgeSectors(ctx, 1, 0, nil)

	// Let the evil miner's sectors gain power.
	evilMinerAddr, err := evilMiner.ActorAddress(ctx)
	require.NoError(t, err)

	di := client.CurrentProvingDeadline(ctx, evilMinerAddr)

	t.Logf("Running one proving period\n")

	waitUntil := di.PeriodStart + di.WPoStProvingPeriod*2 + 1
	t.Logf("End for head.Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err := client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := evilMiner.ActorSectorSize(ctx, evilMinerAddr)
	require.NoError(t, err)

	// make sure it has gained power.
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)))

	evilSectors, err := evilMiner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	evilSectorNo := evilSectors[0] // only one.
	evilSectorLoc, err := client.StateSectorPartition(ctx, evilMinerAddr, evilSectorNo, types.EmptyTSK)
	require.NoError(t, err)

	t.Log("evil miner stopping")

	// Now stop the evil miner, and start manually submitting bad proofs.
	require.NoError(t, evilMiner.Stop(ctx))

	t.Log("evil miner stopped")

	// Wait until we need to prove our sector.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index == evilSectorLoc.Deadline && di.CurrentEpoch-di.Open > 1 {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	err = submitBadProof(ctx, client, evilMiner.OwnerKey.Address, evilMinerAddr, di, evilSectorLoc.Deadline, evilSectorLoc.Partition)
	require.NoError(t, err, "evil proof not accepted")

	// Wait until after the proving period.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index != evilSectorLoc.Deadline {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	t.Log("accepted evil proof")

	// Make sure the evil node didn't lose any power.
	p, err = client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)))

	// OBJECTION! The good miner files a DISPUTE!!!!
	{
		params := &minertypes.DisputeWindowedPoStParams{
			Deadline:  evilSectorLoc.Deadline,
			PoStIndex: 0,
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     evilMinerAddr,
			Method: builtin.MethodsMiner.DisputeWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
			From:   defaultFrom,
		}
		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		t.Log("waiting dispute")
		rec, err := client.StateWaitMsg(ctx, sm.Cid(), buildconstants.MessageConfidence, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Zero(t, rec.Receipt.ExitCode, "dispute not accepted: %s", rec.Receipt.ExitCode.Error())
	}

	// Objection SUSTAINED!
	// Make sure the evil node lost power.
	p, err = client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	// Now we begin the redemption arc.
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	// First, recover the sector.

	{
		minerInfo, err := client.StateMinerInfo(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)

		params := &minertypes.DeclareFaultsRecoveredParams{
			Recoveries: []minertypes.RecoveryDeclaration{{
				Deadline:  evilSectorLoc.Deadline,
				Partition: evilSectorLoc.Partition,
				Sectors:   bitfield.NewFromSet([]uint64{uint64(evilSectorNo)}),
			}},
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     evilMinerAddr,
			Method: builtin.MethodsMiner.DeclareFaultsRecovered,
			Params: enc,
			Value:  types.FromFil(30), // repay debt.
			From:   minerInfo.Owner,
		}
		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		rec, err := client.StateWaitMsg(ctx, sm.Cid(), buildconstants.MessageConfidence, api.LookbackNoLimit, true)
		require.NoError(t, err)
		require.Zero(t, rec.Receipt.ExitCode, "recovery not accepted: %s", rec.Receipt.ExitCode.Error())
	}

	// Then wait for the deadline.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)

		if di.Index == evilSectorLoc.Deadline && di.CurrentEpoch-di.Open > 1 {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	// Now try to be evil again
	err = submitBadProof(ctx, client, evilMiner.OwnerKey.Address, evilMinerAddr, di, evilSectorLoc.Deadline, evilSectorLoc.Partition)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid post was submitted")
	require.Contains(t, err.Error(), "(RetCode=16)")

	// It didn't work because we're recovering.
}

func TestWindowPostDisputeFails(t *testing.T) {
	kit.Expensive(t)

	kit.QuietMiningLogs()

	blocktime := 2 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(blocktime)

	defaultFrom, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	build.Clock.Sleep(time.Second)

	miner.PledgeSectors(ctx, 10, 0, nil)

	di := client.CurrentProvingDeadline(ctx, maddr)

	t.Log("Running one proving period")
	waitUntil := di.PeriodStart + di.WPoStProvingPeriod*2 + 1
	t.Logf("End for head.Height > %d", waitUntil)

	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)
	expectedPower := types.NewInt(uint64(ssz) * (kit.DefaultPresealsPerBootstrapMiner + 10))

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	// make sure it has gained power.
	require.Equal(t, p.MinerPower.RawBytePower, expectedPower)

	// Wait until a proof has been submitted.
	var targetDeadline uint64
waitForProof:
	for {

		deadlines, err := client.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		for dlIdx, dl := range deadlines {
			isEmpty, err := dl.PostSubmissions.IsEmpty()
			require.NoError(t, err)
			if !isEmpty {
				head, err := client.ChainHead(ctx)
				require.NoError(t, err)
				di, err := client.StateMinerProvingDeadline(ctx, maddr, head.Key())
				require.NoError(t, err)
				disputeEpoch := di.Close + 5
				_ = client.WaitTillChain(ctx, kit.HeightAtLeast(disputeEpoch))

				targetDeadline = uint64(dlIdx)
				break waitForProof
			}
		}
		build.Clock.Sleep(blocktime)
	}

	// Try to object to the proof. This should fail.
	{
		params := &minertypes.DisputeWindowedPoStParams{
			Deadline:  targetDeadline,
			PoStIndex: 0,
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     maddr,
			Method: builtin.MethodsMiner.DisputeWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
			From:   defaultFrom,
		}
		_, err = client.MpoolPushMessage(ctx, msg, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to dispute valid post")
		require.Contains(t, err.Error(), "(RetCode=16)")
	}
}

func submitBadProof(
	ctx context.Context,
	client api.FullNode, owner address.Address, maddr address.Address,
	di *dline.Info, dlIdx, partIdx uint64,
) error {
	head, err := client.ChainHead(ctx)
	if err != nil {
		return err
	}

	minerInfo, err := client.StateMinerInfo(ctx, maddr, head.Key())
	if err != nil {
		return err
	}

	commEpoch := di.Open
	commRand, err := client.StateGetRandomnessFromTickets(
		ctx, crypto.DomainSeparationTag_PoStChainCommit,
		commEpoch, nil, head.Key(),
	)
	if err != nil {
		return err
	}
	params := &minertypes.SubmitWindowedPoStParams{
		ChainCommitEpoch: commEpoch,
		ChainCommitRand:  commRand,
		Deadline:         dlIdx,
		Partitions:       []minertypes.PoStPartition{{Index: partIdx}},
		Proofs: []prooftypes.PoStProof{{
			PoStProof:  minerInfo.WindowPoStProofType,
			ProofBytes: []byte("I'm soooo very evil."),
		}},
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return aerr
	}

	msg := &types.Message{
		To:     maddr,
		Method: builtin.MethodsMiner.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(0),
		From:   owner,
	}
	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return err
	}

	rec, err := client.StateWaitMsg(ctx, sm.Cid(), buildconstants.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return err
	}
	if rec.Receipt.ExitCode.IsError() {
		return rec.Receipt.ExitCode
	}
	return nil
}
