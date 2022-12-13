package network_test

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/network"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

type testReceiver struct {
	t                  *testing.T
	queryStreamHandler func(network.RetrievalQueryStream)
}

func (tr *testReceiver) HandleQueryStream(s network.RetrievalQueryStream) {
	defer s.Close()
	if tr.queryStreamHandler != nil {
		tr.queryStreamHandler(s)
	}
}

func TestQueryStreamSendReceiveQuery(t *testing.T) {
	ctx := context.Background()

	testCases := map[string]struct {
		senderDisabledNew   bool
		receiverDisabledNew bool
	}{
		"both clients current version": {},
		"sender old supports old queries": {
			senderDisabledNew: true,
		},
		"receiver only supports old queries": {
			receiverDisabledNew: true,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			td := shared_testutil.NewLibp2pTestData(ctx, t)
			var fromNetwork, toNetwork network.RetrievalMarketNetwork
			if data.senderDisabledNew {
				fromNetwork = network.NewFromLibp2pHost(td.Host1, network.SupportedProtocols([]protocol.ID{retrievalmarket.OldQueryProtocolID}))
			} else {
				fromNetwork = network.NewFromLibp2pHost(td.Host1)
			}
			if data.receiverDisabledNew {
				toNetwork = network.NewFromLibp2pHost(td.Host2, network.SupportedProtocols([]protocol.ID{retrievalmarket.OldQueryProtocolID}))
			} else {
				toNetwork = network.NewFromLibp2pHost(td.Host2)
			}
			toHost := td.Host2.ID()

			// host1 gets no-op receiver
			tr := &testReceiver{t: t}
			require.NoError(t, fromNetwork.SetDelegate(tr))

			// host2 gets receiver
			qchan := make(chan retrievalmarket.Query)
			tr2 := &testReceiver{t: t, queryStreamHandler: func(s network.RetrievalQueryStream) {
				readq, err := s.ReadQuery()
				require.NoError(t, err)
				qchan <- readq
			}}
			require.NoError(t, toNetwork.SetDelegate(tr2))

			// setup query stream host1 --> host 2
			assertQueryReceived(ctx, t, fromNetwork, toHost, qchan)
		})
	}
}

func TestQueryStreamSendReceiveQueryResponse(t *testing.T) {
	ctx := context.Background()

	testCases := map[string]struct {
		senderDisabledNew   bool
		receiverDisabledNew bool
	}{
		"both clients current version": {},
		"sender old supports old queries": {
			senderDisabledNew: true,
		},
		"receiver only supports old queries": {
			receiverDisabledNew: true,
		},
	}
	for testCase, data := range testCases {
		t.Run(testCase, func(t *testing.T) {
			td := shared_testutil.NewLibp2pTestData(ctx, t)
			var fromNetwork, toNetwork network.RetrievalMarketNetwork
			if data.senderDisabledNew {
				fromNetwork = network.NewFromLibp2pHost(td.Host1, network.SupportedProtocols([]protocol.ID{retrievalmarket.OldQueryProtocolID}))
			} else {
				fromNetwork = network.NewFromLibp2pHost(td.Host1)
			}
			if data.receiverDisabledNew {
				toNetwork = network.NewFromLibp2pHost(td.Host2, network.SupportedProtocols([]protocol.ID{retrievalmarket.OldQueryProtocolID}))
			} else {
				toNetwork = network.NewFromLibp2pHost(td.Host2)
			}
			toHost := td.Host2.ID()

			// host1 gets no-op receiver
			tr := &testReceiver{t: t}
			require.NoError(t, fromNetwork.SetDelegate(tr))

			// host2 gets receiver
			qchan := make(chan retrievalmarket.QueryResponse)
			tr2 := &testReceiver{t: t, queryStreamHandler: func(s network.RetrievalQueryStream) {
				q, err := s.ReadQueryResponse()
				require.NoError(t, err)
				qchan <- q
			}}
			require.NoError(t, toNetwork.SetDelegate(tr2))

			assertQueryResponseReceived(ctx, t, fromNetwork, toHost, qchan)
		})
	}
}

func TestQueryStreamSendReceiveMultipleSuccessful(t *testing.T) {
	// send query, read in handler, send response back, read response
	ctxBg := context.Background()
	td := shared_testutil.NewLibp2pTestData(ctxBg, t)
	nw1 := network.NewFromLibp2pHost(td.Host1)
	nw2 := network.NewFromLibp2pHost(td.Host2)
	require.NoError(t, td.Host1.Connect(ctxBg, peer.AddrInfo{ID: td.Host2.ID()}))

	// host2 gets a query and sends a response
	qr := shared_testutil.MakeTestQueryResponse()
	done := make(chan bool)
	tr2 := &testReceiver{t: t, queryStreamHandler: func(s network.RetrievalQueryStream) {
		_, err := s.ReadQuery()
		require.NoError(t, err)

		require.NoError(t, s.WriteQueryResponse(qr))
		done <- true
	}}
	require.NoError(t, nw2.SetDelegate(tr2))

	ctx, cancel := context.WithTimeout(ctxBg, 10*time.Second)
	defer cancel()

	qs, err := nw1.NewQueryStream(td.Host2.ID())
	require.NoError(t, err)

	testCid := shared_testutil.GenerateCids(1)[0]

	var resp retrievalmarket.QueryResponse
	go require.NoError(t, qs.WriteQuery(retrievalmarket.Query{PayloadCID: testCid}))
	resp, err = qs.ReadQueryResponse()
	require.NoError(t, err)

	select {
	case <-ctx.Done():
		t.Error("response not received")
	case <-done:
	}

	assert.Equal(t, qr, resp)
}

func TestLibp2pRetrievalMarketNetwork_StopHandlingRequests(t *testing.T) {
	bgCtx := context.Background()
	td := shared_testutil.NewLibp2pTestData(bgCtx, t)

	fromNetwork := network.NewFromLibp2pHost(td.Host1, network.RetryParameters(0, 0, 0, 0))
	toNetwork := network.NewFromLibp2pHost(td.Host2)
	toHost := td.Host2.ID()

	// host1 gets no-op receiver
	tr := &testReceiver{t: t}
	require.NoError(t, fromNetwork.SetDelegate(tr))

	// host2 gets receiver
	qchan := make(chan retrievalmarket.Query)
	tr2 := &testReceiver{t: t, queryStreamHandler: func(s network.RetrievalQueryStream) {
		readq, err := s.ReadQuery()
		require.NoError(t, err)
		qchan <- readq
	}}
	require.NoError(t, toNetwork.SetDelegate(tr2))

	require.NoError(t, toNetwork.StopHandlingRequests())

	_, err := fromNetwork.NewQueryStream(toHost)
	require.Error(t, err, "protocol not supported")
}

// assertQueryReceived performs the verification that a DealStatusRequest is received
func assertQueryReceived(inCtx context.Context, t *testing.T, fromNetwork network.RetrievalMarketNetwork, toHost peer.ID, qchan chan retrievalmarket.Query) {
	ctx, cancel := context.WithTimeout(inCtx, 10*time.Second)
	defer cancel()

	qs1, err := fromNetwork.NewQueryStream(toHost)
	require.NoError(t, err)

	// send query to host2
	cids := shared_testutil.GenerateCids(2)
	q := retrievalmarket.NewQueryV1(cids[0], &cids[1])
	require.NoError(t, qs1.WriteQuery(q))

	var inq retrievalmarket.Query
	select {
	case <-ctx.Done():
		t.Error("msg not received")
	case inq = <-qchan:
	}
	require.NotNil(t, inq)
	assert.Equal(t, q.PayloadCID, inq.PayloadCID)
	assert.Equal(t, q.PieceCID, inq.PieceCID)
}

// assertQueryResponseReceived performs the verification that a DealStatusResponse is received
func assertQueryResponseReceived(inCtx context.Context, t *testing.T,
	fromNetwork network.RetrievalMarketNetwork,
	toHost peer.ID,
	qchan chan retrievalmarket.QueryResponse) {
	ctx, cancel := context.WithTimeout(inCtx, 10*time.Second)
	defer cancel()

	// setup query stream host1 --> host 2
	qs1, err := fromNetwork.NewQueryStream(toHost)
	require.NoError(t, err)

	// send queryresponse to host2
	qr := shared_testutil.MakeTestQueryResponse()
	require.NoError(t, qs1.WriteQueryResponse(qr))

	// read queryresponse
	var inqr retrievalmarket.QueryResponse
	select {
	case <-ctx.Done():
		t.Error("msg not received")
	case inqr = <-qchan:
	}

	require.NotNil(t, inqr)
	assert.Equal(t, qr, inqr)
}
