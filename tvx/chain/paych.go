package chain

import (
	"github.com/filecoin-project/go-address"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/types"
)

func (mp *MessageProducer) PaychConstructor(from, to address.Address, params *paych.ConstructorParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPaych.Constructor, ser, opts...)
}
func (mp *MessageProducer) PaychUpdateChannelState(from, to address.Address, params *paych.UpdateChannelStateParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPaych.UpdateChannelState, ser, opts...)
}
func (mp *MessageProducer) PaychSettle(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPaych.Settle, ser, opts...)
}
func (mp *MessageProducer) PaychCollect(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsPaych.Collect, ser, opts...)
}
