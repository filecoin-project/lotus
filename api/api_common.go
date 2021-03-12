package api

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/filecoin-project/go-jsonrpc/auth"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
)

type Common interface {

	// MethodGroup: Auth

	AuthVerify(ctx context.Context, token string) ([]auth.Permission, error)
	AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error)

	// MethodGroup: Net

	NetConnectedness(context.Context, peer.ID) (network.Connectedness, error)
	NetPeers(context.Context) ([]peer.AddrInfo, error)
	NetConnect(context.Context, peer.AddrInfo) error
	NetAddrsListen(context.Context) (peer.AddrInfo, error)
	NetDisconnect(context.Context, peer.ID) error
	NetFindPeer(context.Context, peer.ID) (peer.AddrInfo, error)
	NetPubsubScores(context.Context) ([]PubsubScore, error)
	NetAutoNatStatus(context.Context) (NatInfo, error)
	NetAgentVersion(ctx context.Context, p peer.ID) (string, error)
	NetPeerInfo(context.Context, peer.ID) (*ExtendedPeerInfo, error)

	// NetBandwidthStats returns statistics about the nodes total bandwidth
	// usage and current rate across all peers and protocols.
	NetBandwidthStats(ctx context.Context) (metrics.Stats, error)

	// NetBandwidthStatsByPeer returns statistics about the nodes bandwidth
	// usage and current rate per peer
	NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error)

	// NetBandwidthStatsByProtocol returns statistics about the nodes bandwidth
	// usage and current rate per protocol
	NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error)

	// ConnectionGater API
	NetBlockAdd(ctx context.Context, acl NetBlockList) error
	NetBlockRemove(ctx context.Context, acl NetBlockList) error
	NetBlockList(ctx context.Context) (NetBlockList, error)

	// MethodGroup: Common

	// ID returns peerID of libp2p node backing this API
	ID(context.Context) (peer.ID, error)

	// Version provides information about API provider
	Version(context.Context) (APIVersion, error)

	LogList(context.Context) ([]string, error)
	LogSetLevel(context.Context, string, string) error

	// trigger graceful shutdown
	Shutdown(context.Context) error

	// Session returns a random UUID of api provider session
	Session(context.Context) (uuid.UUID, error)

	Closing(context.Context) (<-chan struct{}, error)
}

// APIVersion provides various build-time information
type APIVersion struct {
	Version string

	// APIVersion is a binary encoded semver version of the remote implementing
	// this api
	//
	// See APIVersion in build/version.go
	APIVersion Version

	// TODO: git commit / os / genesis cid?

	// Seconds
	BlockDelay uint64
}

func (v APIVersion) String() string {
	return fmt.Sprintf("%s+api%s", v.Version, v.APIVersion.String())
}

type NatInfo struct {
	Reachability network.Reachability
	PublicAddr   string
}
