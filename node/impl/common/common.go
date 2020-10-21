package common

import (
	"context"
	"sort"
	"strings"

	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/tag"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/libp2p/go-libp2p-core/host"
	lpmetrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
)

type CommonAPI struct {
	fx.In

	APISecret    *dtypes.APIAlg
	RawHost      lp2p.RawHost
	Host         host.Host
	Router       lp2p.BaseIpfsRouting
	Reporter     lpmetrics.Reporter
	Sk           *dtypes.ScoreKeeper
	ShutdownChan dtypes.ShutdownChan
}

type jwtPayload struct {
	Allow []auth.Permission
}

func (a *CommonAPI) AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "AuthVerify"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	var payload jwtPayload
	if _, err := jwt.Verify([]byte(token), (*jwt.HMACSHA)(a.APISecret), &payload); err != nil {
		return nil, xerrors.Errorf("JWT Verification failed: %w", err)
	}

	return payload.Allow, nil
}

func (a *CommonAPI) AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "AuthNew"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	p := jwtPayload{
		Allow: perms, // TODO: consider checking validity
	}

	return jwt.Sign(&p, (*jwt.HMACSHA)(a.APISecret))
}

func (a *CommonAPI) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetConnectedness"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Host.Network().Connectedness(pid), nil
}
func (a *CommonAPI) NetPubsubScores(ctx context.Context) ([]api.PubsubScore, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetPubsubScores"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

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

func (a *CommonAPI) NetPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetPeers"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

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
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetConnect"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	if swrm, ok := a.Host.Network().(*swarm.Swarm); ok {
		swrm.Backoff().Clear(p.ID)
	}

	return a.Host.Connect(ctx, p)
}

func (a *CommonAPI) NetAddrsListen(ctx context.Context) (peer.AddrInfo, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetAddrsListen"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return peer.AddrInfo{
		ID:    a.Host.ID(),
		Addrs: a.Host.Addrs(),
	}, nil
}

func (a *CommonAPI) NetDisconnect(ctx context.Context, p peer.ID) error {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetDisconnect"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Host.Network().ClosePeer(p)
}

func (a *CommonAPI) NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetFindPeer"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Router.FindPeer(ctx, p)
}

func (a *CommonAPI) NetAutoNatStatus(ctx context.Context) (i api.NatInfo, err error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetAutoNatStatus"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	autonat := a.RawHost.(*basichost.BasicHost).AutoNat

	if autonat == nil {
		return api.NatInfo{
			Reachability: network.ReachabilityUnknown,
		}, nil
	}

	var maddr string
	if autonat.Status() == network.ReachabilityPublic {
		pa, err := autonat.PublicAddr()
		if err != nil {
			return api.NatInfo{}, err
		}
		maddr = pa.String()
	}

	return api.NatInfo{
		Reachability: autonat.Status(),
		PublicAddr:   maddr,
	}, nil
}

func (a *CommonAPI) NetAgentVersion(ctx context.Context, p peer.ID) (string, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetAgentVersion"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	ag, err := a.Host.Peerstore().Get(p, "AgentVersion")
	if err != nil {
		return "", err
	}

	if ag == nil {
		return "unknown", nil
	}

	return ag.(string), nil
}

func (a *CommonAPI) NetBandwidthStats(ctx context.Context) (lpmetrics.Stats, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetBandwidthStats"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Reporter.GetBandwidthTotals(), nil
}

func (a *CommonAPI) NetBandwidthStatsByPeer(ctx context.Context) (map[string]lpmetrics.Stats, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetBandwidthStatsByPeer"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	out := make(map[string]lpmetrics.Stats)
	for p, s := range a.Reporter.GetBandwidthByPeer() {
		out[p.String()] = s
	}
	return out, nil
}

func (a *CommonAPI) NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]lpmetrics.Stats, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "NetBandwidthStatsByProtocol"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Reporter.GetBandwidthByProtocol(), nil
}

func (a *CommonAPI) ID(ctx context.Context) (peer.ID, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "ID"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.Host.ID(), nil
}

func (a *CommonAPI) Version(ctx context.Context) (api.Version, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "Version"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	v, err := build.VersionForType(build.RunningNodeType)
	if err != nil {
		return api.Version{}, err
	}

	return api.Version{
		Version:    build.UserVersion(),
		APIVersion: v,

		BlockDelay: build.BlockDelaySecs,
	}, nil
}

func (a *CommonAPI) LogList(ctx context.Context) ([]string, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "LogList"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return logging.GetSubsystems(), nil
}

func (a *CommonAPI) LogSetLevel(ctx context.Context, subsystem, level string) error {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "LogSetLevel"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return logging.SetLogLevel(subsystem, level)
}

func (a *CommonAPI) Shutdown(ctx context.Context) error {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "Shutdown"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	a.ShutdownChan <- struct{}{}
	return nil
}

func (a *CommonAPI) Closing(ctx context.Context) (<-chan struct{}, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "Closing"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return make(chan struct{}), nil // relies on jsonrpc closing
}

var _ api.Common = &CommonAPI{}
