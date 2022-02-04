package idxprov

import (
	"context"
	"fmt"

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
	addrs, err := mc.fullnodeApi.NetAddrsListen(ctx)
	if err != nil {
		return err
	}

	if err := mc.idxProvHost.Connect(ctx, addrs); err != nil {
		return fmt.Errorf("failed to connect index provider host with the full node: %w", err)
	}
	mc.idxProvHost.ConnManager().Protect(addrs.ID, "markets")
	log.Debugw("successfully connected to full node", "fullNodeInfo", addrs.String())

	return nil
}

func NewMeshCreator(fullnodeApi v1api.FullNode, idxProvHost Host) MeshCreator {
	return Libp2pMeshCreator{fullnodeApi, idxProvHost}
}
