package peermgr

import (
	"context"
	"sync"
	"time"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"go.uber.org/fx"

	host "github.com/libp2p/go-libp2p-core/host"
	net "github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"

	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("peermgr")

const (
	MaxFilPeers = 32
	MinFilPeers = 12
)

type MaybePeerMgr struct {
	fx.In

	Mgr *PeerMgr `optional:"true"`
}

type PeerMgr struct {
	bootstrappers []peer.AddrInfo

	// peerLeads is a set of peers we hear about through the network
	// and who may be good peers to connect to for expanding our peer set
	//peerLeads map[peer.ID]time.Time // TODO: unused

	peersLk sync.Mutex
	peers   map[peer.ID]time.Duration

	maxFilPeers int
	minFilPeers int

	expanding bool

	h   host.Host
	dht *dht.IpfsDHT

	notifee *net.NotifyBundle
}

func NewPeerMgr(h host.Host, dht *dht.IpfsDHT, bootstrap dtypes.BootstrapPeers) *PeerMgr {
	pm := &PeerMgr{
		h:             h,
		dht:           dht,
		bootstrappers: bootstrap,

		peers: make(map[peer.ID]time.Duration),

		maxFilPeers: MaxFilPeers,
		minFilPeers: MinFilPeers,
	}

	pm.notifee = &net.NotifyBundle{
		DisconnectedF: func(_ net.Network, c net.Conn) {
			pm.Disconnect(c.RemotePeer())
		},
	}

	h.Network().Notify(pm.notifee)

	return pm
}

func (pmgr *PeerMgr) AddFilecoinPeer(p peer.ID) {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	pmgr.peers[p] = time.Duration(0)
}

func (pmgr *PeerMgr) GetPeerLatency(p peer.ID) (time.Duration, bool) {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	dur, ok := pmgr.peers[p]
	return dur, ok
}

func (pmgr *PeerMgr) SetPeerLatency(p peer.ID, latency time.Duration) {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	if _, ok := pmgr.peers[p]; ok {
		pmgr.peers[p] = latency
	}

}

func (pmgr *PeerMgr) Disconnect(p peer.ID) {
	if pmgr.h.Network().Connectedness(p) == net.NotConnected {
		pmgr.peersLk.Lock()
		defer pmgr.peersLk.Unlock()
		delete(pmgr.peers, p)
	}
}

func (pmgr *PeerMgr) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-tick.C:
			pcount := pmgr.getPeerCount()
			if pcount < pmgr.minFilPeers {
				pmgr.expandPeers()
			} else if pcount > pmgr.maxFilPeers {
				log.Debug("peer count about threshold: %d > %d", pcount, pmgr.maxFilPeers)
			}
		}
	}
}

func (pmgr *PeerMgr) getPeerCount() int {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	return len(pmgr.peers)
}

func (pmgr *PeerMgr) expandPeers() {
	if pmgr.expanding {
		return
	}
	pmgr.expanding = true
	go func() {
		defer func() {
			pmgr.expanding = false
		}()
		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
		defer cancel()

		pmgr.doExpand(ctx)
	}()
}

func (pmgr *PeerMgr) doExpand(ctx context.Context) {
	pcount := pmgr.getPeerCount()
	if pcount == 0 {
		if len(pmgr.bootstrappers) == 0 {
			log.Warn("no peers connected, and no bootstrappers configured")
			return
		}

		log.Info("connecting to bootstrap peers")
		for _, bsp := range pmgr.bootstrappers {
			if err := pmgr.h.Connect(ctx, bsp); err != nil {
				log.Warnf("failed to connect to bootstrap peer: %s", err)
			}
		}
		return
	}

	// if we already have some peers and need more, the dht is really good at connecting to most peers. Use that for now until something better comes along.
	if err := pmgr.dht.Bootstrap(ctx); err != nil {
		log.Warnf("dht bootstrapping failed: %s", err)
	}
}
