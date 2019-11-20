package graphsyncimpl
import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-graphsync"
	"github.com/ipfs/go-graphsync/ipldbridge"
	gsnet "github.com/ipfs/go-graphsync/network"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	ipldformat "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipld/go-ipld-prime"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/datatransfer"
)

func TestGraphsyncImpl_SubscribeToEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gsData := newGraphsyncTestingData(t, ctx)
	host1 := gsData.host1
	gs1 := &fakeGraphSync{
		receivedRequests: make(chan receivedGraphSyncRequest, 1),
	}
	dt1 := NewGraphSyncDataTransfer(ctx, host1, gs1)

	subscribe1Calls := make(chan struct{}, 1)
	subscriber := func(event datatransfer.Event, channelState datatransfer.ChannelState) {
		if event == datatransfer.Error {
			subscribe1Calls <- struct{}{}
		}
	}

	subscribe2Calls := make(chan struct{}, 1)
	subscriber2 := func(event datatransfer.Event, cst datatransfer.ChannelState) {
		if event != datatransfer.Error {
			subscribe2Calls <- struct{}{}
		}
	}

	unsubFunc := dt1.SubscribeToEvents(subscriber)
	impl := dt1.(*graphsyncImpl)
	assert.Equal(t, 1, len(impl.subscribers))

	unsubFunc2 := impl.SubscribeToEvents(subscriber2)
	assert.Equal(t, 2, len(impl.subscribers))

	//  ensure subsequent calls don't cause errors, and also check that the right item
	// is removed, i.e. no false positives.
	unsubFunc()
	unsubFunc()
	assert.Equal(t, 1, len(impl.subscribers))

	// ensure it can delete all elems
	unsubFunc2()
	assert.Equal(t, 0, len(impl.subscribers))
}

func newGraphsyncTestingData(t *testing.T, ctx context.Context) *graphsyncTestingData {

	gsData := &graphsyncTestingData{}
	gsData.ctx = ctx
	makeLoader := func(bs bstore.Blockstore) ipld.Loader {
		return func(lnk ipld.Link, lnkCtx ipld.LinkContext) (io.Reader, error) {
			c, ok := lnk.(cidlink.Link)
			if !ok {
				return nil, errors.New("Incorrect Link Type")
			}
			// read block from one store
			block, err := bs.Get(c.Cid)
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(block.RawData()), nil
		}
	}

	makeStorer := func(bs bstore.Blockstore) ipld.Storer {
		return func(lnkCtx ipld.LinkContext) (io.Writer, ipld.StoreCommitter, error) {
			var buf bytes.Buffer
			var committer ipld.StoreCommitter = func(lnk ipld.Link) error {
				c, ok := lnk.(cidlink.Link)
				if !ok {
					return errors.New("Incorrect Link Type")
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
	// make a blockstore and dag service
	gsData.bs1 = bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))
	gsData.bs2 = bstore.NewBlockstore(dss.MutexWrap(datastore.NewMapDatastore()))

	gsData.dagService1 = merkledag.NewDAGService(blockservice.New(gsData.bs1, offline.Exchange(gsData.bs1)))
	gsData.dagService2 = merkledag.NewDAGService(blockservice.New(gsData.bs2, offline.Exchange(gsData.bs2)))

	// setup an IPLD loader/storer for blockstore 1
	gsData.loader1 = makeLoader(gsData.bs1)
	gsData.storer1 = makeStorer(gsData.bs1)

	// setup an IPLD loader/storer for blockstore 2
	gsData.loader2 = makeLoader(gsData.bs2)
	gsData.storer2 = makeStorer(gsData.bs2)

	mn := mocknet.New(ctx)

	// setup network
	var err error
	gsData.host1, err = mn.GenPeer()
	require.NoError(t, err)

	gsData.host2, err = mn.GenPeer()
	require.NoError(t, err)

	err = mn.LinkAll()
	require.NoError(t, err)

	gsData.gsNet1 = gsnet.NewFromLibp2pHost(gsData.host1)
	gsData.gsNet2 = gsnet.NewFromLibp2pHost(gsData.host2)

	gsData.bridge1 = ipldbridge.NewIPLDBridge()
	gsData.bridge2 = ipldbridge.NewIPLDBridge()

	// create a selector for the whole UnixFS dag
	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	gsData.allSelector = ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	return gsData
}


type receivedGraphSyncRequest struct {
	p          peer.ID
	root       ipld.Link
	selector   ipld.Node
	extensions []graphsync.ExtensionData
}

type fakeGraphSync struct {
	receivedRequests chan receivedGraphSyncRequest
}

// Request initiates a new GraphSync request to the given peer using the given selector spec.
func (fgs *fakeGraphSync) Request(ctx context.Context, p peer.ID, root ipld.Link, selector ipld.Node, extensions ...graphsync.ExtensionData) (<-chan graphsync.ResponseProgress, <-chan error) {
	fgs.receivedRequests <- receivedGraphSyncRequest{p, root, selector, extensions}
	responses := make(chan graphsync.ResponseProgress)
	errors := make(chan error)
	close(responses)
	close(errors)
	return responses, errors
}

// RegisterResponseReceivedHook adds a hook that runs when a request is received
func (fgs *fakeGraphSync)RegisterRequestReceivedHook(overrideDefaultValidation bool, hook graphsync.OnRequestReceivedHook) error {
	return nil
}

// RegisterResponseReceivedHook adds a hook that runs when a response is received
func (fgs *fakeGraphSync)RegisterResponseReceivedHook(graphsync.OnResponseReceivedHook) error {
	return nil
}

type graphsyncTestingData struct {
	ctx         context.Context
	bs1         bstore.Blockstore
	bs2         bstore.Blockstore
	dagService1 ipldformat.DAGService
	dagService2 ipldformat.DAGService
	loader1     ipld.Loader
	loader2     ipld.Loader
	storer1     ipld.Storer
	storer2     ipld.Storer
	host1       host.Host
	host2       host.Host
	gsNet1      gsnet.GraphSyncNetwork
	gsNet2      gsnet.GraphSyncNetwork
	bridge1     ipldbridge.IPLDBridge
	bridge2     ipldbridge.IPLDBridge
	allSelector ipld.Node
	origBytes   []byte
}