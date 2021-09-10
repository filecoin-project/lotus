package tracer

import (
	"encoding/json"

	logging "github.com/ipfs/go-log/v2"
	peer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

var log = logging.Logger("lotus-tracer")

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

func (lt *lotusTracer) TracePeerScore(scores map[peer.ID]*pubsub.PeerScoreSnapshot) {
	jsonEvent, err := json.Marshal(scores)
	if err != nil {
		log.Errorf("error while marshaling peer score: %s", err)
		return
	}

	err = lt.tt.Transport(jsonEvent)
	if err != nil {
		log.Errorf("error while transporting peer scores: %s", err)
	}
}

func (lt *lotusTracer) Trace(evt *pubsub_pb.TraceEvent) {
	jsonEvent, err := json.Marshal(evt)
	if err != nil {
		log.Errorf("error while marshaling tracer event: %s", err)
		return
	}

	err = lt.tt.Transport(jsonEvent)
	if err != nil {
		log.Errorf("error while transporting trace event: %s", err)
	}
}
