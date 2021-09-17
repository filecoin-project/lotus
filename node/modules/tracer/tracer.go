package tracer

import (
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

var log = logging.Logger("lotus-tracer")

func NewLotusTracer(tt []TracerTransport, pid peer.ID) LotusTracer {
	return &lotusTracer{
		tt:  tt,
		pid: pid,
	}
}

type lotusTracer struct {
	tt  []TracerTransport
	pid peer.ID
}

const (
	TraceEvent_PEER_SCORES pubsub_pb.TraceEvent_Type = 100
)

type LotusTraceEvent struct {
	Type       pubsub_pb.TraceEvent_Type `json:"type,omitempty"`
	PeerID     string                    `json:"peerID,omitempty"`
	Timestamp  *int64                    `json:"timestamp,omitempty"`
	PeerScores []TraceEvent_PeerScore    `json:"peerScores,omitempty"`
}

type TraceEvent_PeerScore struct {
	PeerID string  `json:"peerID"`
	Score  float32 `json:"score"`
}

type LotusTracer interface {
	Trace(evt *pubsub_pb.TraceEvent)
	TraceLotusEvent(evt *LotusTraceEvent)

	PeerScores(scores map[peer.ID]*pubsub.PeerScoreSnapshot)
}

func (lt *lotusTracer) PeerScores(scores map[peer.ID]*pubsub.PeerScoreSnapshot) {
	var peerScores []TraceEvent_PeerScore
	for pid, score := range scores {
		peerScores = append(peerScores, TraceEvent_PeerScore{PeerID: pid.Pretty(), Score: float32(score.Score)})
	}

	now := time.Now().UnixNano()
	evt := &LotusTraceEvent{
		Type:       *TraceEvent_PEER_SCORES.Enum(),
		PeerID:     string(lt.pid),
		Timestamp:  &now,
		PeerScores: peerScores,
	}

	lt.TraceLotusEvent(evt)
}

func (lt *lotusTracer) TraceLotusEvent(evt *LotusTraceEvent) {
	for _, t := range lt.tt {
		err := t.Transport(TracerTransportEvent{
			lotusTraceEvent:  evt,
			pubsubTraceEvent: nil,
		})
		if err != nil {
			log.Errorf("error while transporting peer scores: %s", err)
		}
	}

}

func (lt *lotusTracer) Trace(evt *pubsub_pb.TraceEvent) {
	for _, t := range lt.tt {
		err := t.Transport(TracerTransportEvent{
			lotusTraceEvent:  nil,
			pubsubTraceEvent: evt,
		})
		if err != nil {
			log.Errorf("error while transporting trace event: %s", err)
		}
	}
}
