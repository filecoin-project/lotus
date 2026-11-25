package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	miner19 "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	smoothing19 "github.com/filecoin-project/go-state-types/builtin/v19/util/smoothing"
	reward20 "github.com/filecoin-project/go-state-types/builtin/v20/reward"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	minertypes "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

func TestTerminate(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		nSectors  = 2
		ctx       = context.Background()
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.PresealSectors(nSectors))
	ens.InterconnectAll().BeginMining(blocktime)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors)))

	t.Log("Seal a sector")

	miner.PledgeSectors(ctx, 1, 0, nil)

	t.Log("wait for power")

	{
		// Wait until proven.
		di := client.CurrentProvingDeadline(ctx, maddr)

		waitUntil := di.Open + di.WPoStProvingPeriod
		t.Logf("End for head.Height > %d", waitUntil)

		ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
		t.Logf("Now head.Height = %d", ts.Height())
	}

	nSectors++

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, types.NewInt(uint64(ssz)*uint64(nSectors)), p.MinerPower.RawBytePower)

	toTerminate := abi.SectorNumber(3)

	// Testing FIP-0098 additions

	t.Log("Check MaxTerminationFee method")

	sector, err := client.StateSectorGetInfo(ctx, maddr, toTerminate, types.EmptyTSK)
	require.NoError(t, err)

	qaPower := big.NewInt(int64(ssz * 10)) // full verified sector

	maxTermFeeCases := []struct {
		power         abi.StoragePower
		expectedFeeAt func(tsk types.TipSetKey) abi.TokenAmount
	}{
		{
			// low power resulting in low fault fee => termination fee should be pledge multiple * initial pledge
			power: big.Zero(),
			expectedFeeAt: func(tsk types.TipSetKey) abi.TokenAmount {
				return big.Div(big.Mul(sector.InitialPledge, minertypes.TermFeePledgeMultiple.Numerator), minertypes.TermFeePledgeMultiple.Denominator)
			},
		},
		{
			// high power resulting in high fault fee => termination fee should be fault fee * max fault fee multiple
			power: qaPower,
			expectedFeeAt: func(tsk types.TipSetKey) abi.TokenAmount {
				var faultFee abi.TokenAmount
				rewardActor, err := client.StateGetActor(ctx, reward.Address, tsk)
				require.NoError(t, err)
				rewardState, err := reward.Load(adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client))), rewardActor)
				require.NoError(t, err)
				epochRewardSmooth, err := rewardState.ThisEpochRewardSmoothed()
				require.NoError(t, err)
				powerActor, err := client.StateGetActor(ctx, power.Address, tsk)
				require.NoError(t, err)
				powerState, err := power.Load(adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client))), powerActor)
				require.NoError(t, err)
				epochQaPowerSmoothed, err := powerState.TotalPowerSmoothed()
				require.NoError(t, err)
				nv, err := client.StateNetworkVersion(ctx, tsk)
				require.NoError(t, err)
				weight := reward19.Denom
				if nv >= network.Version30 {
					ts, err := client.ChainGetTipSet(ctx, tsk)
					require.NoError(t, err)
					ledger, err := rewardState.StreamLedger(ts.Height())
					require.NoError(t, err)
					weight = 0
					for _, stream := range ledger.Streams {
						if stream.Implicit {
							weight = stream.EvaluatedWeight
						}
					}
				}
				faultFee, err = minertypes.PledgePenaltyForContinuedFaultWithWeight(
					nv,
					builtin.FilterEstimate{
						PositionEstimate: epochRewardSmooth.PositionEstimate,
						VelocityEstimate: epochRewardSmooth.VelocityEstimate,
					},
					builtin.FilterEstimate{
						PositionEstimate: epochQaPowerSmoothed.PositionEstimate,
						VelocityEstimate: epochQaPowerSmoothed.VelocityEstimate,
					},
					qaPower, weight,
				)
				require.NoError(t, err)
				expectedPenalty := big.Div(big.Mul(faultFee, minertypes.TermFeeMaxFaultFeeMultiple.Numerator), minertypes.TermFeeMaxFaultFeeMultiple.Denominator)

				// compare against go-state-types implementation
				calculatedPenalty, err := minertypes.PledgePenaltyForTermination(nv, sector.InitialPledge, sector.Expiration-sector.Activation, faultFee)
				require.NoError(t, err)
				require.Equal(t, calculatedPenalty, expectedPenalty)
				return expectedPenalty
			},
		},
	}
	for _, tc := range maxTermFeeCases {
		t.Logf("Testing MaxTerminationFeeExported method with %s", tc.power)

		params, aerr := actors.SerializeParams(&minertypes.MaxTerminationFeeParams{
			Power:         tc.power,
			InitialPledge: sector.InitialPledge,
		})
		require.NoError(t, aerr)
		sm, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     miner.ActorAddr,
			From:   client.DefaultKey.Address,
			Method: minertypes.Methods.MaxTerminationFeeExported,
			Params: params,
			Value:  big.Zero(),
		}, nil)
		require.NoError(t, err)

		res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)
		var retval minertypes.MaxTerminationFeeReturn
		require.NoError(t, retval.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return)))
		ts, err := client.ChainGetTipSet(ctx, res.TipSet)
		require.NoError(t, err)
		expectedFee := tc.expectedFeeAt(ts.Parents())
		require.Equal(t, expectedFee, retval)
	}

	{
		t.Log("Testing InitialPledgeExported method")
		sm, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     miner.ActorAddr,
			From:   client.DefaultKey.Address,
			Method: minertypes.Methods.InitialPledgeExported,
			Params: nil,
			Value:  big.Zero(),
		}, nil)
		require.NoError(t, err)
		res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)
		var retval minertypes.InitialPledgeReturn
		require.NoError(t, retval.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return)))
		ts, err := client.ChainGetTipSet(ctx, res.TipSet)
		require.NoError(t, err)
		actor, err := client.StateGetActor(ctx, miner.ActorAddr, ts.Parents())
		require.NoError(t, err)
		actorState, err := minertypes.Load(adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client))), actor)
		require.NoError(t, err)
		require.Equal(t, must.One(actorState.InitialPledge()), retval)
	}

	{
		t.Log("Testing MinerPowerExported method")
		// This is not strictly related to terminations but it's exposed for contracts to use to
		// calculate termination fees so we'll test the export here.
		actorId := power.MinerPowerParams(must.One(address.IDFromAddress(miner.ActorAddr)))
		params, aerr := actors.SerializeParams(&actorId)
		require.NoError(t, aerr)
		sm, err := client.MpoolPushMessage(ctx, &types.Message{
			To:     power.Address,
			From:   client.DefaultKey.Address,
			Method: power.Methods.MinerPowerExported,
			Params: params,
			Value:  big.Zero(),
		}, nil)
		require.NoError(t, err)
		res, err := client.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Receipt.ExitCode)
		var retval power.MinerPowerReturn
		require.NoError(t, retval.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return)))
		ts, err := client.ChainGetTipSet(ctx, res.TipSet)
		require.NoError(t, err)
		minerPower, err := client.StateMinerPower(ctx, miner.ActorAddr, ts.Parents())
		require.NoError(t, err)
		require.Equal(t, minerPower.MinerPower.RawBytePower, retval.RawBytePower)
		require.Equal(t, minerPower.MinerPower.QualityAdjPower, retval.QualityAdjPower)
	}

	t.Log("Terminate a sector")

	require.NoError(t, miner.SectorTerminate(ctx, toTerminate))

	msgTriggerred := false
loop:
	for {
		si, err := miner.SectorsStatus(ctx, toTerminate, false)
		require.NoError(t, err)

		t.Log("state: ", si.State, msgTriggerred)

		switch sealing.SectorState(si.State) {
		case sealing.Terminating:
			if !msgTriggerred {
				{
					p, err := miner.SectorTerminatePending(ctx)
					require.NoError(t, err)
					require.Len(t, p, 1)
					require.Equal(t, abi.SectorNumber(3), p[0].Number)
				}

				c, err := miner.SectorTerminateFlush(ctx)
				require.NoError(t, err)
				if c != nil {
					msgTriggerred = true
					t.Log("terminate message:", c)

					{
						p, err := miner.SectorTerminatePending(ctx)
						require.NoError(t, err)
						require.Len(t, p, 0)
					}
				}
			}
		case sealing.TerminateWait, sealing.TerminateFinality, sealing.Removed:
			break loop
		}

		time.Sleep(100 * time.Millisecond)
	}

	// need to wait for message to be mined and applied.
	time.Sleep(5 * time.Second)

	// check power decreased
	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, types.NewInt(uint64(ssz)*uint64(nSectors-1)), p.MinerPower.RawBytePower)

	// check in terminated set
	{
		parts, err := client.StateMinerPartitions(ctx, maddr, 1, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		bflen := func(b bitfield.BitField) uint64 {
			l, err := b.Count()
			require.NoError(t, err)
			return l
		}

		require.Equal(t, uint64(1), bflen(parts[0].AllSectors))
		require.Equal(t, uint64(0), bflen(parts[0].LiveSectors))
	}

	di := client.CurrentProvingDeadline(ctx, maddr)

	waitUntil := di.PeriodStart + di.WPoStProvingPeriod + 20 // slack like above
	t.Logf("End for head.Height > %d", waitUntil)
	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, types.NewInt(uint64(ssz)*uint64(nSectors-1)), p.MinerPower.RawBytePower)

	// check "sector-terminated" actor event
	var epochZero abi.ChainEpoch
	allEvents, err := miner.FullNode.GetActorEventsRaw(ctx, &types.ActorEventFilter{
		FromHeight: &epochZero,
	})
	require.NoError(t, err)
	for _, key := range []string{"sector-precommitted", "sector-activated", "sector-terminated"} {
		var found bool
		keyBytes := must.One(ipld.Encode(basicnode.NewString(key), dagcbor.Encode))
		for _, event := range allEvents {
			for _, e := range event.Entries {
				if e.Key == "$type" && bytes.Equal(e.Value, keyBytes) {
					found = true
					if key == "sector-terminated" {
						expectedEntries := []types.EventEntry{
							{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: keyBytes},
							{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(toTerminate)), dagcbor.Encode))},
						}
						require.Equal(t, expectedEntries, event.Entries)
					}
					break
				}
			}
		}
		require.True(t, found, "expected to find event %s", key)
	}
}

func TestContinuedFaultPenaltyWeight(t *testing.T) {
	// One attoFIL per epoch for one unit of network power projects to 10,108
	// attoFIL over the 3.51-day fault window. Fractional weights round down.
	estimate := builtin.FilterEstimate{
		PositionEstimate: big.Lsh(big.NewInt(1), 128),
		VelocityEstimate: big.Zero(),
	}
	for _, tc := range []struct {
		name   string
		weight uint64
		want   int64
	}{
		{"full", 1_000_000_000_000_000_000, 10108},
		{"95 percent", 950_000_000_000_000_000, 9602},
		{"65 percent", 650_000_000_000_000_000, 6570},
		{"half", 500_000_000_000_000_000, 5054},
		{"zero", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fee, err := minertypes.PledgePenaltyForContinuedFaultWithWeight(network.Version30, estimate, estimate, big.NewInt(1), tc.weight)
			require.NoError(t, err)
			require.Equal(t, big.NewInt(tc.want), fee)

			fee, err = minertypes.PledgePenaltyForContinuedFaultWithWeight(network.Version29, estimate, estimate, big.NewInt(1), tc.weight)
			require.NoError(t, err)
			require.Equal(t, big.NewInt(10108), fee)
		})
	}

	fee, err := minertypes.PledgePenaltyForContinuedFault(network.Version29, estimate, estimate, big.NewInt(1))
	require.NoError(t, err)
	require.Equal(t, big.NewInt(10108), fee)
	_, err = minertypes.PledgePenaltyForContinuedFault(network.Version30, estimate, estimate, big.NewInt(1))
	require.ErrorContains(t, err, "implicit stream weight required")
}

func TestNV30PenaltyScaling(t *testing.T) {
	kit.QuietMiningLogs()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	swa, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)
	const upgradeEpoch abi.ChainEpoch = 100
	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(),
		kit.RewardSWA(swa, 0),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version29, Height: -1},
			stmgr.Upgrade{Network: network.Version30, Height: upgradeEpoch, Migration: filcns.UpgradeActorsV20},
		),
	)
	ens.InterconnectAll().BeginMining(100 * time.Millisecond)
	_, err = client.WalletImport(ctx, &swa.KeyInfo)
	require.NoError(t, err)

	setWeight := func(record reward19.WeightRecord) *types.TipSet {
		params, aerr := actors.SerializeParams(&reward19.SetWeightRecordsParams{Updates: []reward19.WeightRecordUpdate{{
			ID: 1, Weight: record,
		}}})
		require.NoError(t, aerr)
		msg, err := client.MpoolPushMessage(ctx, &types.Message{
			From: swa.Address, To: reward.Address, Method: stbuiltin.MethodsReward.SetWeightRecordsExported,
			Params: params, Value: big.Zero(),
		}, nil)
		require.NoError(t, err)
		lookup, err := client.WaitMsgResult(ctx, msg.Cid(), 3)
		require.NoError(t, err)
		kit.RequireMessageSuccess(t, lookup)
		ts, err := client.ChainGetTipSet(ctx, lookup.TipSet)
		require.NoError(t, err)
		return client.WaitTillChain(ctx, kit.HeightAtLeast(ts.Height()+2))
	}

	checkFee := func(ts *types.TipSet, weight uint64) {
		store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client)))
		rawReward, err := client.StateGetActor(ctx, reward.Address, ts.Key())
		require.NoError(t, err)
		rst, err := reward.Load(store, rawReward)
		require.NoError(t, err)
		estimate, err := rst.ThisEpochRewardSmoothed()
		require.NoError(t, err)
		rawPower, err := client.StateGetActor(ctx, power.Address, ts.Key())
		require.NoError(t, err)
		pst, err := power.Load(store, rawPower)
		require.NoError(t, err)
		powerEstimate, err := pst.TotalPowerSmoothed()
		require.NoError(t, err)
		qaPower := big.NewInt(20 << 10)
		unscaled := miner19.PledgePenaltyForContinuedFault(
			smoothing19.FilterEstimate(estimate), smoothing19.FilterEstimate(powerEstimate), qaPower)
		require.True(t, unscaled.GreaterThan(big.NewInt(10000)))
		faultFee := big.Div(big.Mul(unscaled, big.NewIntUnsigned(weight)), big.NewIntUnsigned(reward19.Denom))
		expected := big.Max(big.NewInt(850), big.Div(big.Mul(faultFee, big.NewInt(105)), big.NewInt(100)))
		params, err := actors.SerializeParams(&miner19.MaxTerminationFeeParams{Power: qaPower, InitialPledge: big.NewInt(10000)})
		require.NoError(t, err)
		result, err := client.StateCall(ctx, &types.Message{
			From: client.DefaultKey.Address, To: miner.ActorAddr,
			Method: stbuiltin.MethodsMiner.MaxTerminationFeeExported, Params: params, Value: big.Zero(),
		}, ts.Key())
		require.NoError(t, err)
		require.Equal(t, exitcode.Ok, result.MsgRct.ExitCode, result.Error)
		var fee miner19.MaxTerminationFeeReturn
		require.NoError(t, fee.UnmarshalCBOR(bytes.NewReader(result.MsgRct.Return)))
		require.Equal(t, expected.String(), fee.String(), "penalty at epoch %d and weight %d", ts.Height(), weight)
		if ts.Height() > upgradeEpoch {
			reply, err := client.StateCall(ctx, &types.Message{
				From: client.DefaultKey.Address, To: reward.Address, Value: big.Zero(),
				Method: stbuiltin.MethodsReward.ThisEpochReward,
			}, ts.Key())
			require.NoError(t, err)
			require.Equal(t, exitcode.Ok, reply.MsgRct.ExitCode, reply.Error)
			var epochReward reward20.ThisEpochRewardReturn
			require.NoError(t, epochReward.UnmarshalCBOR(bytes.NewReader(reply.MsgRct.Return)))
			require.Equal(t, weight, epochReward.ThisEpochImplicitStreamWeight)
			require.Equal(t, estimate.PositionEstimate, epochReward.ThisEpochRewardSmoothed.PositionEstimate)
			require.Equal(t, estimate.VelocityEstimate, epochReward.ThisEpochRewardSmoothed.VelocityEstimate)
		}
	}

	flat := func(weight uint64) reward19.WeightRecord {
		return reward19.WeightRecord{VStart: weight, Floor: weight, Cap: weight}
	}
	before := setWeight(flat(reward19.Denom / 100 * 65))
	require.Less(t, before.Height(), upgradeEpoch)
	checkFee(before, reward19.Denom)
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeEpoch+5))
	after := kit.TipsetAtOrAfter(ctx, t, client, upgradeEpoch+2)
	checkFee(after, reward19.Denom/100*65)
	checkFee(before, reward19.Denom)
	checkFee(setWeight(flat(reward19.Denom)), reward19.Denom)

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)
	pct := reward19.Denom / 100
	rampStart := head.Height() + 30
	setWeight(reward19.WeightRecord{
		VStart: 95 * pct, Slope: -int64(pct), TStart: rampStart, Floor: 50 * pct, Cap: 95 * pct,
	})
	for _, elapsed := range []abi.ChainEpoch{0, 20, 45} {
		client.WaitTillChain(ctx, kit.HeightAtLeast(rampStart+elapsed+2))
		ts := kit.TipsetAtOrAfter(ctx, t, client, rampStart+elapsed)
		rampEpochs := min(ts.Height()-rampStart, abi.ChainEpoch(45))
		weight := (95 - uint64(rampEpochs)) * pct
		checkFee(ts, weight)
	}
	zero := setWeight(flat(0))
	checkFee(zero, 0)
	pledge, err := client.StateMinerInitialPledgeForSector(ctx, stbuiltin.EpochsInYear, 2<<10, 2<<10, zero.Key())
	require.NoError(t, err)
	require.True(t, pledge.GreaterThan(big.Zero()))
	params, aerr := actors.SerializeParams(&reward19.RemoveStreamParams{ID: 1})
	require.NoError(t, aerr)
	msg, err := client.MpoolPushMessage(ctx, &types.Message{
		From: swa.Address, To: reward.Address, Value: big.Zero(),
		Method: stbuiltin.MethodsReward.RemoveStreamExported, Params: params,
	}, nil)
	require.NoError(t, err)
	lookup, err := client.WaitMsgResult(ctx, msg.Cid(), 3)
	require.NoError(t, err)
	kit.RequireMessageSuccess(t, lookup)
	ts, err := client.ChainGetTipSet(ctx, lookup.TipSet)
	require.NoError(t, err)
	checkFee(client.WaitTillChain(ctx, kit.HeightAtLeast(ts.Height()+2)), 0)
}
