package validation

import (
	"context"
	"github.com/libp2p/go-libp2p-core/peer"
	"math/big"

	"github.com/pkg/errors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"

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
	//paramsDec, err := []byte{}, nil // FIXME encode params as CBOR tuple byte[] using reflection
	paramsDec, err := decodeMessageParams(method, params...)
	if err != nil {
		return nil, err
	}

	if int(method) >= len(methods) {
		return nil, errors.Errorf("No method name for method %v", method)
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

func (mf *MessageFactory) FromSingletonAddress(addr state.SingletonActorID) (state.Address) {
	return fromSingletonAddress(addr)
}

// Maps method enumeration values to method names.
// This will change to a mapping to method ids when method dispatch is updated to use integers.
var methods = []uint64{
	chain.NoMethod: 0,
	chain.InitExec: actors.IAMethods.Exec,
	chain.StoragePowerConstructor: actors.SPAMethods.Constructor,
	chain.StoragePowerCreateStorageMiner: actors.SPAMethods.CreateStorageMiner,
	chain.StoragePowerUpdatePower: actors.SPAMethods.UpdateStorage,
	// More to follow...
}

func decodeMessageParams(methodID chain.MethodID, params ...interface{}) ([]byte, error){
	if len(params) == 0 {
		return []byte{}, nil
	}
	switch methodID {
	case chain.StoragePowerCreateStorageMiner:
		if len(params) != 4 {
			return []byte{}, errors.Errorf("not enough params for methodID %d expected %d, got %d", methodID, 4, len(params))
		}
		ownerAddr, err := address.NewFromBytes([]byte(params[0].(state.Address)))
		if err != nil {
			panic(err)
		}
		rawSector := params[2].(state.BytesAmount)
		sectorSize := big.Int(*rawSector)
		rawPeer := params[3].(state.PeerID)
		peerID, err := peer.IDFromBytes(rawPeer)
		if err != nil {
			panic(err)
		}
		return actors.SerializeParams(&actors.CreateStorageMinerParams{
			Owner:      ownerAddr,
			Worker:     ownerAddr,
			SectorSize: sectorSize.Uint64(),
			PeerID:     peerID,
		})
	case chain.StoragePowerUpdatePower:
		if len(params) != 1 {
			return []byte{}, errors.Errorf("not enough params for methodID %d expected %d, got %d", methodID, 1, len(params))
		}
		rawDelta := params[0].(state.BytesAmount)
		delta := big.Int(*rawDelta)
		return actors.SerializeParams(&actors.UpdateStorageParams{Delta:types.BigInt{&delta}})
	default:
		panic("not handled")
	}

}