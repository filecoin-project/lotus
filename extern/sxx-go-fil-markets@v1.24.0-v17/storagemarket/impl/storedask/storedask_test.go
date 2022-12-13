package storedask_test

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerutils"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/storedask"
	"github.com/filecoin-project/go-fil-markets/storagemarket/migrations"
	"github.com/filecoin-project/go-fil-markets/storagemarket/testnodes"
)

func TestStoredAsk(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	spn := &testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			SMState: testnodes.NewStorageMarketState(),
		},
	}
	actor := address.TestAddress2
	storedAsk, err := storedask.NewStoredAsk(ds, datastore.NewKey("latest-ask"), spn, actor)
	require.NoError(t, err)

	testPrice := abi.NewTokenAmount(1000000000)
	testVerifiedPrice := abi.NewTokenAmount(100000000)
	testDuration := abi.ChainEpoch(200)
	t.Run("auto initializing", func(t *testing.T) {
		ask := storedAsk.GetAsk()
		require.NotNil(t, ask)
	})
	t.Run("setting ask price", func(t *testing.T) {
		minPieceSize := abi.PaddedPieceSize(1024)
		err := storedAsk.SetAsk(testPrice, testVerifiedPrice, testDuration, storagemarket.MinPieceSize(minPieceSize))
		require.NoError(t, err)
		ask := storedAsk.GetAsk()
		require.Equal(t, ask.Ask.Price, testPrice)
		require.Equal(t, ask.Ask.Expiry-ask.Ask.Timestamp, testDuration)
		require.Equal(t, ask.Ask.MinPieceSize, minPieceSize)
	})
	t.Run("reloading stored ask from disk", func(t *testing.T) {
		storedAsk2, err := storedask.NewStoredAsk(ds, datastore.NewKey("latest-ask"), spn, actor)
		require.NoError(t, err)
		ask := storedAsk2.GetAsk()
		require.Equal(t, ask.Ask.Price, testPrice)
		require.Equal(t, ask.Ask.VerifiedPrice, testVerifiedPrice)
		require.Equal(t, ask.Ask.Expiry-ask.Ask.Timestamp, testDuration)
	})

	t.Run("node errors", func(t *testing.T) {
		spnStateIDErr := &testnodes.FakeProviderNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				GetChainHeadError: errors.New("something went wrong"),
				SMState:           testnodes.NewStorageMarketState(),
			},
		}
		// should load cause ask is is still in data store
		storedAskError, err := storedask.NewStoredAsk(ds, datastore.NewKey("latest-ask"), spnStateIDErr, actor)
		require.NoError(t, err)
		err = storedAskError.SetAsk(testPrice, testVerifiedPrice, testDuration)
		require.Error(t, err)

		spnMinerWorkerErr := &testnodes.FakeProviderNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState: testnodes.NewStorageMarketState(),
			},
			MinerWorkerError: errors.New("something went wrong"),
		}
		// should load cause ask is is still in data store
		storedAskError, err = storedask.NewStoredAsk(ds, datastore.NewKey("latest-ask"), spnMinerWorkerErr, actor)
		require.NoError(t, err)
		err = storedAskError.SetAsk(testPrice, testVerifiedPrice, testDuration)
		require.Error(t, err)

		spnSignBytesErr := &testnodes.FakeProviderNode{
			FakeCommonNode: testnodes.FakeCommonNode{
				SMState:        testnodes.NewStorageMarketState(),
				SignBytesError: errors.New("something went wrong"),
			},
		}
		// should load cause ask is is still in data store
		storedAskError, err = storedask.NewStoredAsk(ds, datastore.NewKey("latest-ask"), spnSignBytesErr, actor)
		require.NoError(t, err)
		err = storedAskError.SetAsk(testPrice, testVerifiedPrice, testDuration)
		require.Error(t, err)
	})
}

func TestPieceSizeLimits(t *testing.T) {
	// create ask with options
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	spn := &testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			SMState: testnodes.NewStorageMarketState(),
		},
	}
	actor := address.TestAddress2
	min := abi.PaddedPieceSize(1024)
	max := abi.PaddedPieceSize(4096)
	sa, err := storedask.NewStoredAsk(ds, datastore.NewKey("latest-ask"), spn, actor, storagemarket.MinPieceSize(min), storagemarket.MaxPieceSize(max))
	require.NoError(t, err)
	ask := sa.GetAsk()
	require.EqualValues(t, min, ask.Ask.MinPieceSize)
	require.EqualValues(t, max, ask.Ask.MaxPieceSize)

	// SetAsk should not clobber previously-set options
	require.NoError(t, sa.SetAsk(ask.Ask.Price, ask.Ask.VerifiedPrice, ask.Ask.Expiry))
	require.NoError(t, err)
	ask = sa.GetAsk()
	require.EqualValues(t, min, ask.Ask.MinPieceSize)
	require.EqualValues(t, max, ask.Ask.MaxPieceSize)

	// now change the size limits via set ask
	testPrice := abi.NewTokenAmount(1000000000)
	testVerifiedPrice := abi.NewTokenAmount(100000000)
	testDuration := abi.ChainEpoch(200)
	newMin := abi.PaddedPieceSize(150)
	newMax := abi.PaddedPieceSize(12345)
	require.NoError(t, sa.SetAsk(testPrice, testVerifiedPrice, testDuration, storagemarket.MinPieceSize(newMin), storagemarket.MaxPieceSize(newMax)))

	// call get
	ask = sa.GetAsk()
	require.EqualValues(t, newMin, ask.Ask.MinPieceSize)
	require.EqualValues(t, newMax, ask.Ask.MaxPieceSize)
}

func TestMigrations(t *testing.T) {
	ctx := context.Background()
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	spn := &testnodes.FakeProviderNode{
		FakeCommonNode: testnodes.FakeCommonNode{
			SMState: testnodes.NewStorageMarketState(),
		},
	}
	actor := address.TestAddress2
	oldAsk := &migrations.StorageAsk0{
		Price:         abi.NewTokenAmount(rand.Int63()),
		VerifiedPrice: abi.NewTokenAmount(rand.Int63()),
		MinPieceSize:  abi.PaddedPieceSize(rand.Uint64()),
		MaxPieceSize:  abi.PaddedPieceSize(rand.Uint64()),
		Miner:         address.TestAddress2,
		Timestamp:     abi.ChainEpoch(rand.Int63()),
		Expiry:        abi.ChainEpoch(rand.Int63()),
		SeqNo:         rand.Uint64(),
	}
	tok, _, err := spn.GetChainHead(ctx)
	require.NoError(t, err)
	sig, err := providerutils.SignMinerData(ctx, oldAsk, actor, tok, spn.GetMinerWorkerAddress, spn.SignBytes)
	require.NoError(t, err)
	oldSignedAsk := &migrations.SignedStorageAsk0{
		Ask:       oldAsk,
		Signature: sig,
	}
	buf := new(bytes.Buffer)
	err = oldSignedAsk.MarshalCBOR(buf)
	require.NoError(t, err)
	err = ds.Put(ctx, datastore.NewKey("latest-ask"), buf.Bytes())
	require.NoError(t, err)
	storedAsk, err := storedask.NewStoredAsk(ds, datastore.NewKey("latest-ask"), spn, actor)
	require.NoError(t, err)
	ask := storedAsk.GetAsk()
	expectedAsk := &storagemarket.StorageAsk{
		Price:         oldAsk.Price,
		VerifiedPrice: oldAsk.VerifiedPrice,
		MinPieceSize:  oldAsk.MinPieceSize,
		MaxPieceSize:  oldAsk.MaxPieceSize,
		Miner:         oldAsk.Miner,
		Timestamp:     oldAsk.Timestamp,
		Expiry:        oldAsk.Expiry,
		SeqNo:         oldAsk.SeqNo,
	}
	require.Equal(t, expectedAsk, ask.Ask)
}
