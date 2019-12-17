package peers

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/chain/stmgr"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/prometheus/common/log"
	"go.opencensus.io/trace"
)

// PeerTagger uses information from the chain to tag peer connections to
// prevent them from being closed by the connection manager
type PeerTagger struct {
	h  host.Host
	st *stmgr.StateManager

	closing chan struct{}
}

func NewPeerTagger(h host.Host, st *stmgr.StateManager) *PeerTagger {
	log.Error("NEW PEER TAGGER")
	return &PeerTagger{
		h:       h,
		st:      st,
		closing: make(chan struct{}),
	}
}

func (pt *PeerTagger) Run() {
	go pt.loop()
}

func (pt *PeerTagger) loop() {
	tick := time.NewTimer(time.Second)
	for {
		select {
		case <-tick.C:
			if err := pt.tagMiners(context.TODO()); err != nil {
				log.Warn("failed to run tag miners: ", err)
			}

			tick.Reset(time.Minute * 2)

		case <-pt.closing:
			log.Warn("peer tagger shutting down")
			return
		}
	}
}

func (pt *PeerTagger) tagMiners(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "tagMiners")
	defer span.End()

	ts := pt.st.ChainStore().GetHeaviestTipSet()
	mactors, err := stmgr.ListMinerActors(context.TODO(), pt.st, ts)
	if err != nil {
		return err
	}

	for _, m := range mactors {
		mpow, _, err := stmgr.GetPower(context.TODO(), pt.st, ts, m)
		if err != nil {
			log.Warn("failed to get miners power: ", err)
			continue
		}
		if !mpow.IsZero() {
			pid, err := stmgr.GetMinerPeerID(context.TODO(), pt.st, ts, m)
			if err != nil {
				log.Warn("failed to get peer ID for miner: ", err)
				continue
			}
			pt.h.ConnManager().TagPeer(pid, "miner", 10)
		}
	}
	return nil
}

func (pt *PeerTagger) Close() error {
	close(pt.closing)
	return nil
}
