package messagesigner

import (
	"context"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	consensus "github.com/filecoin-project/lotus/lib/consensus/raft"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type MessageSignerConsensus struct {
	MsgSigner
	Consensus *consensus.Consensus
}

func NewMessageSignerConsensus(
	wallet api.Wallet,
	mpool messagepool.MpoolNonceAPI,
	ds dtypes.MetadataDS,
	consensus *consensus.Consensus) *MessageSignerConsensus {

	ds = namespace.Wrap(ds, datastore.NewKey("/message-signer-consensus/"))
	return &MessageSignerConsensus{
		MsgSigner: &MessageSigner{
			wallet: wallet,
			mpool:  mpool,
			ds:     ds,
		},
		Consensus: consensus,
	}
}

func (ms *MessageSignerConsensus) IsLeader(ctx context.Context) bool {
	return ms.Consensus.IsLeader(ctx)
}

func (ms *MessageSignerConsensus) RedirectToLeader(ctx context.Context, method string, arg interface{}, ret interface{}) (bool, error) {
	ok, err := ms.Consensus.RedirectToLeader(method, arg, ret.(*types.SignedMessage))
	if err != nil {
		return ok, err
	}
	return ok, nil
}

func (ms *MessageSignerConsensus) SignMessage(
	ctx context.Context,
	msg *types.Message,
	spec *api.MessageSendSpec,
	cb func(*types.SignedMessage) error) (*types.SignedMessage, error) {

	signedMsg, err := ms.MsgSigner.SignMessage(ctx, msg, spec, cb)
	if err != nil {
		return nil, err
	}

	op := &consensus.ConsensusOp{
		Nonce:     signedMsg.Message.Nonce,
		Uuid:      spec.MsgUuid,
		Addr:      signedMsg.Message.From,
		SignedMsg: signedMsg,
	}
	err = ms.Consensus.Commit(ctx, op)
	if err != nil {
		return nil, err
	}

	return signedMsg, nil
}

func (ms *MessageSignerConsensus) GetSignedMessage(ctx context.Context, uuid uuid.UUID) (*types.SignedMessage, error) {
	cstate, err := ms.Consensus.State(ctx)
	if err != nil {
		return nil, err
	}

	//cstate := state.(Consensus.RaftState)
	msg, ok := cstate.MsgUuids[uuid]
	if !ok {
		return nil, xerrors.Errorf("Msg with Uuid %s not available", uuid)
	}
	return msg, nil
}

func (ms *MessageSignerConsensus) GetRaftState(ctx context.Context) (*consensus.RaftState, error) {
	return ms.Consensus.State(ctx)
}

func (ms *MessageSignerConsensus) Leader(ctx context.Context) (peer.ID, error) {
	return ms.Consensus.Leader(ctx)
}
