package chain

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/types"
)

func (mp *MessageProducer) RewardConstructor(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsReward.Constructor, ser, opts...)
}
func (mp *MessageProducer) RewardAwardBlockReward(from, to address.Address, params *reward.AwardBlockRewardParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsReward.AwardBlockReward, ser, opts...)
}
func (mp *MessageProducer) RewardLastPerEpochReward(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsReward.ThisEpochReward, ser, opts...)
}
func (mp *MessageProducer) RewardUpdateNetworkKPI(from, to address.Address, params *big.Int, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsReward.UpdateNetworkKPI, ser, opts...)
}
