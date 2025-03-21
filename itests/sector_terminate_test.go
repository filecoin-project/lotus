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

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	minertypes "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
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
		di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)

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
				faultFee, err = minertypes.PledgePenaltyForContinuedFault(
					nv,
					builtin.FilterEstimate{
						PositionEstimate: epochRewardSmooth.PositionEstimate,
						VelocityEstimate: epochRewardSmooth.VelocityEstimate,
					},
					builtin.FilterEstimate{
						PositionEstimate: epochQaPowerSmoothed.PositionEstimate,
						VelocityEstimate: epochQaPowerSmoothed.VelocityEstimate,
					},
					qaPower,
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

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

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
