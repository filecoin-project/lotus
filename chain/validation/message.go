package validation

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	vchain "github.com/filecoin-project/chain-validation/pkg/chain"
	vactors "github.com/filecoin-project/chain-validation/pkg/state/actors"
	vaddress "github.com/filecoin-project/chain-validation/pkg/state/address"
	vtypes "github.com/filecoin-project/chain-validation/pkg/state/types"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/chain/types"
)

type Signer interface {
	Sign(ctx context.Context, addr vaddress.Address, msg []byte) (*types.Signature, error)
}

type MessageFactory struct {
	signer Signer
}

var _ vchain.MessageFactory = &MessageFactory{}

func NewMessageFactory(signer Signer) *MessageFactory {
	return &MessageFactory{signer}
}

func (mf *MessageFactory) MakeMessage(from, to vaddress.Address, method vchain.MethodID, nonce uint64, value, gasPrice vtypes.BigInt, gasLimit vtypes.GasUnit, params []byte) (interface{}, error) {
	fromDec, err := address.NewFromBytes(from.Bytes())
	if err != nil {
		return nil, err
	}
	toDec, err := address.NewFromBytes(to.Bytes())
	if err != nil {
		return nil, err
	}
	valueDec := types.BigInt{value.Int}

	if int(method) >= len(methods) {
		return nil, xerrors.Errorf("No method name for method %v", method)
	}
	methodId := methods[method]
	msg := &types.Message{
		toDec,
		fromDec,
		nonce,
		valueDec,
		types.BigInt{gasPrice.Int},
		types.NewInt(uint64(gasLimit)),
		abi.MethodNum(methodId),
		params,
	}

	return msg, nil
}

func (mf *MessageFactory) FromSingletonAddress(addr vactors.SingletonActorID) vaddress.Address {
	return fromSingletonAddress(addr)
}

func (mf *MessageFactory) FromActorCodeCid(code vactors.ActorCodeID) cid.Cid {
	return fromActorCode(code)
}

// Maps method enumeration values to method names.
// This will change to a mapping to method ids when method dispatch is updated to use integers.
var methods = []uint64{
	vchain.NoMethod: 0,
	vchain.InitExec: uint64(builtin.MethodsInit.Exec),

	vchain.StoragePowerConstructor:        uint64(builtin.MethodsPower.Constructor),
	vchain.StoragePowerCreateStorageMiner: uint64(builtin.MethodsPower.CreateMiner),

	vchain.StorageMinerUpdatePeerID:  0, //actors.MAMethods.UpdatePeerID,
	vchain.StorageMinerGetOwner:      0, //actors.MAMethods.GetOwner,
	vchain.StorageMinerGetPower:      0, //actors.MAMethods.GetPower,
	vchain.StorageMinerGetWorkerAddr: 0, //actors.MAMethods.GetWorkerAddr,
	vchain.StorageMinerGetPeerID:     0, //actors.MAMethods.GetPeerID,
	vchain.StorageMinerGetSectorSize: 0, //actors.MAMethods.GetSectorSize,

	vchain.MultiSigConstructor:       uint64(builtin.MethodsMultisig.Constructor),
	vchain.MultiSigPropose:           uint64(builtin.MethodsMultisig.Propose),
	vchain.MultiSigApprove:           uint64(builtin.MethodsMultisig.Approve),
	vchain.MultiSigCancel:            uint64(builtin.MethodsMultisig.Cancel),
	vchain.MultiSigClearCompleted:    uint64(builtin.MethodsMultisig.ClearCompleted),
	vchain.MultiSigAddSigner:         uint64(builtin.MethodsMultisig.AddSigner),
	vchain.MultiSigRemoveSigner:      uint64(builtin.MethodsMultisig.RemoveSigner),
	vchain.MultiSigSwapSigner:        uint64(builtin.MethodsMultisig.SwapSigner),
	vchain.MultiSigChangeRequirement: uint64(builtin.MethodsMultisig.ChangeNumApprovalsThreshold),
	// More to follow...
}
