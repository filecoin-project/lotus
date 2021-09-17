package tracer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/elastic/go-elasticsearch/v7/estransport"
)

const (
	ElasticSearch_INDEX = "lotus2"

	ElasticSearch_DOC_LOTUS  = "doc_lotus"
	ElasticSearch_DOC_PUBSUB = "doc_pubsub"
)

func NewElasticSearchTransport(connectionString string) (TracerTransport, error) {
	conUrl, err := url.Parse(connectionString)

	username := conUrl.User.Username()
	password, _ := conUrl.User.Password()
	cfg := elasticsearch.Config{
		Addresses: []string{
			"https://" + conUrl.Host,
		},
		Username: username,
		Password: password,
		Logger: &estransport.CurlLogger{
			Output:             os.Stdout,
			EnableRequestBody:  true,
			EnableResponseBody: true,
		},
	}

	es, err := elasticsearch.NewClient(cfg)

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

func (est *elasticSearchTransport) Transport(evt TracerTransportEvent) error {
	var e interface{}
	var docId string
	if evt.lotusTraceEvent != nil {
		e = *evt.lotusTraceEvent
		docId = ElasticSearch_DOC_PUBSUB
	} else if evt.pubsubTraceEvent != nil {
		e = *evt.pubsubTraceEvent
		docId = ElasticSearch_DOC_PUBSUB
	} else {
		return nil
	}

	jsonEvt, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("error while marshaling event: %s", err)
	}

	req := esapi.IndexRequest{
		Index:      ElasticSearch_INDEX,
		DocumentID: docId,
		Body:       strings.NewReader(string(jsonEvt)),
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
	}
	return nil
}
