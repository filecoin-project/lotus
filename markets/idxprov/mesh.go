package idxprov

import (
	"context"
	"fmt"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/lotus/api/v1api"
)

var log = logging.Logger("idxprov")

const protectTag = "index-provider-gossipsub"

type MeshCreator interface {
	Connect(ctx context.Context) error
}

type Libp2pMeshCreator struct {
	fullnodeApi v1api.FullNode
	marketsHost host.Host
}

func (mc Libp2pMeshCreator) Connect(ctx context.Context) error {

	// Add the markets host ID to list of daemon's protected peers first, before any attempt to
	// connect to full node over libp2p.
	marketsPeerID := mc.marketsHost.ID()
	if err := mc.fullnodeApi.NetProtectAdd(ctx, []peer.ID{marketsPeerID}); err != nil {
		return fmt.Errorf("failed to call NetProtectAdd on the full node, err: %w", err)
	}

	faddrs, err := mc.fullnodeApi.NetAddrsListen(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch full node listen addrs, err: %w", err)
	}

	// Connect from the full node, ask it to protect the connection and protect the connection on
	// markets end too. Connection is initiated form full node to avoid the need to expose libp2p port on full node
	if err := mc.fullnodeApi.NetConnect(ctx, peer.AddrInfo{
		ID:    mc.marketsHost.ID(),
		Addrs: mc.marketsHost.Addrs(),
	}); err != nil {
		return fmt.Errorf("failed to connect to index provider host from full node: %w", err)
	}
	mc.marketsHost.ConnManager().Protect(faddrs.ID, protectTag)

	log.Debugw("successfully connected to full node and asked it protect indexer provider peer conn", "fullNodeInfo", faddrs.String(),
		"peerId", marketsPeerID)

	return nil
}

func NewMeshCreator(fullnodeApi v1api.FullNode, marketsHost host.Host) MeshCreator {
	return Libp2pMeshCreator{fullnodeApi, marketsHost}
}
