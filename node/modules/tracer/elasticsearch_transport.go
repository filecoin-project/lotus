package tracer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
)

const (
	ElasticSearch_INDEX = "pubsub"

	ElasticSearch_DOC_LOTUS  = "doc_lotus"
	ElasticSearch_DOC_PUBSUB = "doc_pubsub"
)

func NewElasticSearchTransport() (TracerTransport, error) {
	es, err := elasticsearch.NewDefaultClient()

	if err != nil {
		return nil, err
	}

	return &elasticSearchTransport{
		cl: es,
	}, nil
}

type elasticSearchTransport struct {
	cl *elasticsearch.Client
}

func (est *elasticSearchTransport) Transport(event TracerTransportEvent) error {
	var e interface{}
	var docId string
	if event.lotusTraceEvent != nil {
		e = *event.lotusTraceEvent
		docId = ElasticSearch_DOC_LOTUS
	} else if event.pubsubTraceEvent != nil {
		e = *event.pubsubTraceEvent
		docId = ElasticSearch_DOC_PUBSUB
	} else {
		return nil
	}

	jsonEvent, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("error while marshaling event: %s", err)
	}

	req := esapi.IndexRequest{
		Index:      ElasticSearch_INDEX,
		DocumentID: docId,
		Body:       strings.NewReader(string(jsonEvent)),
		Refresh:    "true",
	}

	// Perform the request with the client.
	res, err := req.Do(context.Background(), est.cl)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("[%s] Error indexing document ID=%s", res.Status(), req.DocumentID)
	} else {
		// Deserialize the response into a map.
		var r map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
			return err
		}
	}
	return nil
}
