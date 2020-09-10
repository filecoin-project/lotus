package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-ipfs-blockstore"
	"github.com/ipfs/go-ipfs-chunker"
	"github.com/ipfs/go-ipfs-exchange-offline"
	"github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs/importer/balanced"
	ihelper "github.com/ipfs/go-unixfs/importer/helpers"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/testground/sdk-go/network"

	gs "github.com/ipfs/go-graphsync"
	gsi "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-noise"
	"github.com/libp2p/go-libp2p-secio"
	tls "github.com/libp2p/go-libp2p-tls"

	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

var testcases = map[string]interface{}{
	"stress": run.InitializedTestCaseFn(runStress),
}

func main() {
	run.InvokeMap(testcases)
}

func runStress(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	var (
		size       = runenv.IntParam("size")
		bandwidths = runenv.SizeArrayParam("bandwidths")
		latencies  []time.Duration
	)

	lats := runenv.StringArrayParam("latencies")
	for _, l := range lats {
		d, err := time.ParseDuration(l)
		if err != nil {
			return err
		}
		latencies = append(latencies, d)
	}

	runenv.RecordMessage("started test instance")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	initCtx.MustWaitAllInstancesInitialized(ctx)
	defer initCtx.SyncClient.MustSignalAndWait(ctx, "done", runenv.TestInstanceCount)

	host, peers := makeHost(ctx, runenv, initCtx)
	defer host.Close()

	var (
		// make datastore, blockstore, dag service, graphsync
		ds     = dss.MutexWrap(datastore.NewMapDatastore())
		bs     = blockstore.NewBlockstore(ds)
		dagsrv = merkledag.NewDAGService(blockservice.New(bs, offline.Exchange(bs)))
		gsync  = gsi.New(ctx, gsnet.NewFromLibp2pHost(host), makeLoader(bs), makeStorer(bs))
	)

	switch runenv.TestGroupID {
	case "providers":
		if runenv.TestGroupInstanceCount > 1 {
			panic("test case only supports one provider")
		}

		runenv.RecordMessage("we are the provider")
		defer runenv.RecordMessage("done provider")

		return runProvider(ctx, runenv, initCtx, dagsrv, size, latencies, bandwidths)

	case "requestors":
		runenv.RecordMessage("we are the requestor")
		defer runenv.RecordMessage("done requestor")

		p := *peers[0]
		if err := host.Connect(ctx, p); err != nil {
			return err
		}
		runenv.RecordMessage("done dialling provider")
		return runRequestor(ctx, runenv, initCtx, gsync, p, bs, latencies, bandwidths)

	default:
		panic("unsupported group ID")
	}
}

func runRequestor(ctx context.Context, runenv *runtime.RunEnv, initCtx *run.InitContext, gsync gs.GraphExchange, p peer.AddrInfo, bs blockstore.Blockstore, latencies []time.Duration, bandwidths []uint64) error {
	// create a selector for the whole UnixFS dag
	ssb := builder.NewSelectorSpecBuilder(basicnode.Style.Any)
	sel := ssb.ExploreRecursive(
		selector.RecursionLimitNone(),
		ssb.ExploreAll(
			ssb.ExploreRecursiveEdge()),
	).Node()

	for i, latency := range latencies {
		for j, bandwidth := range bandwidths {
			round := i*len(latencies) + j

			var (
				topicCid  = sync.NewTopic(fmt.Sprintf("cid-%d", round), new(cid.Cid))
				stateNext = sync.State(fmt.Sprintf("next-%d", round))
				stateNet  = sync.State(fmt.Sprintf("network-configured-%d", round))
			)

			runenv.RecordMessage("waiting to start round %d", round)
			initCtx.SyncClient.MustSignalAndWait(ctx, stateNext, runenv.TestInstanceCount)

			sctx, scancel := context.WithCancel(ctx)
			cidCh := make(chan *cid.Cid, 1)
			initCtx.SyncClient.MustSubscribe(sctx, topicCid, cidCh)
			cid := <-cidCh
			scancel()

			// make a go-ipld-prime link for the root UnixFS node
			clink := cidlink.Link{Cid: *cid}

			runenv.RecordMessage("ROUND %d: latency=%s, bandwidth=%d", round, latency, bandwidth)
			runenv.RecordMessage("CID: %s", cid)

			runenv.RecordMessage("waiting for provider's network to be configured %d", round)
			<-initCtx.SyncClient.MustBarrier(ctx, stateNet, 1).C
			runenv.RecordMessage("network configured for round %d", round)

			// execute the traversal.
			runenv.RecordMessage(">>>>> requesting")
			progressCh, errCh := gsync.Request(ctx, p.ID, clink, sel)
			for r := range progressCh {
				runenv.RecordMessage("******* progress: %+v", r)
			}

			runenv.RecordMessage("<<<<< request complete")
			if len(errCh) > 0 {
				return <-errCh
			}
		}
	}

	return nil
}

func runProvider(ctx context.Context, runenv *runtime.RunEnv, initCtx *run.InitContext, dagsrv format.DAGService, size int, latencies []time.Duration, bandwidths []uint64) error {
	for i, latency := range latencies {
		for j, bandwidth := range bandwidths {
			round := i*len(latencies) + j

			var (
				topicCid  = sync.NewTopic(fmt.Sprintf("cid-%d", round), new(cid.Cid))
				stateNext = sync.State(fmt.Sprintf("next-%d", round))
				stateNet  = sync.State(fmt.Sprintf("network-configured-%d", round))
			)

			runenv.RecordMessage("waiting to start round %d", round)
			initCtx.SyncClient.MustSignalAndWait(ctx, stateNext, runenv.TestInstanceCount)

			// file with random data
			file := files.NewReaderFile(io.LimitReader(rand.Reader, int64(size)))

			// import to UnixFS
			bufferedDS := format.NewBufferedDAG(ctx, dagsrv)

			const unixfsChunkSize uint64 = 1 << 10
			const unixfsLinksPerLevel = 1024

			params := ihelper.DagBuilderParams{
				Maxlinks:   unixfsLinksPerLevel,
				RawLeaves:  true,
				CidBuilder: nil,
				Dagserv:    bufferedDS,
			}

			db, err := params.New(chunk.NewSizeSplitter(file, int64(unixfsChunkSize)))
			if err != nil {
				return fmt.Errorf("unable to setup dag builder: %w", err)
			}

			node, err := balanced.Layout(db)
			if err != nil {
				return fmt.Errorf("unable to create unix fs node: %w", err)
			}

			err = bufferedDS.Commit()
			if err != nil {
				return fmt.Errorf("unable to commit unix fs node: %w", err)
			}

			runenv.RecordMessage("CID is: %s", node.Cid())

			initCtx.SyncClient.MustPublish(ctx, topicCid, node.Cid())

			runenv.RecordMessage("ROUND %d: latency=%s, bandwidth=%d", round, latency, bandwidth)

			runenv.RecordMessage("configuring network for round %d", round)
			initCtx.NetClient.MustConfigureNetwork(ctx, &network.Config{
				Network: "default",
				Enable:  true,
				Default: network.LinkShape{
					Latency:   latency,
					Bandwidth: bandwidth,
				},
				CallbackState:  stateNet,
				CallbackTarget: 1,
			})
			runenv.RecordMessage("network configured for round %d", round)
		}
	}

	return nil
}

func makeHost(ctx context.Context, runenv *runtime.RunEnv, initCtx *run.InitContext) (host.Host, []*peer.AddrInfo) {
	secureChannel := runenv.StringParam("secure_channel")

	var security libp2p.Option
	switch secureChannel {
	case "noise":
		security = libp2p.Security(noise.ID, noise.New)
	case "secio":
		security = libp2p.Security(secio.ID, secio.New)
	case "tls":
		security = libp2p.Security(tls.ID, tls.New)
	}

	// ☎️  Let's construct the libp2p node.
	ip := initCtx.NetClient.MustGetDataNetworkIP()
	listenAddr := fmt.Sprintf("/ip4/%s/tcp/0", ip)
	host, err := libp2p.New(ctx,
		security,
		libp2p.ListenAddrStrings(listenAddr),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to instantiate libp2p instance: %s", err))
	}

	// Record our listen addrs.
	runenv.RecordMessage("my listen addrs: %v", host.Addrs())

	// Obtain our own address info, and use the sync service to publish it to a
	// 'peersTopic' topic, where others will read from.
	var (
		id = host.ID()
		ai = &peer.AddrInfo{ID: id, Addrs: host.Addrs()}

		// the peers topic where all instances will advertise their AddrInfo.
		peersTopic = sync.NewTopic("peers", new(peer.AddrInfo))

		// initialize a slice to store the AddrInfos of all other peers in the run.
		peers = make([]*peer.AddrInfo, 0, runenv.TestInstanceCount-1)
	)

	// Publish our own.
	initCtx.SyncClient.MustPublish(ctx, peersTopic, ai)

	// Now subscribe to the peers topic and consume all addresses, storing them
	// in the peers slice.
	peersCh := make(chan *peer.AddrInfo)
	sctx, scancel := context.WithCancel(ctx)
	defer scancel()

	sub := initCtx.SyncClient.MustSubscribe(sctx, peersTopic, peersCh)

	// Receive the expected number of AddrInfos.
	for len(peers) < cap(peers) {
		select {
		case ai := <-peersCh:
			if ai.ID == id {
				continue // skip over ourselves.
			}
			peers = append(peers, ai)
		case err := <-sub.Done():
			panic(err)
		}
	}

	return host, peers
}

func makeLoader(bs blockstore.Blockstore) ipld.Loader {
	return func(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
		c, ok := lnk.(cidlink.Link)
		if !ok {
			return nil, errors.New("incorrect link type")
		}
		// read block from one store
		block, err := bs.Get(c.Cid)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(block.RawData()), nil
	}
}

func makeStorer(bs blockstore.Blockstore) ipld.Storer {
	return func(lnkCtx ipld.LinkContext) (io.Writer, ipld.StoreCommitter, error) {
		var buf bytes.Buffer
		var committer ipld.StoreCommitter = func(lnk ipld.Link) error {
			c, ok := lnk.(cidlink.Link)
			if !ok {
				return errors.New("incorrect link type")
			}
			block, err := blocks.NewBlockWithCid(buf.Bytes(), c.Cid)
			if err != nil {
				return err
			}
			return bs.Put(block)
		}
		return &buf, committer, nil
	}
}
