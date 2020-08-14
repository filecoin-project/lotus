package builders

import (
	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"

	"github.com/libp2p/go-libp2p-core/peer"
)

type sugarMsg struct{ m *Messages }

// Transfer enlists a value transfer message.
func (s *sugarMsg) Transfer(from, to address.Address, opts ...MsgOpt) *ApplicableMessage {
	return s.m.Typed(from, to, Transfer(), opts...)
}

func (s *sugarMsg) CreatePaychActor(from, to address.Address, opts ...MsgOpt) *ApplicableMessage {
	ctorparams := &paych.ConstructorParams{
		From: from,
		To:   to,
	}
	return s.m.Typed(from, builtin.InitActorAddr, InitExec(&init_.ExecParams{
		CodeCID:           builtin.PaymentChannelActorCodeID,
		ConstructorParams: MustSerialize(ctorparams),
	}), opts...)
}

func (s *sugarMsg) CreateMultisigActor(from address.Address, signers []address.Address, unlockDuration abi.ChainEpoch, numApprovals uint64, opts ...MsgOpt) *ApplicableMessage {
	ctorparams := &multisig.ConstructorParams{
		Signers:               signers,
		NumApprovalsThreshold: numApprovals,
		UnlockDuration:        unlockDuration,
	}

	return s.m.Typed(from, builtin.InitActorAddr, InitExec(&init_.ExecParams{
		CodeCID:           builtin.MultisigActorCodeID,
		ConstructorParams: MustSerialize(ctorparams),
	}), opts...)
}

func (s *sugarMsg) CreateMinerActor(owner, worker address.Address, sealProofType abi.RegisteredSealProof, pid peer.ID, maddrs []abi.Multiaddrs, opts ...MsgOpt) *ApplicableMessage {
	params := &power.CreateMinerParams{
		Worker:        worker,
		Owner:         owner,
		SealProofType: sealProofType,
		Peer:          abi.PeerID(pid),
		Multiaddrs:    maddrs,
	}
	return s.m.Typed(owner, builtin.StoragePowerActorAddr, PowerCreateMiner(params), opts...)
}
