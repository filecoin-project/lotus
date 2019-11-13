package api

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/filecoin-project/lotus/build"
)

type Common interface {
	// Auth
	AuthVerify(ctx context.Context, token string) ([]Permission, error)
	AuthNew(ctx context.Context, perms []Permission) ([]byte, error)

	// network

	NetConnectedness(context.Context, peer.ID) (network.Connectedness, error)
	NetPeers(context.Context) ([]peer.AddrInfo, error)
	NetConnect(context.Context, peer.AddrInfo) error
	NetAddrsListen(context.Context) (peer.AddrInfo, error)
	NetDisconnect(context.Context, peer.ID) error

	// ID returns peerID of libp2p node backing this API
	ID(context.Context) (peer.ID, error)

	// Version provides information about API provider
	Version(context.Context) (Version, error)
}

// Version provides various build-time information
type Version struct {
	Version string

	// APIVersion is a binary encoded semver version of the remote implementing
	// this api
	//
	// See APIVersion in build/version.go
	APIVersion uint32

	// TODO: git commit / os / genesis cid?

	// Seconds
	BlockDelay uint64
}

func (v Version) String() string {
	vM, vm, vp := build.VersionInts(v.APIVersion)
	return fmt.Sprintf("%s+api%d.%d.%d", v.Version, vM, vm, vp)
}

func VersionFullNode() *Version {
	return &Version{
		Version:    build.Version,
		APIVersion: build.APIVersion,

		BlockDelay: build.BlockDelay,
	}
}

func VersionStorageMiner() *Version {
	return &Version{
		Version:    build.Version,
		APIVersion: build.APIVersion,

		BlockDelay: build.BlockDelay,
	}
}

func AppVersion(t interface{}) ([]byte, error) {
	ver, ok := t.(*Version)
	if !ok {
		return nil, nil
	}
	return []byte(ver.String()), nil
}
