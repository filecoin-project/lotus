package modules

import (
	"context"

	rpc "github.com/libp2p/go-libp2p-gorpc"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	consensus "github.com/filecoin-project/lotus/lib/consensus/raft"
	"github.com/filecoin-project/lotus/node/impl/full"
)

type RPCHandler struct {
	mpoolAPI full.MpoolAPI
	cons     *consensus.Consensus
}

func NewRPCHandler(mpoolAPI full.MpoolAPI, cons *consensus.Consensus) *RPCHandler {
	return &RPCHandler{mpoolAPI, cons}
}

func (h *RPCHandler) MpoolPushMessage(ctx context.Context, msgWhole *api.MpoolMessageWhole, ret *types.SignedMessage) error {
	signedMsg, err := h.mpoolAPI.MpoolPushMessage(ctx, msgWhole.Msg, msgWhole.Spec)
	if err != nil {
		return err
	}
	*ret = *signedMsg
	return nil
}

func (h *RPCHandler) AddPeer(ctx context.Context, pid peer.ID, ret *struct{}) error {
	return h.cons.AddPeer(ctx, pid)
}

// Add other consensus RPC calls here

func NewRPCClient(host host.Host) *rpc.Client {
	protocolID := protocol.ID("/rpc/lotus-chain/v0")
	return rpc.NewClient(host, protocolID)
}

func NewRPCServer(ctx context.Context, host host.Host, rpcHandler *RPCHandler) error {

	authF := func(pid peer.ID, svc, method string) bool {
		return rpcHandler.cons.IsTrustedPeer(ctx, pid)
	}

	protocolID := protocol.ID("/rpc/lotus-chain/v0")
	rpcServer := rpc.NewServer(host, protocolID, rpc.WithAuthorizeFunc(authF))
	return rpcServer.RegisterName("Consensus", rpcHandler)
}
