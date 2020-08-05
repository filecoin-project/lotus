package chain

import (
	"github.com/filecoin-project/go-address"
	puppet "github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/types"
)

func (mp *MessageProducer) PuppetConstructor(from, to address.Address, params *adt.EmptyValue, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, puppet.MethodsPuppet.Constructor, ser, opts...)
}
func (mp *MessageProducer) PuppetSend(from, to address.Address, params *puppet.SendParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, puppet.MethodsPuppet.Send, ser, opts...)
}
