package tracer

import (
	"encoding/json"
	"fmt"
	"os"
)

type jsonTracerTransport struct {
	out *os.File
}

func NewJsonTracerTransport(file string) (TracerTransport, error) {
	out, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0660)
	if err != nil {
		return nil, err
	}

	return &jsonTracerTransport{
		out: out,
	}, nil
}

func (jtt *jsonTracerTransport) Transport(evt TracerTransportEvent) error {
	var e interface{}
	if evt.lotusTraceEvent != nil {
		e = *evt.lotusTraceEvent
	} else if evt.pubsubTraceEvent != nil {
		e = *evt.pubsubTraceEvent
	} else {
		return nil
	}

	jsonEvt, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("error while marshaling event: %s", err)
	}

	_, err = jtt.out.WriteString(string(jsonEvt) + "\n")
	return err
}
