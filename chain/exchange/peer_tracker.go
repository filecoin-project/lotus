package exchange

// FIXME: This needs to be reviewed.

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/peermgr"
)

type peerStats struct {
	successes   int
	failures    int
	firstSeen   time.Time
	averageTime time.Duration
}

type bsPeerTracker struct {
	lk sync.Mutex

	peers         map[peer.ID]*peerStats
	avgGlobalTime time.Duration

	pmgr *peermgr.PeerMgr
}

func newPeerTracker(lc fx.Lifecycle, h host.Host, pmgr *peermgr.PeerMgr) *bsPeerTracker {
	bsPt := &bsPeerTracker{
		peers: make(map[peer.ID]*peerStats),
		pmgr:  pmgr,
	}

	evtSub, err := h.EventBus().Subscribe(new(peermgr.FilPeerEvt))
	if err != nil {
		panic(err)
	}

	go func() {
		for evt := range evtSub.Out() {
			pEvt := evt.(peermgr.FilPeerEvt)
			switch pEvt.Type {
			case peermgr.AddFilPeerEvt:
				bsPt.addPeer(pEvt.ID)
			case peermgr.RemoveFilPeerEvt:
				bsPt.removePeer(pEvt.ID)
			}
		}
	}()

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return evtSub.Close()
		},
	})

	return bsPt
}

func (bpt *bsPeerTracker) addPeer(p peer.ID) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	if _, ok := bpt.peers[p]; ok {
		return
	}
	bpt.peers[p] = &peerStats{
		firstSeen: build.Clock.Now(),
	}

}

const (
	// newPeerMul is how much better than average is the new peer assumed to be
	// less than one to encourouge trying new peers
	newPeerMul = 0.9
)

func (bpt *bsPeerTracker) prefSortedPeers() []peer.ID {
	// TODO: this could probably be cached, but as long as its not too many peers, fine for now
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	out := make([]peer.ID, 0, len(bpt.peers))
	for p := range bpt.peers {
		out = append(out, p)
	}

	// sort by 'expected cost' of requesting data from that peer
	// additionally handle edge cases where not enough data is available
	sort.Slice(out, func(i, j int) bool {
		pi := bpt.peers[out[i]]
		pj := bpt.peers[out[j]]

		var costI, costJ float64

		getPeerInitLat := func(p peer.ID) float64 {
			return float64(bpt.avgGlobalTime) * newPeerMul
		}

		if pi.successes+pi.failures > 0 {
			failRateI := float64(pi.failures) / float64(pi.failures+pi.successes)
			costI = float64(pi.averageTime) + failRateI*float64(bpt.avgGlobalTime)
		} else {
			costI = getPeerInitLat(out[i])
		}

		if pj.successes+pj.failures > 0 {
			failRateJ := float64(pj.failures) / float64(pj.failures+pj.successes)
			costJ = float64(pj.averageTime) + failRateJ*float64(bpt.avgGlobalTime)
		} else {
			costJ = getPeerInitLat(out[j])
		}

		return costI < costJ
	})

	return out
}

const (
	// xInvAlpha = (N+1)/2

	localInvAlpha  = 10 // 86% of the value is the last 19
	globalInvAlpha = 25 // 86% of the value is the last 49
)

func (bpt *bsPeerTracker) logGlobalSuccess(dur time.Duration) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()

	if bpt.avgGlobalTime == 0 {
		bpt.avgGlobalTime = dur
		return
	}
	delta := (dur - bpt.avgGlobalTime) / globalInvAlpha
	bpt.avgGlobalTime += delta
}

func logTime(pi *peerStats, dur time.Duration) {
	if pi.averageTime == 0 {
		pi.averageTime = dur
		return
	}
	delta := (dur - pi.averageTime) / localInvAlpha
	pi.averageTime += delta

}

func (bpt *bsPeerTracker) logSuccess(p peer.ID, dur time.Duration, reqSize uint64) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()

	var pi *peerStats
	var ok bool
	if pi, ok = bpt.peers[p]; !ok {
		log.Warnw("log success called on peer not in tracker", "peerid", p.String())
		return
	}

	pi.successes++
	if reqSize == 0 {
		reqSize = 1
	}
	logTime(pi, dur/time.Duration(reqSize))
}

func (bpt *bsPeerTracker) logFailure(p peer.ID, dur time.Duration, reqSize uint64) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()

	var pi *peerStats
	var ok bool
	if pi, ok = bpt.peers[p]; !ok {
		log.Warn("log failure called on peer not in tracker", "peerid", p.String())
		return
	}

	pi.failures++
	if reqSize == 0 {
		reqSize = 1
	}
	logTime(pi, dur/time.Duration(reqSize))
}

func (bpt *bsPeerTracker) removePeer(p peer.ID) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	delete(bpt.peers, p)
}
