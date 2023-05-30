package peermgr

import (
	"context"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	net "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.opencensus.io/stats"
	"go.uber.org/fx"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
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

	expanding chan struct{}

	h   host.Host
	dht *dht.IpfsDHT

	notifee *net.NotifyBundle
	emitter event.Emitter

	done chan struct{}
}

type FilPeerEvt struct {
	Type FilPeerEvtType
	ID   peer.ID
}

type FilPeerEvtType int

const (
	AddFilPeerEvt FilPeerEvtType = iota
	RemoveFilPeerEvt
)

func NewPeerMgr(lc fx.Lifecycle, h host.Host, dht *dht.IpfsDHT, bootstrap dtypes.BootstrapPeers) (*PeerMgr, error) {
	pm := &PeerMgr{
		h:             h,
		dht:           dht,
		bootstrappers: bootstrap,

		peers:     make(map[peer.ID]time.Duration),
		expanding: make(chan struct{}, 1),

		maxFilPeers: MaxFilPeers,
		minFilPeers: MinFilPeers,

		done: make(chan struct{}),
	}
	emitter, err := h.EventBus().Emitter(new(FilPeerEvt))
	if err != nil {
		return nil, xerrors.Errorf("creating FilPeerEvt emitter: %w", err)
	}
	pm.emitter = emitter

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return multierr.Combine(
				pm.emitter.Close(),
				pm.Stop(ctx),
			)
		},
	})

	pm.notifee = &net.NotifyBundle{
		DisconnectedF: func(_ net.Network, c net.Conn) {
			pm.Disconnect(c.RemotePeer())
		},
	}

	h.Network().Notify(pm.notifee)

	return pm, nil
}

func (pmgr *PeerMgr) AddFilecoinPeer(p peer.ID) {
	_ = pmgr.emitter.Emit(FilPeerEvt{Type: AddFilPeerEvt, ID: p}) //nolint:errcheck
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
	disconnected := false

	if pmgr.h.Network().Connectedness(p) == net.NotConnected {
		pmgr.peersLk.Lock()
		_, disconnected = pmgr.peers[p]
		if disconnected {
			delete(pmgr.peers, p)
		}
		pmgr.peersLk.Unlock()
	}

	if disconnected {
		_ = pmgr.emitter.Emit(FilPeerEvt{Type: RemoveFilPeerEvt, ID: p}) //nolint:errcheck
	}
}

func (pmgr *PeerMgr) Stop(ctx context.Context) error {
	log.Warn("closing peermgr done")
	close(pmgr.done)
	return nil
}

func (pmgr *PeerMgr) Run(ctx context.Context) {
	tick := build.Clock.Ticker(time.Second * 5)
	for {
		select {
		case <-tick.C:
			pcount := pmgr.getPeerCount()
			if pcount < pmgr.minFilPeers {
				pmgr.expandPeers()
			} else if pcount > pmgr.maxFilPeers {
				log.Debugf("peer count about threshold: %d > %d", pcount, pmgr.maxFilPeers)
			}
			stats.Record(ctx, metrics.PeerCount.M(int64(pmgr.getPeerCount())))
		case <-pmgr.done:
			log.Warn("exiting peermgr run")
			return
		}
	}
}

func (pmgr *PeerMgr) getPeerCount() int {
	pmgr.peersLk.Lock()
	defer pmgr.peersLk.Unlock()
	return len(pmgr.peers)
}

func (pmgr *PeerMgr) expandPeers() {
	select {
	case pmgr.expanding <- struct{}{}:
	default:
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.TODO(), time.Second*30)
		defer cancel()

		pmgr.doExpand(ctx)

		<-pmgr.expanding
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
		wg := sync.WaitGroup{}
		for _, bsp := range pmgr.bootstrappers {
			wg.Add(1)
			go func(bsp peer.AddrInfo) {
				defer wg.Done()
				if err := pmgr.h.Connect(ctx, bsp); err != nil {
					log.Warnf("failed to connect to bootstrap peer: %s", err)
				}
			}(bsp)
		}
		wg.Wait()
		return
	}

	// if we already have some peers and need more, the dht is really good at connecting to most peers. Use that for now until something better comes along.
	if err := pmgr.dht.Bootstrap(ctx); err != nil {
		log.Warnf("dht bootstrapping failed: %s", err)
	}
}
