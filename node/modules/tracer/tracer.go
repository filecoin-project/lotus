package tracer

import (
	"time"

	logging "github.com/ipfs/go-log/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
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
	TraceEventPeerScores pubsub_pb.TraceEvent_Type = 100
)

type LotusTraceEvent struct {
	Type       pubsub_pb.TraceEvent_Type `json:"type,omitempty"`
	PeerID     []byte                    `json:"peerID,omitempty"`
	Timestamp  *int64                    `json:"timestamp,omitempty"`
	PeerScore  TraceEventPeerScore       `json:"peerScore,omitempty"`
	SourceAuth string                    `json:"sourceAuth,omitempty"`
}

type TopicScore struct {
	Topic                    string        `json:"topic"`
	TimeInMesh               time.Duration `json:"timeInMesh"`
	FirstMessageDeliveries   float64       `json:"firstMessageDeliveries"`
	MeshMessageDeliveries    float64       `json:"meshMessageDeliveries"`
	InvalidMessageDeliveries float64       `json:"invalidMessageDeliveries"`
}

type TraceEventPeerScore struct {
	PeerID             []byte       `json:"peerID"`
	Score              float64      `json:"score"`
	AppSpecificScore   float64      `json:"appSpecificScore"`
	IPColocationFactor float64      `json:"ipColocationFactor"`
	BehaviourPenalty   float64      `json:"behaviourPenalty"`
	Topics             []TopicScore `json:"topics"`
}

type LotusTracer interface {
	Trace(evt *pubsub_pb.TraceEvent)
	TraceLotusEvent(evt *LotusTraceEvent)

	PeerScores(scores map[peer.ID]*pubsub.PeerScoreSnapshot)
}

func (lt *lotusTracer) PeerScores(scores map[peer.ID]*pubsub.PeerScoreSnapshot) {
	now := time.Now().UnixNano()
	for pid, score := range scores {
		var topics []TopicScore
		for topic, snapshot := range score.Topics {
			topics = append(topics, TopicScore{
				Topic:                    topic,
				TimeInMesh:               snapshot.TimeInMesh,
				FirstMessageDeliveries:   snapshot.FirstMessageDeliveries,
				MeshMessageDeliveries:    snapshot.MeshMessageDeliveries,
				InvalidMessageDeliveries: snapshot.InvalidMessageDeliveries,
			})
		}

		evt := &LotusTraceEvent{
			Type:       *TraceEventPeerScores.Enum(),
			Timestamp:  &now,
			PeerID:     []byte(lt.pid),
			SourceAuth: lt.sa,
			PeerScore: TraceEventPeerScore{
				PeerID:             []byte(pid),
				Score:              score.Score,
				AppSpecificScore:   score.AppSpecificScore,
				IPColocationFactor: score.IPColocationFactor,
				BehaviourPenalty:   score.BehaviourPenalty,
				Topics:             topics,
			},
		}

		lt.TraceLotusEvent(evt)
	}
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
