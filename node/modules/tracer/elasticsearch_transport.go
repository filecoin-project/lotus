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

func (est *elasticSearchTransport) Transport(jsonEvent []byte) error {
	req := esapi.IndexRequest{
		Index:      ElasticSearch_INDEX,
		DocumentID: "1", // todo
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
