package chain

import (
	"github.com/filecoin-project/go-address"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"

	"github.com/filecoin-project/lotus/chain/types"
)

func (mp *MessageProducer) InitConstructor(from, to address.Address, params *init_.ConstructorParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsInit.Constructor, ser, opts...)
}
func (mp *MessageProducer) InitExec(from, to address.Address, params *init_.ExecParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsInit.Exec, ser, opts...)
}
