package tracer

import (
	"encoding/json"
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
	PeerID     []byte                    `json:"peerID,omitempty"`
	Timestamp  *int64                    `json:"timestamp,omitempty"`
	PeerScores *TraceEvent_PeerScores    `json:"peerScores,omitempty"`
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
		Type:      *TraceEvent_PEER_SCORES.Enum(),
		PeerID:    []byte(lt.pid),
		Timestamp: &now,
		PeerScores: &TraceEvent_PeerScores{
			Scores: scores,
		},
	}

	lt.TraceLotusEvent(evt)
}

func (lt *lotusTracer) TraceLotusEvent(evt *LotusTraceEvent) {
	jsonEvent, err := json.Marshal(evt)
	if err != nil {
		log.Errorf("error while marshaling peer score: %s", err)
		return
	}

	for _, t := range lt.tt {
		err = t.Transport(jsonEvent)
		if err != nil {
			log.Errorf("error while transporting peer scores: %s", err)
		}
	}

}

func (lt *lotusTracer) Trace(evt *pubsub_pb.TraceEvent) {
	jsonEvent, err := json.Marshal(evt)
	if err != nil {
		log.Errorf("error while marshaling tracer event: %s", err)
		return
	}

	for _, t := range lt.tt {
		err = t.Transport(jsonEvent)
		if err != nil {
			log.Errorf("error while transporting trace event: %s", err)
		}
	}
}
