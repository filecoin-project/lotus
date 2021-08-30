package lp2p

import (
	peer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func newLotusTracer(tr pubsub.EventTracer) LotusTracer {
	return &lotusTracer{
		tr: tr,
	}
}

type lotusTracer struct {
	tr pubsub.EventTracer
}

type LotusTracer interface {
	TracePeerScore(scores map[peer.ID]*pubsub.PeerScoreSnapshot)
	Trace(evt *pubsub_pb.TraceEvent)
}

func (lt *lotusTracer) TracePeerScore(scores map[peer.ID]*pubsub.PeerScoreSnapshot) {}

func (lt *lotusTracer) Trace(evt *pubsub_pb.TraceEvent) {}
