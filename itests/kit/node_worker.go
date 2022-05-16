package kit

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/multiformats/go-multiaddr"
)

// TestWorker represents a worker enrolled in an Ensemble.
type TestWorker struct {
	api.Worker

	t *testing.T

	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node
	ListenAddr multiaddr.Multiaddr

	Stop func(context.Context) error

	FetchHandler   http.HandlerFunc
	MinerNode      *TestMiner
	RemoteListener net.Listener

	options nodeOpts
}
