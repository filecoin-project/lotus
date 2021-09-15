package tracer

import (
	"os"
)

type jsonTracerTransport struct {
	out *os.File
}

func NewJsonTracerTransport(out *os.File) TracerTransport {
	return &jsonTracerTransport{
		out: out,
	}
}

func (jtt *jsonTracerTransport) Transport(jsonEvent []byte) error {
	_, err := jtt.out.Write(jsonEvent)
	return err
}
