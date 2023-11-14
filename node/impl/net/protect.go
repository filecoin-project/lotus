package net

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
)

const apiProtectTag = "api"

func (a *NetAPI) NetProtectAdd(ctx context.Context, peers []peer.ID) error {
	for _, p := range peers {
		a.Host.ConnManager().Protect(p, apiProtectTag)
	}

	return nil
}

func (a *NetAPI) NetProtectRemove(ctx context.Context, peers []peer.ID) error {
	for _, p := range peers {
		a.Host.ConnManager().Unprotect(p, apiProtectTag)
	}

	return nil
}

func (a *NetAPI) NetProtectList(ctx context.Context) (result []peer.ID, err error) {
	for _, conn := range a.Host.Network().Conns() {
		if a.Host.ConnManager().IsProtected(conn.RemotePeer(), apiProtectTag) {
			result = append(result, conn.RemotePeer())
		}
	}

	return
}
