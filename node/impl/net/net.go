package net

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/lp2p"
)

type NetAPI struct {
	fx.In

	RawHost         lp2p.RawHost
	Host            host.Host
	Router          lp2p.BaseIpfsRouting
	ConnGater       *conngater.BasicConnectionGater
	ResourceManager network.ResourceManager
	Reporter        metrics.Reporter
	Sk              *dtypes.ScoreKeeper
}

func (a *NetAPI) ID(context.Context) (peer.ID, error) {
	return a.Host.ID(), nil
}

func (a *NetAPI) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	return a.Host.Network().Connectedness(pid), nil
}

func (a *NetAPI) NetPubsubScores(context.Context) ([]api.PubsubScore, error) {
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

func (a *NetAPI) NetPeers(context.Context) ([]peer.AddrInfo, error) {
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

func (a *NetAPI) NetPeerInfo(_ context.Context, p peer.ID) (*api.ExtendedPeerInfo, error) {
	info := &api.ExtendedPeerInfo{ID: p}

	agent, err := a.Host.Peerstore().Get(p, "AgentVersion")
	if err == nil {
		info.Agent = agent.(string)
	}

	for _, a := range a.Host.Peerstore().Addrs(p) {
		info.Addrs = append(info.Addrs, a.String())
	}
	sort.Strings(info.Addrs)

	protocols, err := a.Host.Peerstore().GetProtocols(p)
	if err == nil {
		protocolStrings := make([]string, 0, len(protocols))
		for _, protocol := range protocols {
			protocolStrings = append(protocolStrings, string(protocol))
		}
		sort.Strings(protocolStrings)
		info.Protocols = protocolStrings
	}

	if cm := a.Host.ConnManager().GetTagInfo(p); cm != nil {
		info.ConnMgrMeta = &api.ConnMgrInfo{
			FirstSeen: cm.FirstSeen,
			Value:     cm.Value,
			Tags:      cm.Tags,
			Conns:     cm.Conns,
		}
	}

	return info, nil
}

func (a *NetAPI) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	if swrm, ok := a.Host.Network().(*swarm.Swarm); ok {
		swrm.Backoff().Clear(p.ID)
	}

	return a.Host.Connect(ctx, p)
}

func (a *NetAPI) NetAddrsListen(context.Context) (peer.AddrInfo, error) {
	return peer.AddrInfo{
		ID:    a.Host.ID(),
		Addrs: a.Host.Addrs(),
	}, nil
}

func (a *NetAPI) NetDisconnect(ctx context.Context, p peer.ID) error {
	return a.Host.Network().ClosePeer(p)
}

func (a *NetAPI) NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	return a.Router.FindPeer(ctx, p)
}

type autoNatGetter interface {
	GetAutoNat() autonat.AutoNAT
}

func (a *NetAPI) NetAutoNatStatus(context.Context) (i api.NatInfo, err error) {
	autonat := a.RawHost.(autoNatGetter).GetAutoNat()

	if autonat == nil {
		return api.NatInfo{
			Reachability: network.ReachabilityUnknown,
		}, nil
	}

	var addrs []string
	if autonat.Status() == network.ReachabilityPublic {
		for _, addr := range a.Host.Addrs() {
			if manet.IsPublicAddr(addr) {
				addrs = append(addrs, addr.String())
			}
		}
	}

	return api.NatInfo{
		Reachability: autonat.Status(),
		PublicAddrs:  addrs,
	}, nil
}

func (a *NetAPI) NetAgentVersion(ctx context.Context, p peer.ID) (string, error) {
	ag, err := a.Host.Peerstore().Get(p, "AgentVersion")
	if err != nil {
		return "", err
	}

	if ag == nil {
		return "unknown", nil
	}

	return ag.(string), nil
}

func (a *NetAPI) NetBandwidthStats(ctx context.Context) (metrics.Stats, error) {
	return a.Reporter.GetBandwidthTotals(), nil
}

func (a *NetAPI) NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error) {
	out := make(map[string]metrics.Stats)
	for p, s := range a.Reporter.GetBandwidthByPeer() {
		out[p.String()] = s
	}
	return out, nil
}

func (a *NetAPI) NetPing(ctx context.Context, p peer.ID) (time.Duration, error) {
	result, ok := <-ping.Ping(ctx, a.Host, p)
	if !ok {
		return 0, xerrors.Errorf("didn't get ping result: %w", ctx.Err())
	}
	return result.RTT, result.Error
}

func (a *NetAPI) NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error) {
	return a.Reporter.GetBandwidthByProtocol(), nil
}

var _ api.Net = &NetAPI{}
