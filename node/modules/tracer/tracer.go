package tracer

import (
	peer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

func newLotusTracer(et pubsub.EventTracer, tt TracerTransport) LotusTracer {
	return &lotusTracer{
		et: et,
		tt: tt,
	}
}

type lotusTracer struct {
	et pubsub.EventTracer
	tt TracerTransport
}

type LotusTracer interface {
	TracePeerScore(scores map[peer.ID]*pubsub.PeerScoreSnapshot)
	Trace(evt *pubsub_pb.TraceEvent)
}

func (lt *lotusTracer) TracePeerScore(scores map[peer.ID]*pubsub.PeerScoreSnapshot) {}

func (lt *lotusTracer) Trace(evt *pubsub_pb.TraceEvent) {}
