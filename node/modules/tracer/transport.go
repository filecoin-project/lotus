package tracer

type TracerTransport interface {
	Transport(jsonEvent []byte) error
}
