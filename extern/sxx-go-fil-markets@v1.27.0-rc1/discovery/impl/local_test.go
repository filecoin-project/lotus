package discoveryimpl_test

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestLocal_AddPeer(t *testing.T) {
	ctx := context.Background()
	peer1 := retrievalmarket.RetrievalPeer{
		Address:  shared_testutil.NewIDAddr(t, 1),
		ID:       peer.NewPeerRecord().PeerID,
		PieceCID: nil,
	}
	pieceCid := shared_testutil.GenerateCids(1)[0]
	peer2 := retrievalmarket.RetrievalPeer{
		Address:  shared_testutil.NewIDAddr(t, 2),
		ID:       peer.NewPeerRecord().PeerID,
		PieceCID: &pieceCid,
	}
	testCases := []struct {
		name      string
		peers2add []retrievalmarket.RetrievalPeer
		expPeers  []retrievalmarket.RetrievalPeer
	}{
		{
			name:      "can add 3 peers",
			peers2add: []retrievalmarket.RetrievalPeer{peer1, peer2},
			expPeers:  []retrievalmarket.RetrievalPeer{peer1, peer2},
		},
		{
			name:      "can add same peer without duping",
			peers2add: []retrievalmarket.RetrievalPeer{peer1, peer1},
			expPeers:  []retrievalmarket.RetrievalPeer{peer1},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			ds := datastore.NewMapDatastore()
			l, err := discoveryimpl.NewLocal(ds)
			require.NoError(t, err)
			shared_testutil.StartAndWaitForReady(ctx, t, l)

			payloadCID := shared_testutil.GenerateCids(1)[0]
			for _, testpeer := range tc.peers2add {
				require.NoError(t, l.AddPeer(ctx, payloadCID, testpeer))
			}
			actualPeers, err := l.GetPeers(payloadCID)
			require.NoError(t, err)
			assert.Equal(t, len(tc.expPeers), len(actualPeers))
			assert.Equal(t, tc.expPeers[0], actualPeers[0])
		})
	}
}
