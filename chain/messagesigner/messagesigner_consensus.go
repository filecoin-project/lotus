package messagesigner

import (
	"context"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	libp2pconsensus "github.com/libp2p/go-libp2p-consensus"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	consensus "github.com/filecoin-project/lotus/lib/consensus/raft"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type MessageSignerConsensus struct {
	MsgSigner
	consensus *consensus.Consensus
}

func NewMessageSignerConsensus(
	wallet api.Wallet,
	mpool MpoolNonceAPI,
	ds dtypes.MetadataDS,
	consensus *consensus.Consensus) *MessageSignerConsensus {

	ds = namespace.Wrap(ds, datastore.NewKey("/message-signer-consensus/"))
	return &MessageSignerConsensus{
		MsgSigner: &MessageSigner{
			wallet: wallet,
			mpool:  mpool,
			ds:     ds,
		},
		consensus: consensus,
	}
}

func (ms *MessageSignerConsensus) IsLeader(ctx context.Context) bool {
	return ms.consensus.IsLeader(ctx)
}

func (ms *MessageSignerConsensus) RedirectToLeader(ctx context.Context, method string, arg interface{}, ret interface{}) (bool, error) {
	ok, err := ms.consensus.RedirectToLeader(method, arg, ret.(*types.SignedMessage))
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

	// We can't have an empty/default uuid as part of the consensus state so generate a new uuid if spec is empty
	u := uuid.New()
	if spec != nil {
		u = spec.MsgUuid
	}

	op := &consensus.ConsensusOp{signedMsg.Message.Nonce, u, signedMsg.Message.From, signedMsg}
	err = ms.consensus.Commit(ctx, op)
	if err != nil {
		return nil, err
	}

	return signedMsg, nil
}

func (ms *MessageSignerConsensus) GetSignedMessage(ctx context.Context, uuid uuid.UUID) (*types.SignedMessage, error) {
	state, err := ms.consensus.State(ctx)
	if err != nil {
		return nil, err
	}

	cstate := state.(consensus.RaftState)
	msg, ok := cstate.MsgUuids[uuid]
	if !ok {
		return nil, xerrors.Errorf("Msg with Uuid %s not available", uuid)
	}
	return msg, nil
}

func (ms *MessageSignerConsensus) GetRaftState(ctx context.Context) (libp2pconsensus.State, error) {
	return ms.consensus.State(ctx)
}

func (ms *MessageSignerConsensus) Leader(ctx context.Context) (peer.ID, error) {
	return ms.consensus.Leader(ctx)
}
