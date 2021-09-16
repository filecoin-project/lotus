package tracer

import (
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

var log = logging.Logger("lotus-tracer")

func NewLotusTracer(tt []TracerTransport, pid peer.ID, sourceAuth string) LotusTracer {
	return &lotusTracer{
		tt:  tt,
		pid: pid,
		sa:  sourceAuth,
	}
}

type lotusTracer struct {
	tt  []TracerTransport
	pid peer.ID
	sa  string
}

const (
	TraceEvent_PEER_SCORES pubsub_pb.TraceEvent_Type = 100
)

type LotusTraceEvent struct {
	Type       pubsub_pb.TraceEvent_Type `json:"type,omitempty"`
	PeerID     []byte                    `json:"peerID,omitempty"`
	Timestamp  *int64                    `json:"timestamp,omitempty"`
	PeerScores *TraceEvent_PeerScores    `json:"peerScores,omitempty"`
	SourceAuth string                    `json:"sourceAuth,omitempty"`
}

type TraceEvent_PeerScores struct {
	Scores map[peer.ID]*pubsub.PeerScoreSnapshot `json:"scores,omitempty"`
}

type LotusTracer interface {
	Trace(evt *pubsub_pb.TraceEvent)
	TraceLotusEvent(evt *LotusTraceEvent)

	PeerScores(scores map[peer.ID]*pubsub.PeerScoreSnapshot)
}

func (lt *lotusTracer) PeerScores(scores map[peer.ID]*pubsub.PeerScoreSnapshot) {
	now := time.Now().UnixNano()
	evt := &LotusTraceEvent{
		Type:       *TraceEvent_PEER_SCORES.Enum(),
		PeerID:     []byte(lt.pid),
		Timestamp:  &now,
		SourceAuth: lt.sa,
		PeerScores: &TraceEvent_PeerScores{
			Scores: scores,
		},
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
