package consensus

import (
	"context"

	consensus "github.com/libp2p/go-libp2p-consensus"
	rpc "github.com/libp2p/go-libp2p-gorpc"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Component represents a piece of ipfscluster. Cluster components
// usually run their own goroutines (a http server for example). They
// communicate with the main Cluster component and other components
// (both local and remote), using an instance of rpc.Client.
type Component interface {
	SetClient(*rpc.Client)
	Shutdown(context.Context) error
}

type ConsensusAPI interface {
	Component
	// Returns a channel to signal that the consensus layer is ready
	// allowing the main component to wait for it during start.
	Ready(context.Context) <-chan struct{}

	AddPeer(context.Context, peer.ID) error
	RmPeer(context.Context, peer.ID) error
	State(context.Context) (consensus.State, error)
	// Provide a node which is responsible to perform
	// specific tasks which must only run in 1 cluster peer.
	Leader(context.Context) (peer.ID, error)
	// Only returns when the consensus state has all log
	// updates applied to it.
	WaitForSync(context.Context) error
	// Clean removes all consensus data.
	Clean(context.Context) error
	// Peers returns the peerset participating in the Consensus.
	Peers(context.Context) ([]peer.ID, error)
	// IsTrustedPeer returns true if the given peer is "trusted".
	// This will grant access to more rpc endpoints and a
	// non-trusted one. This should be fast as it will be
	// called repeatedly for every remote RPC request.
	IsTrustedPeer(context.Context, peer.ID) bool
	// Trust marks a peer as "trusted".
	Trust(context.Context, peer.ID) error
	// Distrust removes a peer from the "trusted" set.
	Distrust(context.Context, peer.ID) error
	// Returns true if current node is the cluster leader
	IsLeader(ctx context.Context) bool
}
