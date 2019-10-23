package validation

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

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
	var paramsDec []byte
		paramsDec, err = DecodeParams(method, params...)
		if err != nil {
			return nil, err
		}

	if int(method) >= len(methods) {
		return nil, errors.Errorf("No method name for method %v", method)
	}
	methodId := methods[method]
	msg := &types.Message{
		To:       toDec,
		From:     fromDec,
		Nonce:    nonce,
		Value:    valueDec,
		GasPrice: types.BigInt{gasPrice},
		GasLimit: types.NewInt(uint64(gasLimit)),
		Method:   methodId,
		Params:   paramsDec,
	}
	return msg, nil
}

// FIXME wrote this when I thought we needed the actor code to compute the params, I understand I am wrong now if we use reflection.
func DecodeParams(method chain.MethodID, params ...interface{}) ([]byte, error) {
	if len(params) == 0 {
		return []byte{}, nil
	}
	// TODO use ToEncodedValues() in validation/types.go and remove this switch
	switch method {
	case chain.StoragePowerCreateStorageMiner:
		return decodeStorageMarketParams(method, params...)
	default:
		panic("nyi")
	}
}

func decodeStorageMarketParams(method chain.MethodID, params ...interface{}) ([]byte, error ) {
	switch method {
	case chain.StoragePowerCreateStorageMiner:
		ownerAddr, err := address.NewFromBytes([]byte(params[0].(state.Address)))
		if err != nil {
			return []byte{}, err
		}
		sectorSize := types.BigInt{params[2].(state.BytesAmount)}
		peerID, err := peer.IDFromBytes(params[3].(state.PeerID))
		if err != nil {
			panic(err)
		}
		p := actors.CreateStorageMinerParams{
			Owner:      ownerAddr,
			Worker:     ownerAddr,
			SectorSize: sectorSize,
			PeerID:     peerID,
		}
		return actors.SerializeParams(&p)
	default:
		panic("nyi")
	}
}

func (mf *MessageFactory) FromSingletonAddress(addr state.SingletonActorAddress) (state.Address, error) {
	return fromSingletonAddress(addr)
}

// Maps method enumeration values to method names.
// This will change to a mapping to method ids when method dispatch is updated to use integers.
var methods = []uint64{
	chain.NoMethod: 0,
	chain.InitExec: actors.IAMethods.Exec,
	chain.StoragePowerConstructor: actors.SPAMethods.Constructor,
	chain.StoragePowerCreateStorageMiner: actors.SPAMethods.CreateStorageMiner,
	// More to follow...
}