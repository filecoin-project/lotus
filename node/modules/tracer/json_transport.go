package tracer

import (
	"encoding/json"
	"fmt"
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

func (jtt *jsonTracerTransport) Transport(event TracerTransportEvent) error {
	var e interface{}
	if event.lotusTraceEvent != nil {
		e = *event.lotusTraceEvent
	} else if event.pubsubTraceEvent != nil {
		e = *event.pubsubTraceEvent
	} else {
		return nil
	}

	jsonEvent, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("error while marshaling event: %s", err)
	}

	_, err = jtt.out.Write(jsonEvent)
	return err
}
