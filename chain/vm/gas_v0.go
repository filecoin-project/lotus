package vm

import (
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/crypto"
)

type pricelistV0 struct {
	///////////////////////////////////////////////////////////////////////////
	// System operations
	///////////////////////////////////////////////////////////////////////////

	// Gas cost charged to the originator of an on-chain message (regardless of
	// whether it succeeds or fails in application) is given by:
	//   OnChainMessageBase + len(serialized message)*OnChainMessagePerByte
	// Together, these account for the cost of message propagation and validation,
	// up to but excluding any actual processing by the VM.
	// This is the cost a block producer burns when including an invalid message.
	onChainMessageBase    int64
	onChainMessagePerByte int64

	// Gas cost charged to the originator of a non-nil return value produced
	// by an on-chain message is given by:
	//   len(return value)*OnChainReturnValuePerByte
	onChainReturnValuePerByte int64

	// Gas cost for any message send execution(including the top-level one
	// initiated by an on-chain message).
	// This accounts for the cost of loading sender and receiver actors and
	// (for top-level messages) incrementing the sender's sequence number.
	// Load and store of actor sub-state is charged separately.
	sendBase int64

	// Gas cost charged, in addition to SendBase, if a message send
	// is accompanied by any nonzero currency amount.
	// Accounts for writing receiver's new balance (the sender's state is
	// already accounted for).
	sendTransferFunds int64

	// Gas cost charged, in addition to SendBase, if a message invokes
	// a method on the receiver.
	// Accounts for the cost of loading receiver code and method dispatch.
	sendInvokeMethod int64

	// Gas cost (Base + len*PerByte) for any Get operation to the IPLD store
	// in the runtime VM context.
	ipldGetBase    int64
	ipldGetPerByte int64

	// Gas cost (Base + len*PerByte) for any Put operation to the IPLD store
	// in the runtime VM context.
	//
	// Note: these costs should be significantly higher than the costs for Get
	// operations, since they reflect not only serialization/deserialization
	// but also persistent storage of chain data.
	ipldPutBase    int64
	ipldPutPerByte int64

	// Gas cost for creating a new actor (via InitActor's Exec method).
	//
	// Note: this costs assume that the extra will be partially or totally refunded while
	// the base is covering for the put.
	createActorBase  int64
	createActorExtra int64

	// Gas cost for deleting an actor.
	//
	// Note: this partially refunds the create cost to incentivise the deletion of the actors.
	deleteActor int64

	verifySignature map[crypto.SigType]func(len int64) int64

	hashingBase    int64
	hashingPerByte int64

	computeUnsealedSectorCidBase int64
	verifySealBase               int64
	verifyPostBase               int64
	verifyConsensusFault         int64
}

var _ Pricelist = (*pricelistV0)(nil)

// OnChainMessage returns the gas used for storing a message of a given size in the chain.
func (pl *pricelistV0) OnChainMessage(msgSize int) GasCharge {
	return newGasCharge("OnChainMessage", 0, pl.onChainMessageBase+pl.onChainMessagePerByte*int64(msgSize))
}

// OnChainReturnValue returns the gas used for storing the response of a message in the chain.
func (pl *pricelistV0) OnChainReturnValue(dataSize int) GasCharge {
	return newGasCharge("OnChainReturnValue", 0, int64(dataSize)*pl.onChainReturnValuePerByte)
}

// OnMethodInvocation returns the gas used when invoking a method.
func (pl *pricelistV0) OnMethodInvocation(value abi.TokenAmount, methodNum abi.MethodNum) GasCharge {
	ret := pl.sendBase
	if value != abi.NewTokenAmount(0) {
		ret += pl.sendTransferFunds
	}
	if methodNum != builtin.MethodSend {
		ret += pl.sendInvokeMethod
	}
	return newGasCharge("OnMethodInvocation", ret, 0)
}

// OnIpldGet returns the gas used for storing an object
func (pl *pricelistV0) OnIpldGet(dataSize int) GasCharge {
	return newGasCharge("OnIpldGet", pl.ipldGetBase+int64(dataSize)*pl.ipldGetPerByte, 0).WithExtra(dataSize)
}

// OnIpldPut returns the gas used for storing an object
func (pl *pricelistV0) OnIpldPut(dataSize int) GasCharge {
	return newGasCharge("OnIpldPut", pl.ipldPutBase, int64(dataSize)*pl.ipldPutPerByte).WithExtra(dataSize)
}

// OnCreateActor returns the gas used for creating an actor
func (pl *pricelistV0) OnCreateActor() GasCharge {
	return newGasCharge("OnCreateActor", pl.createActorBase, pl.createActorExtra)
}

// OnDeleteActor returns the gas used for deleting an actor
func (pl *pricelistV0) OnDeleteActor() GasCharge {
	return newGasCharge("OnDeleteActor", 0, pl.deleteActor)
}

// OnVerifySignature
func (pl *pricelistV0) OnVerifySignature(sigType crypto.SigType, planTextSize int) (GasCharge, error) {
	costFn, ok := pl.verifySignature[sigType]
	if !ok {
		return GasCharge{}, fmt.Errorf("cost function for signature type %d not supported", sigType)
	}
	sigName, _ := sigType.Name()
	return newGasCharge("OnVerifySignature", costFn(int64(planTextSize)), 0).WithExtra(sigName), nil
}

// OnHashing
func (pl *pricelistV0) OnHashing(dataSize int) GasCharge {
	return newGasCharge("OnHashing", pl.hashingBase+int64(dataSize)*pl.hashingPerByte, 0)
}

// OnComputeUnsealedSectorCid
func (pl *pricelistV0) OnComputeUnsealedSectorCid(proofType abi.RegisteredSealProof, pieces []abi.PieceInfo) GasCharge {
	// TODO: this needs more cost tunning, check with @lotus
	return newGasCharge("OnComputeUnsealedSectorCid", pl.computeUnsealedSectorCidBase, 0)
}

// OnVerifySeal
func (pl *pricelistV0) OnVerifySeal(info abi.SealVerifyInfo) GasCharge {
	// TODO: this needs more cost tunning, check with @lotus
	return newGasCharge("OnVerifySeal", pl.verifySealBase, 0)
}

// OnVerifyPost
func (pl *pricelistV0) OnVerifyPost(info abi.WindowPoStVerifyInfo) GasCharge {
	// TODO: this needs more cost tunning, check with @lotus
	return newGasCharge("OnVerifyPost", pl.verifyPostBase, 0)
}

// OnVerifyConsensusFault
func (pl *pricelistV0) OnVerifyConsensusFault() GasCharge {
	return newGasCharge("OnVerifyConsensusFault", pl.verifyConsensusFault, 0)
}
