package tracer

import (
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type testTracerTransport struct {
	t           *testing.T
	executeTest func(t *testing.T, evt TracerTransportEvent)
}

const peerIDA peer.ID = "12D3KooWAbSVMgRejb6ECg6fRTkCPGCfu8396msZVryu8ivcz44G"

func NewTestTraceTransport(t *testing.T, executeTest func(t *testing.T, evt TracerTransportEvent)) TracerTransport {
	return &testTracerTransport{
		t:           t,
		executeTest: executeTest,
	}
}

func (ttt *testTracerTransport) Transport(evt TracerTransportEvent) error {
	ttt.executeTest(ttt.t, evt)
	return nil
}

func TestTracer_PeerScores(t *testing.T) {
	testTransport := NewTestTraceTransport(t, func(t *testing.T, evt TracerTransportEvent) {
		require.Equal(t, []byte(peerIDA), evt.lotusTraceEvent.PeerID)
		require.Equal(t, "source-auth-token-test", evt.lotusTraceEvent.SourceAuth)
		require.Equal(t, float64(32), evt.lotusTraceEvent.PeerScore.Score)

		n := time.Now().UnixNano()
		require.LessOrEqual(t, *evt.lotusTraceEvent.Timestamp, n)

		require.Equal(t, []byte(peerIDA), evt.lotusTraceEvent.PeerScore.PeerID)
		require.Equal(t, 1, len(evt.lotusTraceEvent.PeerScore.Topics))

		topic := evt.lotusTraceEvent.PeerScore.Topics[0]
		require.Equal(t, "topicA", topic.Topic)
		require.Equal(t, float64(100), topic.FirstMessageDeliveries)
	})

	lt := NewLotusTracer(
		[]TracerTransport{testTransport},
		peerIDA,
		"source-auth-token-test",
	)

	topics := make(map[string]*pubsub.TopicScoreSnapshot)
	topics["topicA"] = &pubsub.TopicScoreSnapshot{
		FirstMessageDeliveries: float64(100),
	}

	m := make(map[peer.ID]*pubsub.PeerScoreSnapshot)
	m[peerIDA] = &pubsub.PeerScoreSnapshot{
		Score:  float64(32),
		Topics: topics,
	}

	lt.PeerScores(m)
}

func TestTracer_PubSubTrace(t *testing.T) {
	n := time.Now().Unix()

	testTransport := NewTestTraceTransport(t, func(t *testing.T, evt TracerTransportEvent) {
		require.Equal(t, []byte(peerIDA), evt.pubsubTraceEvent.PeerID)
		require.Equal(t, &n, evt.pubsubTraceEvent.Timestamp)
	})

	lt := NewLotusTracer(
		[]TracerTransport{testTransport},
		"pid",
		"source-auth",
	)

	lt.Trace(&pubsub_pb.TraceEvent{
		PeerID:    []byte(peerIDA),
		Timestamp: &n,
	})
}

func TestTracer_MultipleTransports(t *testing.T) {
	testTransportA := NewTestTraceTransport(t, func(t *testing.T, evt TracerTransportEvent) {
		require.Equal(t, []byte(peerIDA), evt.pubsubTraceEvent.PeerID)
	})

	testTransportB := NewTestTraceTransport(t, func(t *testing.T, evt TracerTransportEvent) {
		require.Equal(t, []byte(peerIDA), evt.pubsubTraceEvent.PeerID)
	})

	executeTest := NewLotusTracer(
		[]TracerTransport{testTransportA, testTransportB},
		"pid",
		"source-auth",
	)

	executeTest.Trace(&pubsub_pb.TraceEvent{
		PeerID: []byte(peerIDA),
	})
}
