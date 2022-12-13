package connmanager_test

import (
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/connmanager"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
)

func TestConnManager(t *testing.T) {
	conns := connmanager.NewConnManager()
	cids := shared_testutil.GenerateCids(10)
	streams := make([]network.StorageDealStream, 0, 10)
	var wait sync.WaitGroup

	for i := 0; i < 10; i++ {
		streams = append(streams, shared_testutil.NewTestStorageDealStream(
			shared_testutil.TestStorageDealStreamParams{}))
	}
	t.Run("no conns present initially", func(t *testing.T) {
		for _, c := range cids {
			stream, err := conns.DealStream(c)
			require.Nil(t, stream)
			require.Error(t, err)
		}
	})

	t.Run("adding conns, can retrieve", func(t *testing.T) {
		for i, c := range cids {
			wait.Add(1)
			stream := streams[i]
			go func(c cid.Cid, stream network.StorageDealStream) {
				defer wait.Done()
				err := conns.AddStream(c, stream)
				require.NoError(t, err)
			}(c, stream)
		}
		wait.Wait()
		for i, c := range cids {
			wait.Add(1)
			stream := streams[i]
			go func(c cid.Cid, stream network.StorageDealStream) {
				defer wait.Done()
				received, err := conns.DealStream(c)
				require.Equal(t, stream, received)
				require.NoError(t, err)
			}(c, stream)
		}
		wait.Wait()
	})

	t.Run("adding conns twice fails", func(t *testing.T) {
		for i, c := range cids {
			wait.Add(1)
			stream := streams[i]
			go func(c cid.Cid, stream network.StorageDealStream) {
				defer wait.Done()
				err := conns.AddStream(c, stream)
				require.Error(t, err)
			}(c, stream)
		}
		wait.Wait()
	})

	t.Run("disconnection removes", func(t *testing.T) {
		for _, c := range cids {
			wait.Add(1)
			go func(c cid.Cid) {
				defer wait.Done()
				err := conns.Disconnect(c)
				require.NoError(t, err)
			}(c)
		}
		wait.Wait()
		for _, c := range cids {
			wait.Add(1)
			go func(c cid.Cid) {
				defer wait.Done()
				received, err := conns.DealStream(c)
				require.Nil(t, received)
				require.Error(t, err)
			}(c)
		}
		wait.Wait()
	})

	t.Run("disconnecting twice causes no error", func(t *testing.T) {
		for _, c := range cids {
			wait.Add(1)
			go func(c cid.Cid) {
				defer wait.Done()
				err := conns.Disconnect(c)
				require.NoError(t, err)
			}(c)
		}
		wait.Wait()
	})
}
