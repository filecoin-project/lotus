package tracer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esutil"
)

const (
	ElasticSearchDefaultIndex = "lotus-pubsub"
	flushInterval             = 10 * time.Second
	flushBytes                = 1024 * 1024 // MB
	esWorkers                 = 2           // TODO: hardcoded
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
		Username:  username,
		Password:  password,
		Transport: &http.Transport{},
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

	// Create the BulkIndexer to batch ES trace submission
	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         esIndex,
		Client:        es,
		NumWorkers:    esWorkers,
		FlushBytes:    int(flushBytes),
		FlushInterval: flushInterval,
		OnError: func(ctx context.Context, err error) {
			log.Errorf("Error persisting queries %s", err.Error())
		},
	})
	if err != nil {
		return nil, err
	}

	return &elasticSearchTransport{
		cl:      es,
		bi:      bi,
		esIndex: esIndex,
	}, nil
}

type elasticSearchTransport struct {
	cl      *elasticsearch.Client
	bi      esutil.BulkIndexer
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

	return est.bi.Add(
		context.Background(),
		esutil.BulkIndexerItem{
			Action: "index",
			Body:   bytes.NewReader(jsonEvt),
			OnFailure: func(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem, err error) {
				if err != nil {
					log.Errorf("unable to submit trace - %s", err)
				} else {
					log.Errorf("unable to submit trace %s: %s", res.Error.Type, res.Error.Reason)
				}
			},
		},
	)
}
