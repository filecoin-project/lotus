package chain

import (
	"github.com/filecoin-project/go-address"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	multisig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/chain/types"
)

func (mp *MessageProducer) MultisigConstructor(from, to address.Address, params *multisig.ConstructorParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.Constructor, ser, opts...)
}
func (mp *MessageProducer) MultisigPropose(from, to address.Address, params *multisig.ProposeParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.Propose, ser, opts...)
}
func (mp *MessageProducer) MultisigApprove(from, to address.Address, params *multisig.TxnIDParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.Approve, ser, opts...)
}
func (mp *MessageProducer) MultisigCancel(from, to address.Address, params *multisig.TxnIDParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.Cancel, ser, opts...)
}
func (mp *MessageProducer) MultisigAddSigner(from, to address.Address, params *multisig.AddSignerParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.AddSigner, ser, opts...)
}
func (mp *MessageProducer) MultisigRemoveSigner(from, to address.Address, params *multisig.RemoveSignerParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.RemoveSigner, ser, opts...)
}
func (mp *MessageProducer) MultisigSwapSigner(from, to address.Address, params *multisig.SwapSignerParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.SwapSigner, ser, opts...)
}
func (mp *MessageProducer) MultisigChangeNumApprovalsThreshold(from, to address.Address, params *multisig.ChangeNumApprovalsThresholdParams, opts ...MsgOpt) *types.Message {
	ser := MustSerialize(params)
	return mp.Build(from, to, builtin_spec.MethodsMultisig.ChangeNumApprovalsThreshold, ser, opts...)
}
