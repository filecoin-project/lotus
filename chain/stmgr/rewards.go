package stmgr

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

// RewardDistribution executes ts and reports its block rewards.
func (sm *StateManager) RewardDistribution(ctx context.Context, ts *types.TipSet) (*v2api.RewardDistribution, error) {
	monitor := &rewardDistributionMonitor{
		store: sm.ChainStore().ActorStore(ctx),
		result: v2api.RewardDistribution{
			TipSetKey: ts.Key(),
			Height:    ts.Height(),
			Denom:     reward.Denom,
			Totals:    zeroRewardAmounts(),
			Blocks:    make([]v2api.BlockReward, 0, len(ts.Blocks())),
		},
	}
	// Execute the selected tipset itself, including its due writes and message
	// fees. This also works at head, before a child commits the resulting state.
	if _, err := sm.ExecutionTraceWithMonitor(ctx, ts, monitor); err != nil {
		return nil, xerrors.Errorf("executing tipset rewards: %w", err)
	}
	return &monitor.result, nil
}

// rewardDistributionForAward reconstructs an award from its minting counter
// changes and post-award ledger. The actor has already applied any due writes.
func rewardDistributionForAward(before, after *reward.StreamLedger) ([]v2api.StreamReward, uint64, v2api.RewardAmounts, error) {
	amounts := zeroRewardAmounts()
	amounts.MintedReward = big.Sub(after.TotalMinted, before.TotalMinted)
	explicitDelta := big.Sub(after.TotalExplicitMinted, before.TotalExplicitMinted)
	burnDelta := big.Sub(after.TotalBurnMinted, before.TotalBurnMinted)

	denom := big.NewIntUnsigned(reward.Denom)
	out := make([]v2api.StreamReward, 0, len(after.Streams))
	for _, stream := range after.Streams {
		portion := big.Div(big.Mul(amounts.MintedReward, big.NewIntUnsigned(stream.EvaluatedWeight)), denom)
		row := v2api.StreamReward{ID: uint64(stream.ID), Weight: stream.EvaluatedWeight, Amount: portion}
		if stream.Implicit {
			amounts.MinerReward = big.Add(amounts.MinerReward, portion)
			out = append(out, row)
			continue
		}

		shareTotal := big.NewFromGo(stream.ShareTotal())
		accrued := big.Div(big.Mul(portion, shareTotal), denom)
		// A due writer change may have opened a new period. Recover the pool just
		// before this award from the post-award pool, rather than the old period.
		poolBefore := big.Sub(stream.Accrued, accrued)
		distribution := &v2api.ExplicitRewardDistribution{
			Writer:             stream.Writer,
			Recipients:         make([]v2api.RecipientReward, 0, len(stream.Shares)),
			BurnShare:          stream.ShareBurn().Uint64(),
			BurnAmount:         big.Sub(portion, accrued),
			RoundingAdjustment: accrued,
		}
		for _, share := range stream.Shares {
			shareAmount := big.NewIntUnsigned(share.Share)
			// Earnings are differences of rounded cumulative entitlements. Earlier
			// dust can become earned here, making the rounding adjustment negative.
			earned := big.Sub(
				big.Div(big.Mul(stream.Accrued, shareAmount), shareTotal),
				big.Div(big.Mul(poolBefore, shareAmount), shareTotal),
			)
			distribution.Recipients = append(distribution.Recipients, v2api.RecipientReward{
				Recipient:    share.Recipient,
				Share:        share.Share,
				EarnedAmount: earned,
			})
			distribution.RoundingAdjustment = big.Sub(distribution.RoundingAdjustment, earned)
		}
		row.Distribution = distribution
		amounts.ExplicitReward = big.Add(amounts.ExplicitReward, accrued)
		out = append(out, row)
	}
	amounts.BurnAllocation = big.Sub(amounts.MintedReward, big.Add(amounts.MinerReward, amounts.ExplicitReward))
	if !amounts.ExplicitReward.Equals(explicitDelta) || !amounts.BurnAllocation.Equals(burnDelta) {
		return nil, 0, amounts, fmt.Errorf("reward allocation does not match committed explicit and burn minting counters")
	}
	return out, after.BurnWeight().Uint64(), amounts, nil
}

func zeroRewardAmounts() v2api.RewardAmounts {
	return v2api.RewardAmounts{
		MintedReward:   big.Zero(),
		MinerReward:    big.Zero(),
		MessageReward:  big.Zero(),
		ExplicitReward: big.Zero(),
		BurnAllocation: big.Zero(),
		MinerPaid:      big.Zero(),
		BurnPaid:       big.Zero(),
	}
}

type rewardDistributionMonitor struct {
	store  adt.Store
	result v2api.RewardDistribution
}

var _ ExecMonitor = (*rewardDistributionMonitor)(nil)

func (m *rewardDistributionMonitor) MessageApplied(context.Context, *types.TipSet, cid.Cid, *types.Message, *vm.ApplyRet, bool) error {
	return nil
}

func (m *rewardDistributionMonitor) RewardApplied(ts *types.TipSet, before, after cid.Cid, msg *types.Message, ret *vm.ApplyRet) error {
	// RewardFunc calls this once per block, in tipset order.
	block := ts.Blocks()[len(m.result.Blocks)]
	var params reward.AwardBlockRewardParams
	if err := params.UnmarshalCBOR(bytes.NewReader(msg.Params)); err != nil {
		return xerrors.Errorf("decoding reward parameters: %w", err)
	}
	pre, err := loadRewardLedger(m.store, before, ts.Height())
	if err != nil {
		return xerrors.Errorf("loading pre-award state: %w", err)
	}
	post, err := loadRewardLedger(m.store, after, ts.Height())
	if err != nil {
		return xerrors.Errorf("loading post-award state: %w", err)
	}
	rows, burnWeight, amounts, err := rewardDistributionForAward(pre, post)
	if err != nil {
		return xerrors.Errorf("calculating block %s reward distribution: %w", block.Cid(), err)
	}
	amounts.MessageReward = params.GasReward
	// Only direct, successful f02 transfers are reward payments. A nested burn
	// inside the miner actor can pay an older debt and is not part of BurnPaid.
	for _, call := range ret.ExecutionTrace.Subcalls {
		if !call.MsgRct.ExitCode.IsSuccess() {
			continue
		}
		switch call.Msg.To {
		case params.Miner:
			amounts.MinerPaid = big.Add(amounts.MinerPaid, call.Msg.Value)
		case builtin.BurntFundsActorAddr:
			amounts.BurnPaid = big.Add(amounts.BurnPaid, call.Msg.Value)
		}
	}
	m.result.Blocks = append(m.result.Blocks, v2api.BlockReward{
		Block:      block.Cid(),
		Miner:      block.Miner,
		WinCount:   params.WinCount,
		Amounts:    amounts,
		BurnWeight: burnWeight,
		Streams:    rows,
	})
	addRewardAmounts(&m.result.Totals, amounts)
	return nil
}

func loadRewardLedger(store adt.Store, root cid.Cid, epoch abi.ChainEpoch) (*reward.StreamLedger, error) {
	tree, err := state.LoadStateTree(store, root)
	if err != nil {
		return nil, err
	}
	actor, err := tree.GetActor(reward.Address)
	if err != nil {
		return nil, err
	}
	loaded, err := reward.Load(store, actor)
	if err != nil {
		return nil, err
	}
	return loaded.StreamLedger(epoch)
}

func addRewardAmounts(total *v2api.RewardAmounts, block v2api.RewardAmounts) {
	total.MintedReward = big.Add(total.MintedReward, block.MintedReward)
	total.MinerReward = big.Add(total.MinerReward, block.MinerReward)
	total.MessageReward = big.Add(total.MessageReward, block.MessageReward)
	total.ExplicitReward = big.Add(total.ExplicitReward, block.ExplicitReward)
	total.BurnAllocation = big.Add(total.BurnAllocation, block.BurnAllocation)
	total.MinerPaid = big.Add(total.MinerPaid, block.MinerPaid)
	total.BurnPaid = big.Add(total.BurnPaid, block.BurnPaid)
}
