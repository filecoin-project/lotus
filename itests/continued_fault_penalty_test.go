package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	smoothing19 "github.com/filecoin-project/go-state-types/builtin/v19/util/smoothing"
	miner20 "github.com/filecoin-project/go-state-types/builtin/v20/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
)

func TestNV30ContinuedFaultPenaltyWeight(t *testing.T) {
	kit.QuietMiningLogs()
	initialReserved := buildconstants.InitialFilReserved
	teepReserved := buildconstants.UpgradeTeepInitialFilReserved
	// Positive circulating supply makes the daily-fee assertion nontrivial.
	buildconstants.InitialFilReserved = types.FromFil(1_000_000_000).Int
	buildconstants.UpgradeTeepInitialFilReserved = buildconstants.InitialFilReserved
	t.Cleanup(func() {
		buildconstants.InitialFilReserved = initialReserved
		buildconstants.UpgradeTeepInitialFilReserved = teepReserved
	})
	const (
		sectorSize        = abi.SectorSize(2 << 10)
		fullWeight uint64 = 1_000_000_000_000_000_000
	)

	for _, tc := range []struct {
		name   string
		weight uint64
	}{{"full", fullWeight}, {"half", fullWeight / 2}, {"zero", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			owner := must.One(key.GenerateKey(types.KTBLS))
			swa := must.One(key.GenerateKey(types.KTSecp256k1))
			client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(),
				kit.GenesisNetworkVersion(network.Version30), kit.RewardSWA(swa, 0),
				kit.Account(owner, types.FromFil(2_000_000)))
			_, err := client.WalletImport(ctx, &swa.KeyInfo)
			require.NoError(t, err)
			blockMiners := ens.InterconnectAll().BeginMiningMustPost(5 * time.Millisecond)
			faultMiner, ens := ens.UnmanagedMiner(ctx, client, kit.SectorSize(sectorSize), kit.OwnerAddr(owner))
			defer faultMiner.Stop()
			ens.Start()
			proof, err := miner.SealProofTypeFromSectorSize(sectorSize, network.Version30, miner.SealProofVariant_Standard)
			require.NoError(t, err)
			sectors, _ := faultMiner.OnboardSectors(proof, kit.NewSectorBatch().AddEmptySectors(1))
			require.Len(t, sectors, 1)
			blockMiners[0].WatchMinerForPost(faultMiner.ActorAddr)
			require.NoError(t, faultMiner.WaitTillActivatedAndAssertPower(sectors, uint64(sectorSize), uint64(sectorSize)*10))
			fund, err := client.MpoolPushMessage(ctx, &types.Message{
				From: owner.Address, To: faultMiner.ActorAddr, Value: types.FromFil(1_000_000),
			}, nil)
			require.NoError(t, err)
			funded, err := client.WaitMsgResult(ctx, fund.Cid(), 2)
			require.NoError(t, err)
			kit.RequireMessageSuccess(t, funded)
			faulted := faultMiner.DeclareFaults(sectors)
			kit.RequireMessageSuccess(t, faulted)
			sector, err := client.StateSectorGetInfo(ctx, faultMiner.ActorAddr, sectors[0], types.EmptyTSK)
			require.NoError(t, err)
			require.True(t, sector.DailyFee.GreaterThan(big.Zero()))
			qaPower := big.NewIntUnsigned(uint64(sectorSize) * 10)
			params := &reward19.SetWeightRecordsParams{Updates: []reward19.WeightRecordUpdate{{
				ID: 1, Weight: reward19.WeightRecord{VStart: tc.weight, Floor: tc.weight, Cap: tc.weight},
			}}}
			message, err := client.MpoolPushMessage(ctx, &types.Message{
				From: swa.Address, To: builtin.RewardActorAddr, Value: big.Zero(),
				Method: builtin.MethodsReward.SetWeightRecordsExported, Params: must.One(actors.SerializeParams(params)),
			}, nil)
			require.NoError(t, err)
			lookup, err := client.WaitMsgResult(ctx, message.Cid(), 3)
			require.NoError(t, err)
			kit.RequireMessageSuccess(t, lookup)
			head, err := client.ChainHead(ctx)
			require.NoError(t, err)
			location, err := client.StateSectorPartition(ctx, faultMiner.ActorAddr, sectors[0], head.Key())
			require.NoError(t, err)
			deadline, err := client.StateMinerProvingDeadline(ctx, faultMiner.ActorAddr, head.Key())
			require.NoError(t, err)
			closeAt := kit.DeadlineCloseAfter(kit.DeadlineForHeight(deadline, head.Height()), location.Deadline, head.Height())
			client.WaitTillChain(ctx, kit.HeightAtLeast(closeAt+5))
			executed := kit.TipsetAtOrAfter(ctx, t, client, closeAt-1)
			computed, err := client.StateCompute(ctx, executed.Height(), nil, executed.Key())
			require.NoError(t, err)

			var cron *types.ExecutionTrace
			var visit func(types.ExecutionTrace)
			visit = func(trace types.ExecutionTrace) {
				if trace.Msg.From == builtin.StoragePowerActorAddr && trace.Msg.To == faultMiner.ActorAddr && trace.Msg.Method == builtin.MethodsMiner.OnDeferredCronEvent {
					require.Nil(t, cron, "one deadline callback expected")
					cron = &trace
				}
				for _, subcall := range trace.Subcalls {
					visit(subcall)
				}
			}
			for _, invocation := range computed.Trace {
				visit(invocation.ExecutionTrace)
			}
			require.NotNil(t, cron)
			require.Equal(t, exitcode.Ok, cron.MsgRct.ExitCode)
			var cronParams miner20.DeferredCronEventParams
			require.NoError(t, cronParams.UnmarshalCBOR(bytes.NewReader(cron.Msg.Params)))
			require.Equal(t, tc.weight, cronParams.ImplicitStreamWeight)
			reward := smoothing19.FilterEstimate(cronParams.RewardSmoothed)
			power := smoothing19.FilterEstimate(cronParams.QualityAdjPowerSmoothed)
			unscaled := stminer.PledgePenaltyForContinuedFault(reward, power, qaPower)
			require.True(t, unscaled.GreaterThan(big.Zero()))
			faultFee := big.Div(big.Mul(unscaled, big.NewIntUnsigned(tc.weight)), big.NewIntUnsigned(fullWeight))
			dayReward := stminer.ExpectedRewardForPower(reward, power, qaPower, builtin.EpochsInDay)
			dailyFee := big.Min(sector.DailyFee, big.Div(dayReward, big.NewInt(stminer.DailyFeeBlockRewardCapDenom)))
			require.True(t, dailyFee.GreaterThan(big.Zero()))
			burned := big.Zero()
			for _, call := range cron.Subcalls {
				if call.Msg.From == faultMiner.ActorAddr && call.Msg.To == builtin.BurntFundsActorAddr {
					burned = big.Add(burned, call.Msg.Value)
				}
			}
			require.Equal(t, big.Add(faultFee, dailyFee), burned)
			after := kit.TipsetAtOrAfter(ctx, t, client, executed.Height()+1)
			state := client.MinerState(ctx, faultMiner.ActorAddr, after.Key())
			require.True(t, state.FeeDebt.IsZero())
			currentSector, err := client.StateSectorGetInfo(ctx, faultMiner.ActorAddr, sectors[0], after.Key())
			require.NoError(t, err)
			require.Equal(t, sector.DailyFee, currentSector.DailyFee)
			powerAfter, err := client.StateMinerPower(ctx, faultMiner.ActorAddr, after.Key())
			require.NoError(t, err)
			require.True(t, powerAfter.MinerPower.RawBytePower.IsZero())
		})
	}
}
