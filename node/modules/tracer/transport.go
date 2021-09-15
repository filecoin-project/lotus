package tracer

import pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"

type TracerTransport interface {
	Transport(jsonEvent TracerTransportEvent) error
}

type TracerTransportEvent struct {
	lotusTraceEvent  *LotusTraceEvent
	pubsubTraceEvent *pubsub_pb.TraceEvent
}
