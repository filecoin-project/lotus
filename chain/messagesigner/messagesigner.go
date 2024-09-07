package messagesigner

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

const dsKeyActorNonce = "ActorNextNonce"
const dsKeyMsgUUIDSet = "MsgUuidSet"

var log = logging.Logger("messagesigner")

type MsgSigner interface {
	SignMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, cb func(*types.SignedMessage) error) (*types.SignedMessage, error)
	GetSignedMessage(ctx context.Context, uuid uuid.UUID) (*types.SignedMessage, error)
	StoreSignedMessage(ctx context.Context, uuid uuid.UUID, message *types.SignedMessage) error
	NextNonce(ctx context.Context, addr address.Address) (uint64, error)
	SaveNonce(ctx context.Context, addr address.Address, nonce uint64) error
}

// MessageSigner keeps track of nonces per address, and increments the nonce
// when signing a message
type MessageSigner struct {
	wallet api.Wallet
	lk     sync.Mutex
	mpool  messagepool.MpoolNonceAPI
	ds     datastore.Batching
}

func NewMessageSigner(wallet api.Wallet, mpool messagepool.MpoolNonceAPI, ds dtypes.MetadataDS) *MessageSigner {
	ds = namespace.Wrap(ds, datastore.NewKey("/message-signer/"))
	return &MessageSigner{
		wallet: wallet,
		mpool:  mpool,
		ds:     ds,
	}
}

// SignMessage increments the nonce for the message From address, and signs
// the message
func (ms *MessageSigner) SignMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec, cb func(*types.SignedMessage) error) (*types.SignedMessage, error) {
	ms.lk.Lock()
	defer ms.lk.Unlock()

	// Get the next message nonce
	nonce, err := ms.NextNonce(ctx, msg.From)
	if err != nil {
		return nil, xerrors.Errorf("failed to create nonce: %w", err)
	}

	// Sign the message with the nonce
	msg.Nonce = nonce

	sb, err := SigningBytes(msg, msg.From.Protocol())
	if err != nil {
		return nil, err
	}
	mb, err := msg.ToStorageBlock()
	if err != nil {
		return nil, xerrors.Errorf("serializing message: %w", err)
	}

	sig, err := ms.wallet.WalletSign(ctx, msg.From, sb, api.MsgMeta{
		Type:  api.MTChainMsg,
		Extra: mb.RawData(),
	})

	if err != nil {
		return nil, xerrors.Errorf("failed to sign message: %w, addr=%s", err, msg.From)
	}

	// Callback with the signed message
	smsg := &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}

	err = cb(smsg)
	if err != nil {
		return nil, err
	}

	// If the callback executed successfully, write the nonce to the datastore
	if err := ms.SaveNonce(ctx, msg.From, nonce); err != nil {
		return nil, xerrors.Errorf("failed to save nonce: %w", err)
	}

	return smsg, nil
}

func (ms *MessageSigner) GetSignedMessage(ctx context.Context, uuid uuid.UUID) (*types.SignedMessage, error) {

	key := datastore.KeyWithNamespaces([]string{dsKeyMsgUUIDSet, uuid.String()})
	bytes, err := ms.ds.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return types.DecodeSignedMessage(bytes)
}

func (ms *MessageSigner) StoreSignedMessage(ctx context.Context, uuid uuid.UUID, message *types.SignedMessage) error {

	key := datastore.KeyWithNamespaces([]string{dsKeyMsgUUIDSet, uuid.String()})
	serializedMsg, err := message.Serialize()
	if err != nil {
		return err
	}
	return ms.ds.Put(ctx, key, serializedMsg)
}

// NextNonce gets the next nonce for the given address.
// If there is no nonce in the datastore, gets the nonce from the message pool.
func (ms *MessageSigner) NextNonce(ctx context.Context, addr address.Address) (uint64, error) {
	// Nonces used to be created by the mempool and we need to support nodes
	// that have mempool nonces, so first check the mempool for a nonce for
	// this address. Note that the mempool returns the actor state's nonce
	// by default.
	nonce, err := ms.mpool.GetNonce(ctx, addr, types.EmptyTSK)
	if err != nil {
		return 0, xerrors.Errorf("failed to get nonce from mempool: %w", err)
	}

	// Get the next nonce for this address from the datastore
	addrNonceKey := ms.dstoreKey(addr)
	dsNonceBytes, err := ms.ds.Get(ctx, addrNonceKey)

	switch {
	case errors.Is(err, datastore.ErrNotFound):
		// If a nonce for this address hasn't yet been created in the
		// datastore, just use the nonce from the mempool
		return nonce, nil

	case err != nil:
		return 0, xerrors.Errorf("failed to get nonce from datastore: %w", err)

	default:
		// There is a nonce in the datastore, so unmarshall it
		maj, dsNonce, err := cbg.CborReadHeader(bytes.NewReader(dsNonceBytes))
		if err != nil {
			return 0, xerrors.Errorf("failed to parse nonce from datastore: %w", err)
		}
		if maj != cbg.MajUnsignedInt {
			return 0, xerrors.Errorf("bad cbor type parsing nonce from datastore")
		}

		// The message pool nonce should be <= than the datastore nonce
		if nonce <= dsNonce {
			nonce = dsNonce
		} else {
			log.Warnf("mempool nonce was larger than datastore nonce (%d > %d)", nonce, dsNonce)
		}

		return nonce, nil
	}
}

// SaveNonce increments the nonce for this address and writes it to the
// datastore
func (ms *MessageSigner) SaveNonce(ctx context.Context, addr address.Address, nonce uint64) error {
	// Increment the nonce
	nonce++

	// Write the nonce to the datastore
	addrNonceKey := ms.dstoreKey(addr)
	buf := bytes.Buffer{}
	_, err := buf.Write(cbg.CborEncodeMajorType(cbg.MajUnsignedInt, nonce))
	if err != nil {
		return xerrors.Errorf("failed to marshall nonce: %w", err)
	}
	err = ms.ds.Put(ctx, addrNonceKey, buf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to write nonce to datastore: %w", err)
	}
	return nil
}

func (ms *MessageSigner) dstoreKey(addr address.Address) datastore.Key {
	return datastore.KeyWithNamespaces([]string{dsKeyActorNonce, addr.String()})
}

func SigningBytes(msg *types.Message, sigType address.Protocol) ([]byte, error) {
	if sigType == address.Delegated {
		txArgs, err := ethtypes.Eth1559TxArgsFromUnsignedFilecoinMessage(msg)
		if err != nil {
			return nil, xerrors.Errorf("failed to reconstruct eth transaction: %w", err)
		}
		rlpEncodedMsg, err := txArgs.ToRlpUnsignedMsg()
		if err != nil {
			return nil, xerrors.Errorf("failed to repack eth rlp message: %w", err)
		}
		return rlpEncodedMsg, nil
	}

	return msg.Cid().Bytes(), nil
}
