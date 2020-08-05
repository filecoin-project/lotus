package chain

import (
	"github.com/filecoin-project/go-address"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	init_spec "github.com/filecoin-project/specs-actors/actors/builtin/init"
	multisig_spec "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	paych_spec "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	power_spec "github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/chain/types"
)

var noParams []byte

// Transfer builds a simple value transfer message and returns it.
func (mp *MessageProducer) Transfer(from, to address.Address, opts ...MsgOpt) *types.Message {
	return mp.Build(from, to, builtin_spec.MethodSend, noParams, opts...)
}

func (mp *MessageProducer) CreatePaymentChannelActor(from, to address.Address, opts ...MsgOpt) *types.Message {
	return mp.InitExec(from, builtin_spec.InitActorAddr, &init_spec.ExecParams{
		CodeCID: builtin_spec.PaymentChannelActorCodeID,
		ConstructorParams: MustSerialize(&paych_spec.ConstructorParams{
			From: from,
			To:   to,
		}),
	}, opts...)
}

func (mp *MessageProducer) CreateMultisigActor(from address.Address, signers []address.Address, unlockDuration abi_spec.ChainEpoch, numApprovals uint64, opts ...MsgOpt) *types.Message {
	return mp.InitExec(from, builtin_spec.InitActorAddr, &init_spec.ExecParams{
		CodeCID: builtin_spec.MultisigActorCodeID,
		ConstructorParams: MustSerialize(&multisig_spec.ConstructorParams{
			Signers:               signers,
			NumApprovalsThreshold: numApprovals,
			UnlockDuration:        unlockDuration,
		}),
	}, opts...)
}

func (mp *MessageProducer) CreateMinerActor(owner, worker address.Address, sealProofType abi_spec.RegisteredSealProof, pid peer.ID, maddrs []abi_spec.Multiaddrs, opts ...MsgOpt) *types.Message {
	return mp.PowerCreateMiner(owner, builtin_spec.StoragePowerActorAddr, &power_spec.CreateMinerParams{
		Worker:        worker,
		Owner:         owner,
		SealProofType: sealProofType,
		Peer:          abi_spec.PeerID(pid),
		Multiaddrs:    maddrs,
	}, opts...)
}
