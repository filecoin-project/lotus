package tracer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
)

const (
	ElasticSearchDefaultIndex = "lotus-pubsub"
)

func NewElasticSearchTransport(connectionString string, elasticsearchIndex string) (TracerTransport, error) {
	conUrl, err := url.Parse(connectionString)

	if err != nil {
		return nil, err
	}

	username := conUrl.User.Username()
	password, _ := conUrl.User.Password()
	cfg := elasticsearch.Config{
		Addresses: []string{
			conUrl.Scheme + "://" + conUrl.Host,
		},
		Username: username,
		Password: password,
	}

	es, err := elasticsearch.NewClient(cfg)

	if err != nil {
		return nil, err
	}

	var esIndex string
	if elasticsearchIndex != "" {
		esIndex = elasticsearchIndex
	} else {
		esIndex = ElasticSearchDefaultIndex
	}

	return &elasticSearchTransport{
		cl:      es,
		esIndex: esIndex,
	}, nil
}

type elasticSearchTransport struct {
	cl      *elasticsearch.Client
	esIndex string
}

func (est *elasticSearchTransport) Transport(evt TracerTransportEvent) error {
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

	req := esapi.IndexRequest{
		Index:   est.esIndex,
		Body:    strings.NewReader(string(jsonEvt)),
		Refresh: "true",
	}

	// Perform the request with the client.
	res, err := req.Do(context.Background(), est.cl)
	if err != nil {
		return err
	}

	err = res.Body.Close()
	if err != nil {
		return err
	}

	if res.IsError() {
		return fmt.Errorf("[%s] Error indexing document ID=%s", res.Status(), req.DocumentID)
	}

	return nil
}
