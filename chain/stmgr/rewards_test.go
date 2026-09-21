package stmgr

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	reward19 "github.com/filecoin-project/go-state-types/builtin/v19/reward"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

func TestRewardDistributionForAward(t *testing.T) {
	const d = reward.Denom
	alice, _ := address.NewIDAddress(101)
	bob, _ := address.NewIDAddress(102)
	writer, _ := address.NewIDAddress(103)
	stream := func(id uint64, weight uint64, shares ...reward.RecipientShare) reward.Stream {
		return reward.Stream{
			ID: reward.StreamID(id), EvaluatedWeight: weight,
			Writer: writer, Shares: shares, Accrued: big.Zero(),
		}
	}
	implicit := reward.Stream{ID: 7, EvaluatedWeight: 3 * d / 5, Implicit: true}

	tests := []struct {
		name                     string
		streams                  []reward.Stream
		minted, explicit, burned int64
		beforePool, afterPool    int64
		poolID                   reward.StreamID
		wantMiner                string
		wantBurnWeight           uint64
		wantEarned               []string
		wantStreamBurn           string
		wantRounding             string
	}{
		{
			name: "unassigned stream weight and recipient shares both burn",
			streams: []reward.Stream{
				implicit,
				stream(8, 3*d/10, reward.RecipientShare{Recipient: alice, Share: d / 2}, reward.RecipientShare{Recipient: bob, Share: d / 4}),
			},
			minted: 100, explicit: 22, burned: 18, beforePool: 30, afterPool: 52, poolID: 8,
			wantMiner: "60", wantBurnWeight: d / 10,
			wantEarned: []string{"14", "7"}, wantStreamBurn: "8", wantRounding: "1",
		},
		{
			name: "previous rounding dust becomes earned",
			streams: []reward.Stream{
				stream(2, d, reward.RecipientShare{Recipient: alice, Share: d / 2}, reward.RecipientShare{Recipient: bob, Share: d / 2}),
			},
			minted: 1, explicit: 1, beforePool: 1, afterPool: 2, poolID: 2,
			wantMiner: "0", wantEarned: []string{"1", "1"}, wantStreamBurn: "0", wantRounding: "-1",
		},
		{
			name: "due writer change settles old pool before award",
			streams: []reward.Stream{
				stream(2, d, reward.RecipientShare{Recipient: alice, Share: d / 2}, reward.RecipientShare{Recipient: bob, Share: d / 2}),
			},
			minted: 1, explicit: 1, beforePool: 101, afterPool: 1, poolID: 2,
			wantMiner: "0", wantEarned: []string{"0", "0"}, wantStreamBurn: "0", wantRounding: "1",
		},
		{
			name: "schedule has no implicit stream",
			streams: []reward.Stream{
				stream(9, d/2, reward.RecipientShare{Recipient: alice, Share: d}),
			},
			minted: 10, explicit: 5, burned: 5, afterPool: 5, poolID: 9,
			wantMiner: "0", wantBurnWeight: d / 2, wantEarned: []string{"5"}, wantStreamBurn: "0", wantRounding: "0",
		},
		{
			name:    "explicit stream burns its entire allocation",
			streams: []reward.Stream{stream(2, d)},
			minted:  10, burned: 10, poolID: 2,
			wantMiner: "0", wantEarned: []string{}, wantStreamBurn: "10", wantRounding: "0",
		},
		{
			name:   "empty schedule burns everything",
			minted: 10, burned: 10, wantMiner: "0", wantBurnWeight: d,
		},
		{
			name: "gas-only award retains fractions without new earnings",
			streams: []reward.Stream{
				stream(2, d, reward.RecipientShare{Recipient: alice, Share: d / 2}, reward.RecipientShare{Recipient: bob, Share: d / 2}),
			},
			beforePool: 13, afterPool: 13, poolID: 2,
			wantMiner: "0", wantEarned: []string{"0", "0"}, wantStreamBurn: "0", wantRounding: "0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before, after := rewardTestLedgers(test.minted, test.explicit, test.burned)
			before.Streams = append([]reward.Stream(nil), test.streams...)
			after.Streams = append([]reward.Stream(nil), test.streams...)
			for i := range after.Streams {
				if after.Streams[i].ID == test.poolID {
					before.Streams[i].Accrued = big.NewInt(test.beforePool)
					after.Streams[i].Accrued = big.NewInt(test.afterPool)
				}
			}
			rows, burnWeight, amounts, err := rewardDistributionForAward(before, after)
			require.NoError(t, err)
			require.Equal(t, test.wantBurnWeight, burnWeight)
			require.Equal(t, big.NewInt(test.minted).String(), amounts.MintedReward.String())
			require.Equal(t, test.wantMiner, amounts.MinerReward.String())
			require.Equal(t, big.NewInt(test.explicit).String(), amounts.ExplicitReward.String())
			require.Equal(t, big.NewInt(test.burned).String(), amounts.BurnAllocation.String())
			require.Len(t, rows, len(test.streams))
			for i, row := range rows {
				require.Equal(t, uint64(test.streams[i].ID), row.ID)
				if test.streams[i].Implicit {
					require.Nil(t, row.Distribution)
					continue
				}
				distribution := row.Distribution
				require.NotNil(t, distribution)
				require.Equal(t, writer, distribution.Writer)
				require.Equal(t, test.wantStreamBurn, distribution.BurnAmount.String())
				require.Equal(t, test.wantRounding, distribution.RoundingAdjustment.String())
				require.Len(t, distribution.Recipients, len(test.wantEarned))
				for i, recipient := range distribution.Recipients {
					require.Equal(t, test.wantEarned[i], recipient.EarnedAmount.String())
				}
			}
		})
	}
}

func rewardTestLedgers(minted, explicit, burned int64) (*reward.StreamLedger, *reward.StreamLedger) {
	before := &reward.StreamLedger{
		TotalMinted: big.NewInt(1_000), TotalExplicitMinted: big.NewInt(100), TotalBurnMinted: big.NewInt(100),
	}
	after := &reward.StreamLedger{
		TotalMinted: big.NewInt(1_000 + minted), TotalExplicitMinted: big.NewInt(100 + explicit), TotalBurnMinted: big.NewInt(100 + burned),
	}
	return before, after
}

func TestRewardDistributionMonitor(t *testing.T) {
	ctx := context.Background()
	store := adt.WrapStore(ctx, cbor.NewMemCborStore())
	miner1, err := address.NewIDAddress(1000)
	require.NoError(t, err)
	miner2, err := address.NewIDAddress(1001)
	require.NoError(t, err)
	root, err := abi.CidBuilder.Sum([]byte("reward monitor fixture"))
	require.NoError(t, err)
	block := func(miner address.Address, wins int64) *types.BlockHeader {
		return &types.BlockHeader{
			Miner:                 miner,
			Ticket:                &types.Ticket{VRFProof: miner.Bytes()},
			ElectionProof:         &types.ElectionProof{WinCount: wins},
			Parents:               []cid.Cid{root},
			Height:                101,
			ParentStateRoot:       root,
			ParentMessageReceipts: root,
			Messages:              root,
			ParentWeight:          big.Zero(),
			ParentBaseFee:         big.Zero(),
		}
	}
	block1, block2 := block(miner1, 1), block(miner2, 2)
	ts, err := types.NewTipSet([]*types.BlockHeader{block1, block2})
	require.NoError(t, err)
	m := &rewardDistributionMonitor{
		store:  store,
		result: v2api.RewardDistribution{TipSetKey: ts.Key(), Height: ts.Height(), Denom: reward19.Denom, Totals: zeroRewardAmounts()},
	}
	st, err := reward19.ConstructState(store, big.Zero())
	require.NoError(t, err)
	code, ok := actors.GetActorCodeID(actorstypes.Version19, manifest.RewardKey)
	require.True(t, ok)
	tree, err := state.NewStateTree(store, types.StateTreeVersion5)
	require.NoError(t, err)
	snapshot := func() cid.Cid {
		head, err := store.Put(ctx, st)
		require.NoError(t, err)
		require.NoError(t, tree.SetActor(reward.Address, &types.Actor{Code: code, Head: head, Balance: big.NewInt(10000)}))
		root, err := tree.Flush(ctx)
		require.NoError(t, err)
		return root
	}
	transfer := func(from, to address.Address, amount int64, result exitcode.ExitCode) types.ExecutionTrace {
		return types.ExecutionTrace{Msg: types.MessageTrace{From: from, To: to, Value: big.NewInt(amount)}, MsgRct: types.ReturnTrace{ExitCode: result}}
	}
	for i, block := range ts.Blocks() {
		before := snapshot()
		fee := int64(7)
		st.TotalMintedReward = big.NewInt(100)
		if i == 1 {
			fee = 11
			// The second block used a different distribution. Its award cannot be
			// reconstructed by applying the final weights to both blocks.
			st.TotalMintedReward = big.NewInt(150)
			st.TotalBurnMinted = big.NewInt(20)
			weight := 3 * reward19.Denom / 5
			streams := &reward19.StreamsState{Streams: []reward19.Stream{{ID: 7, Weight: reward19.WeightRecord{VStart: weight, Floor: weight, Cap: weight}}}}
			st.StreamsRoot, err = store.Put(ctx, streams)
			require.NoError(t, err)
		}
		after := snapshot()
		params, err := actors.SerializeParams(&reward.AwardBlockRewardParams{Miner: block.Miner, WinCount: block.ElectionProof.WinCount, GasReward: big.NewInt(fee), Penalty: big.Zero()})
		require.NoError(t, err)
		msg := &types.Message{From: builtin.SystemActorAddr, To: reward.Address, Method: reward.Methods.AwardBlockReward, Params: params}
		ret := &vm.ApplyRet{}
		if i == 0 {
			minerCall := transfer(reward.Address, block.Miner, 107, exitcode.Ok)
			minerCall.Subcalls = []types.ExecutionTrace{transfer(block.Miner, builtin.BurntFundsActorAddr, 13, exitcode.Ok)}
			ret.ExecutionTrace.Subcalls = []types.ExecutionTrace{
				minerCall,
				transfer(reward.Address, builtin.BurntFundsActorAddr, 2, exitcode.Ok), // older dust settled by this award
				transfer(reward.Address, builtin.BurntFundsActorAddr, 99, exitcode.ErrIllegalState),
			}
		} else {
			ret.ExecutionTrace.Subcalls = []types.ExecutionTrace{
				transfer(reward.Address, block.Miner, 41, exitcode.ErrIllegalState),
				transfer(reward.Address, builtin.BurntFundsActorAddr, 41, exitcode.Ok), // failed miner payment, including fees
				transfer(reward.Address, builtin.BurntFundsActorAddr, 20, exitcode.Ok),
			}
		}
		require.NoError(t, m.RewardApplied()(ts, before, after, msg, ret))
		row := m.result.Blocks[i]
		require.Equal(t, block.Cid(), row.Block)
		require.Equal(t, block.Miner, row.Miner)
		require.Equal(t, block.ElectionProof.WinCount, row.WinCount)
		require.Equal(t, big.NewInt(fee), row.Amounts.MessageReward)
	}
	require.Equal(t, v2api.RewardAmounts{
		MintedReward: big.NewInt(150), MinerReward: big.NewInt(130), MessageReward: big.NewInt(18),
		ExplicitReward: big.Zero(), BurnAllocation: big.NewInt(20), MinerPaid: big.NewInt(107), BurnPaid: big.NewInt(63),
	}, m.result.Totals)
	require.Equal(t, reward19.Denom, m.result.Blocks[0].Streams[0].Weight)
	require.Equal(t, 3*reward19.Denom/5, m.result.Blocks[1].Streams[0].Weight)
	require.Equal(t, big.Zero(), m.result.Blocks[1].Amounts.MinerPaid)
}
