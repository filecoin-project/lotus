package full

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagesigner"
)

type RaftAPI struct {
	fx.In

	MessageSigner *messagesigner.MessageSignerConsensus `optional:"true"`
}

func (r *RaftAPI) GetRaftState(ctx context.Context) (*api.RaftStateData, error) {
	if r.MessageSigner == nil {
		return nil, xerrors.Errorf("raft consensus not enabled. Please check your configuration")
	}
	raftState, err := r.MessageSigner.GetRaftState(ctx)
	if err != nil {
		return nil, err
	}
	return &api.RaftStateData{NonceMap: raftState.NonceMap, MsgUuids: raftState.MsgUuids}, nil
}

func (r *RaftAPI) Leader(ctx context.Context) (peer.ID, error) {
	if r.MessageSigner == nil {
		return "", xerrors.Errorf("raft consensus not enabled. Please check your configuration")
	}
	return r.MessageSigner.Leader(ctx)
}

func (r *RaftAPI) IsLeader(ctx context.Context) bool {
	if r.MessageSigner == nil {
		return true
	}
	return r.MessageSigner.IsLeader(ctx)
}

func (r *RaftAPI) RedirectToLeader(ctx context.Context, method string, arg interface{}, ret interface{}) (bool, error) {
	if r.MessageSigner == nil {
		return false, xerrors.Errorf("raft consensus not enabled. Please check your configuration")
	}
	return r.MessageSigner.RedirectToLeader(ctx, method, arg, ret)
}
