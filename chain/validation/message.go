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
	"github.com/filecoin-project/lotus/chain/actors"
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
		methodId,
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
	vchain.InitExec: actors.IAMethods.Exec,

	vchain.StoragePowerConstructor:        actors.SPAMethods.Constructor,
	vchain.StoragePowerCreateStorageMiner: actors.SPAMethods.CreateStorageMiner,
	vchain.StoragePowerUpdatePower:        actors.SPAMethods.UpdateStorage,

	vchain.StorageMinerUpdatePeerID:  actors.MAMethods.UpdatePeerID,
	vchain.StorageMinerGetOwner:      actors.MAMethods.GetOwner,
	vchain.StorageMinerGetPower:      actors.MAMethods.GetPower,
	vchain.StorageMinerGetWorkerAddr: actors.MAMethods.GetWorkerAddr,
	vchain.StorageMinerGetPeerID:     actors.MAMethods.GetPeerID,
	vchain.StorageMinerGetSectorSize: actors.MAMethods.GetSectorSize,

	vchain.MultiSigConstructor:       actors.MultiSigMethods.MultiSigConstructor,
	vchain.MultiSigPropose:           actors.MultiSigMethods.Propose,
	vchain.MultiSigApprove:           actors.MultiSigMethods.Approve,
	vchain.MultiSigCancel:            actors.MultiSigMethods.Cancel,
	vchain.MultiSigClearCompleted:    actors.MultiSigMethods.ClearCompleted,
	vchain.MultiSigAddSigner:         actors.MultiSigMethods.AddSigner,
	vchain.MultiSigRemoveSigner:      actors.MultiSigMethods.RemoveSigner,
	vchain.MultiSigSwapSigner:        actors.MultiSigMethods.SwapSigner,
	vchain.MultiSigChangeRequirement: actors.MultiSigMethods.ChangeRequirement,
	// More to follow...
}
