package validation

import (
	"context"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/chain-validation/pkg/chain"
	"github.com/filecoin-project/chain-validation/pkg/state"
)

type Signer interface {
	Sign(ctx context.Context, addr address.Address, msg []byte) (*types.Signature, error)
}

type MessageFactory struct {
	signer Signer
}

var _ chain.MessageFactory = &MessageFactory{}

func NewMessageFactory(signer Signer) *MessageFactory {
	return &MessageFactory{signer}
}

func (mf *MessageFactory) MakeMessage(from, to state.Address, method chain.MethodID, nonce uint64, value, gasPrice state.AttoFIL, gasLimit state.GasUnit, params ...interface{}) (interface{}, error) {
	fromDec, err := address.NewFromBytes([]byte(from))
	if err != nil {
		return nil, err
	}
	toDec, err := address.NewFromBytes([]byte(to))
	if err != nil {
		return nil, err
	}
	valueDec := types.BigInt{value}
	paramsDec, err := state.EncodeValues(params...)
	if err != nil {
		return nil, err
	}

	if int(method) >= len(methods) {
		return nil, xerrors.Errorf("No method name for method %v", method)
	}
	methodId := methods[method]
	msg := &types.Message{
		toDec,
		fromDec,
		nonce,
		valueDec,
		types.BigInt{gasPrice},
		types.NewInt(uint64(gasLimit)),
		methodId,
		paramsDec,
	}

	return msg, nil
}

func (mf *MessageFactory) FromSingletonAddress(addr state.SingletonActorID) state.Address {
	return fromSingletonAddress(addr)
}

// Maps method enumeration values to method names.
// This will change to a mapping to method ids when method dispatch is updated to use integers.
var methods = []uint64{
	chain.NoMethod: 0,
	chain.InitExec: actors.IAMethods.Exec,

	chain.StoragePowerConstructor:        actors.SPAMethods.Constructor,
	chain.StoragePowerCreateStorageMiner: actors.SPAMethods.CreateStorageMiner,
	chain.StoragePowerUpdatePower:        actors.SPAMethods.UpdateStorage,

	chain.StorageMinerUpdatePeerID:  actors.MAMethods.UpdatePeerID,
	chain.StorageMinerGetOwner:      actors.MAMethods.GetOwner,
	chain.StorageMinerGetPower:      actors.MAMethods.GetPower,
	chain.StorageMinerGetWorkerAddr: actors.MAMethods.GetWorkerAddr,
	chain.StorageMinerGetPeerID:     actors.MAMethods.GetPeerID,
	chain.StorageMinerGetSectorSize: actors.MAMethods.GetSectorSize,
	// More to follow...
}
