package chain

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/types"
)

func (mp *MessageProducer) PowerConstructor(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.Constructor, ser, opts...)
}
func (mp *MessageProducer) PowerCreateMiner(from, to address.Address, params *power.CreateMinerParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.CreateMiner, ser, opts...)
}
func (mp *MessageProducer) PowerUpdateClaimedPower(from, to address.Address, params *power.UpdateClaimedPowerParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.UpdateClaimedPower, ser, opts...)
}
func (mp *MessageProducer) PowerEnrollCronEvent(from, to address.Address, params *power.EnrollCronEventParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.EnrollCronEvent, ser, opts...)
}
func (mp *MessageProducer) PowerOnEpochTickEnd(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.OnEpochTickEnd, ser, opts...)
}
func (mp *MessageProducer) PowerUpdatePledgeTotal(from, to address.Address, params *big.Int, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.UpdatePledgeTotal, ser, opts...)
}
func (mp *MessageProducer) PowerOnConsensusFault(from, to address.Address, params *big.Int, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.OnConsensusFault, ser, opts...)
}
func (mp *MessageProducer) PowerSubmitPoRepForBulkVerify(from, to address.Address, params *abi.SealVerifyInfo, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.SubmitPoRepForBulkVerify, ser, opts...)
}
func (mp *MessageProducer) PowerCurrentTotalPower(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPower.CurrentTotalPower, ser, opts...)
}
