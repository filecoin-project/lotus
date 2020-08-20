package api

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/build"
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

	// MethodGroup: Common

	// ID returns peerID of libp2p node backing this API
	ID(context.Context) (peer.ID, error)

	// Version provides information about API provider
	Version(context.Context) (Version, error)

	LogList(context.Context) ([]string, error)
	LogSetLevel(context.Context, string, string) error

	// trigger graceful shutdown
	Shutdown(context.Context) error

	Closing(context.Context) (<-chan struct{}, error)
}

// Version provides various build-time information
type Version struct {
	Version string

	// APIVersion is a binary encoded semver version of the remote implementing
	// this api
	//
	// See APIVersion in build/version.go
	APIVersion build.Version

	// TODO: git commit / os / genesis cid?

	// Seconds
	BlockDelay uint64
}

func (v Version) String() string {
	return fmt.Sprintf("%s+api%s", v.Version, v.APIVersion.String())
}

type NatInfo struct {
	Reachability network.Reachability
	PublicAddr   string
}
