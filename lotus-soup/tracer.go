package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"

	"github.com/testground/sdk-go/sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-pubsub-tracer/traced"

	ma "github.com/multiformats/go-multiaddr"
)

var (
	pubsubTracerTopic = sync.NewTopic("pubsubTracer", &PubsubTracerMsg{})
)

type PubsubTracer struct {
	host   host.Host
	traced *traced.TraceCollector
}

type PubsubTracerMsg struct {
	Tracer string
}

func (tr *PubsubTracer) Stop() {
	tr.traced.Stop()
	err := tr.host.Close()
	if err != nil {
		log.Printf("error closing host: %s", err)
	}
}

func preparePubsubTracer(t *TestEnvironment) (*PubsubTracer, error) {
	ctx := context.Background()

	privk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	tracedIP := t.NetClient.MustGetDataNetworkIP().String()
	tracedAddr := fmt.Sprintf("/ip4/%s/tcp/4001", tracedIP)

	host, err := libp2p.New(ctx,
		libp2p.Identity(privk),
		libp2p.ListenAddrStrings(tracedAddr),
	)
	if err != nil {
		return nil, err
	}

	traced, err := traced.NewTraceCollector(host, "traced.logs")
	if err != nil {
		host.Close()
		return nil, err
	}

	tracedMultiaddrStr := fmt.Sprintf("%s/p2p/%s", tracedAddr, host.ID())
	t.RecordMessage("I am %s", tracedMultiaddrStr)

	tracedMultiaddr := ma.StringCast(tracedMultiaddrStr)
	tracedMsg := &PubsubTracerMsg{Tracer: tracedMultiaddr.String()}
	t.SyncClient.MustPublish(ctx, pubsubTracerTopic, tracedMsg)

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, stateReady, t.TestInstanceCount)

	return &PubsubTracer{host: host, traced: traced}, nil
}

func runPubsubTracer(t *TestEnvironment) error {
	t.RecordMessage("running pubsub tracer")
	tracer, err := preparePubsubTracer(t)
	if err != nil {
		return err
	}

	defer tracer.Stop()

	ctx := context.Background()
	t.SyncClient.MustSignalAndWait(ctx, stateDone, t.TestInstanceCount)
	return nil
}
