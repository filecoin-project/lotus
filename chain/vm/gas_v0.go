package vm

import (
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/runtime/proof"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
)

type scalingCost struct {
	flat  int64
	scale int64
}

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
	onChainMessageComputeBase    int64
	onChainMessageStorageBase    int64
	onChainMessageStoragePerByte int64

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

	// Gsa cost charged, in addition to SendBase, if message only transfers funds.
	sendTransferOnlyPremium int64

	// Gas cost charged, in addition to SendBase, if a message invokes
	// a method on the receiver.
	// Accounts for the cost of loading receiver code and method dispatch.
	sendInvokeMethod int64

	// Gas cost for any Get operation to the IPLD store
	// in the runtime VM context.
	ipldGetBase int64

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
	createActorCompute int64
	createActorStorage int64

	// Gas cost for deleting an actor.
	//
	// Note: this partially refunds the create cost to incentivise the deletion of the actors.
	deleteActor int64

	verifySignature map[crypto.SigType]int64

	hashingBase int64

	computeUnsealedSectorCidBase int64
	verifySealBase               int64
	verifyPostLookup             map[abi.RegisteredPoStProof]scalingCost
	verifyConsensusFault         int64
}

var _ Pricelist = (*pricelistV0)(nil)

// OnChainMessage returns the gas used for storing a message of a given size in the chain.
func (pl *pricelistV0) OnChainMessage(msgSize int) GasCharge {
	return newGasCharge("OnChainMessage", pl.onChainMessageComputeBase,
		pl.onChainMessageStorageBase+pl.onChainMessageStoragePerByte*int64(msgSize))
}

// OnChainReturnValue returns the gas used for storing the response of a message in the chain.
func (pl *pricelistV0) OnChainReturnValue(dataSize int) GasCharge {
	return newGasCharge("OnChainReturnValue", 0, int64(dataSize)*pl.onChainReturnValuePerByte)
}

// OnMethodInvocation returns the gas used when invoking a method.
func (pl *pricelistV0) OnMethodInvocation(value abi.TokenAmount, methodNum abi.MethodNum) GasCharge {
	ret := pl.sendBase
	extra := ""

	if big.Cmp(value, abi.NewTokenAmount(0)) != 0 {
		ret += pl.sendTransferFunds
		if methodNum == builtin0.MethodSend {
			// transfer only
			ret += pl.sendTransferOnlyPremium
		}
		extra += "t"
	}

	if methodNum != builtin0.MethodSend {
		extra += "i"
		// running actors is cheaper becase we hand over to actors
		ret += pl.sendInvokeMethod
	}
	return newGasCharge("OnMethodInvocation", ret, 0).WithExtra(extra)
}

// OnIpldGet returns the gas used for storing an object
func (pl *pricelistV0) OnIpldGet() GasCharge {
	return newGasCharge("OnIpldGet", pl.ipldGetBase, 0)
}

// OnIpldPut returns the gas used for storing an object
func (pl *pricelistV0) OnIpldPut(dataSize int) GasCharge {
	return newGasCharge("OnIpldPut", pl.ipldPutBase, int64(dataSize)*pl.ipldPutPerByte).
		WithExtra(dataSize)
}

// OnCreateActor returns the gas used for creating an actor
func (pl *pricelistV0) OnCreateActor() GasCharge {
	return newGasCharge("OnCreateActor", pl.createActorCompute, pl.createActorStorage)
}

// OnDeleteActor returns the gas used for deleting an actor
func (pl *pricelistV0) OnDeleteActor() GasCharge {
	return newGasCharge("OnDeleteActor", 0, pl.deleteActor)
}

// OnVerifySignature

func (pl *pricelistV0) OnVerifySignature(sigType crypto.SigType, planTextSize int) (GasCharge, error) {
	cost, ok := pl.verifySignature[sigType]
	if !ok {
		return GasCharge{}, fmt.Errorf("cost function for signature type %d not supported", sigType)
	}

	sigName, _ := sigType.Name()
	return newGasCharge("OnVerifySignature", cost, 0).
		WithExtra(map[string]interface{}{
			"type": sigName,
			"size": planTextSize,
		}), nil
}

// OnHashing
func (pl *pricelistV0) OnHashing(dataSize int) GasCharge {
	return newGasCharge("OnHashing", pl.hashingBase, 0).WithExtra(dataSize)
}

// OnComputeUnsealedSectorCid
func (pl *pricelistV0) OnComputeUnsealedSectorCid(proofType abi.RegisteredSealProof, pieces []abi.PieceInfo) GasCharge {
	return newGasCharge("OnComputeUnsealedSectorCid", pl.computeUnsealedSectorCidBase, 0)
}

// OnVerifySeal
func (pl *pricelistV0) OnVerifySeal(info proof.SealVerifyInfo) GasCharge {
	// TODO: this needs more cost tunning, check with @lotus
	// this is not used
	return newGasCharge("OnVerifySeal", pl.verifySealBase, 0)
}

// OnVerifyPost
func (pl *pricelistV0) OnVerifyPost(info proof.WindowPoStVerifyInfo) GasCharge {
	sectorSize := "unknown"
	var proofType abi.RegisteredPoStProof

	if len(info.Proofs) != 0 {
		proofType = info.Proofs[0].PoStProof
		ss, err := info.Proofs[0].PoStProof.SectorSize()
		if err == nil {
			sectorSize = ss.ShortString()
		}
	}

	cost, ok := pl.verifyPostLookup[proofType]
	if !ok {
		cost = pl.verifyPostLookup[abi.RegisteredPoStProof_StackedDrgWindow512MiBV1]
	}

	gasUsed := cost.flat + int64(len(info.ChallengedSectors))*cost.scale
	gasUsed /= 2 // XXX: this is an artificial discount

	return newGasCharge("OnVerifyPost", gasUsed, 0).
		WithExtra(map[string]interface{}{
			"type": sectorSize,
			"size": len(info.ChallengedSectors),
		})
}

// OnVerifyConsensusFault
func (pl *pricelistV0) OnVerifyConsensusFault() GasCharge {
	return newGasCharge("OnVerifyConsensusFault", pl.verifyConsensusFault, 0)
}
