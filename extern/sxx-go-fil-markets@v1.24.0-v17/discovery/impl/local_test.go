package discoveryimpl_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"

	discoveryimpl "github.com/filecoin-project/go-fil-markets/discovery/impl"
	"github.com/filecoin-project/go-fil-markets/discovery/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	retrievalmigrations "github.com/filecoin-project/go-fil-markets/retrievalmarket/migrations"
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

func TestLocalMigrations(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ds := datastore.NewMapDatastore()

	peers := shared_testutil.GeneratePeers(4)
	pieceCIDs := shared_testutil.GenerateCids(4)
	payloadCids := shared_testutil.GenerateCids(2)
	for i, c := range payloadCids {
		rps := migrations.RetrievalPeers0{
			Peers: []retrievalmigrations.RetrievalPeer0{
				{
					Address:  address.TestAddress,
					ID:       peers[i*2],
					PieceCID: &pieceCIDs[i*2],
				},
				{
					Address:  address.TestAddress2,
					ID:       peers[i*2+1],
					PieceCID: &pieceCIDs[i*2+1],
				},
			},
		}
		buf := new(bytes.Buffer)
		err := rps.MarshalCBOR(buf)
		require.NoError(t, err)
		err = ds.Put(ctx, dshelp.MultihashToDsKey(c.Hash()), buf.Bytes())
		require.NoError(t, err)
	}

	l, err := discoveryimpl.NewLocal(ds)
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, l)

	for i, c := range payloadCids {
		expectedPeers := []retrievalmarket.RetrievalPeer{
			{
				Address:  address.TestAddress,
				ID:       peers[i*2],
				PieceCID: &pieceCIDs[i*2],
			},
			{
				Address:  address.TestAddress2,
				ID:       peers[i*2+1],
				PieceCID: &pieceCIDs[i*2+1],
			},
		}
		peers, err := l.GetPeers(c)
		require.NoError(t, err)
		require.Equal(t, expectedPeers, peers)
	}
}
