package idxprov

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/api/v1api"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("idxprov")

type MeshCreator interface {
	Connect(ctx context.Context) error
}

type Libp2pMeshCreator struct {
	fullnodeApi v1api.FullNode
	idxProvHost Host
}

func (mc Libp2pMeshCreator) Connect(ctx context.Context) error {

	// Add the index provider's host ID to list of protected peers first, before any attempt to
	// connect to full node.
	idxProvID := mc.idxProvHost.ID()
	if err := mc.fullnodeApi.NetProtectAdd(ctx, []peer.ID{idxProvID}); err != nil {
		return fmt.Errorf("failed to call NetProtectAdd on the full node, err: %w", err)
	}

	faddrs, err := mc.fullnodeApi.NetAddrsListen(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch full node listen addrs, err: %w", err)
	}

	// Connect to the full node, ask it to protect the connection and protect the connection on
	// markets end too.
	if err := mc.idxProvHost.Connect(ctx, faddrs); err != nil {
		return fmt.Errorf("failed to connect index provider host with the full node: %w", err)
	}
	mc.idxProvHost.ConnManager().Protect(faddrs.ID, "index-provider-gossipsub")

	log.Debugw("successfully connected to full node and asked it protect indexer provider peer conn", "fullNodeInfo", faddrs.String(),
		"idxProviderPeerId", idxProvID)

	return nil
}

func NewMeshCreator(fullnodeApi v1api.FullNode, idxProvHost Host) MeshCreator {
	return Libp2pMeshCreator{fullnodeApi, idxProvHost}
}
