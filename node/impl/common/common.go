package common

import (
	"context"
	"sort"
	"strings"

	logging "github.com/ipfs/go-log/v2"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	swarm "github.com/libp2p/go-libp2p-swarm"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
)

type CommonAPI struct {
	fx.In

	APISecret    *dtypes.APIAlg
	Host         host.Host
	Router       lp2p.BaseIpfsRouting
	Sk           *dtypes.ScoreKeeper
	ShutdownChan dtypes.ShutdownChan
}

type jwtPayload struct {
	Allow []auth.Permission
}

func (a *CommonAPI) AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) {
	var payload jwtPayload
	if _, err := jwt.Verify([]byte(token), (*jwt.HMACSHA)(a.APISecret), &payload); err != nil {
		return nil, xerrors.Errorf("JWT Verification failed: %w", err)
	}

	return payload.Allow, nil
}

func (a *CommonAPI) AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error) {
	p := jwtPayload{
		Allow: perms, // TODO: consider checking validity
	}

	return jwt.Sign(&p, (*jwt.HMACSHA)(a.APISecret))
}

func (a *CommonAPI) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	return a.Host.Network().Connectedness(pid), nil
}
func (a *CommonAPI) NetPubsubScores(context.Context) ([]api.PubsubScore, error) {
	scores := a.Sk.Get()
	out := make([]api.PubsubScore, len(scores))
	i := 0
	for k, v := range scores {
		out[i] = api.PubsubScore{ID: k, Score: v}
		i++
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(string(out[i].ID), string(out[j].ID)) > 0
	})

	return out, nil
}

func (a *CommonAPI) NetPeers(context.Context) ([]peer.AddrInfo, error) {
	conns := a.Host.Network().Conns()
	out := make([]peer.AddrInfo, len(conns))

	for i, conn := range conns {
		out[i] = peer.AddrInfo{
			ID: conn.RemotePeer(),
			Addrs: []ma.Multiaddr{
				conn.RemoteMultiaddr(),
			},
		}
	}

	return out, nil
}

func (a *CommonAPI) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	if swrm, ok := a.Host.Network().(*swarm.Swarm); ok {
		swrm.Backoff().Clear(p.ID)
	}

	return a.Host.Connect(ctx, p)
}

func (a *CommonAPI) NetAddrsListen(context.Context) (peer.AddrInfo, error) {
	return peer.AddrInfo{
		ID:    a.Host.ID(),
		Addrs: a.Host.Addrs(),
	}, nil
}

func (a *CommonAPI) NetDisconnect(ctx context.Context, p peer.ID) error {
	return a.Host.Network().ClosePeer(p)
}

func (a *CommonAPI) NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	return a.Router.FindPeer(ctx, p)
}

func (a *CommonAPI) ID(context.Context) (peer.ID, error) {
	return a.Host.ID(), nil
}

func (a *CommonAPI) Version(context.Context) (api.Version, error) {
	return api.Version{
		Version:    build.UserVersion(),
		APIVersion: build.APIVersion,

		BlockDelay: build.BlockDelaySecs,
	}, nil
}

func (a *CommonAPI) LogList(context.Context) ([]string, error) {
	return logging.GetSubsystems(), nil
}

func (a *CommonAPI) LogSetLevel(ctx context.Context, subsystem, level string) error {
	return logging.SetLogLevel(subsystem, level)
}

func (a *CommonAPI) Shutdown(ctx context.Context) error {
	a.ShutdownChan <- struct{}{}
	return nil
}

func (a *CommonAPI) Closing(ctx context.Context) (<-chan struct{}, error) {
	return make(chan struct{}), nil // relies on jsonrpc closing
}

var _ api.Common = &CommonAPI{}
