package common

import (
	"context"

	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
)

type NetAPI interface {
	NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error)
	NetPubsubScores(context.Context) ([]api.PubsubScore, error)
	NetPeers(context.Context) ([]peer.AddrInfo, error)
	NetPeerInfo(_ context.Context, p peer.ID) (*api.ExtendedPeerInfo, error)
	NetConnect(ctx context.Context, p peer.AddrInfo) error
	NetAddrsListen(context.Context) (peer.AddrInfo, error)
	NetDisconnect(ctx context.Context, p peer.ID) error
	NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error)
	NetAutoNatStatus(ctx context.Context) (i api.NatInfo, err error)
	NetAgentVersion(ctx context.Context, p peer.ID) (string, error)
	NetBandwidthStats(ctx context.Context) (metrics.Stats, error)
	NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error)
	NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error)
	Discover(ctx context.Context) (apitypes.OpenRPCDocument, error)
	ID(context.Context) (peer.ID, error)
	NetBlockAdd(ctx context.Context, acl api.NetBlockList) error
	NetBlockRemove(ctx context.Context, acl api.NetBlockList) error
	NetBlockList(ctx context.Context) (api.NetBlockList, error)
}
