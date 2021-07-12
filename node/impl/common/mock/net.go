package mock

import (
	"context"
	"errors"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"go.uber.org/fx"
)

var (
	errNotImplemented = errors.New("not implemented")
)

type MockNetAPI struct {
	fx.In
}

func (a *MockNetAPI) NetAgentVersion(ctx context.Context, p peer.ID) (string, error) {
	return "", errNotImplemented
}

func (a *MockNetAPI) NetConnectedness(ctx context.Context, pid peer.ID) (conn network.Connectedness, err error) {
	err = errNotImplemented
	return
}

func (a *MockNetAPI) NetPubsubScores(context.Context) ([]api.PubsubScore, error) {
	return nil, errNotImplemented
}

func (a *MockNetAPI) NetPeers(context.Context) ([]peer.AddrInfo, error) {
	return nil, errNotImplemented
}

func (a *MockNetAPI) NetPeerInfo(_ context.Context, p peer.ID) (*api.ExtendedPeerInfo, error) {
	return nil, errNotImplemented
}

func (a *MockNetAPI) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return errNotImplemented
}

func (a *MockNetAPI) NetAddrsListen(context.Context) (ai peer.AddrInfo, err error) {
	err = errNotImplemented
	return
}

func (a *MockNetAPI) NetDisconnect(ctx context.Context, p peer.ID) error {
	return errNotImplemented
}

func (a *MockNetAPI) NetFindPeer(ctx context.Context, p peer.ID) (ai peer.AddrInfo, err error) {
	err = errNotImplemented
	return
}

func (a *MockNetAPI) NetAutoNatStatus(ctx context.Context) (i api.NatInfo, err error) {
	err = errNotImplemented
	return
}

func (a *MockNetAPI) NetBandwidthStats(ctx context.Context) (s metrics.Stats, err error) {
	err = errNotImplemented
	return
}

func (a *MockNetAPI) NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error) {
	return nil, errNotImplemented
}

func (a *MockNetAPI) NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error) {
	return nil, errNotImplemented
}

func (a *MockNetAPI) Discover(ctx context.Context) (apitypes.OpenRPCDocument, error) {
	return nil, errNotImplemented
}

func (a *MockNetAPI) ID(context.Context) (p peer.ID, err error) {
	err = errNotImplemented
	return
}

func (a *MockNetAPI) NetBlockAdd(ctx context.Context, acl api.NetBlockList) error {
	return errNotImplemented
}

func (a *MockNetAPI) NetBlockRemove(ctx context.Context, acl api.NetBlockList) error {
	return errNotImplemented
}

func (a *MockNetAPI) NetBlockList(ctx context.Context) (result api.NetBlockList, err error) {
	err = errNotImplemented
	return
}
