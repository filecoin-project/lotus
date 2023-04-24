package askstore_test

import (
	"testing"

	"github.com/ipfs/go-datastore"
	dss "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/askstore"
)

func TestAskStoreImpl(t *testing.T) {
	ds := dss.MutexWrap(datastore.NewMapDatastore())
	store, err := askstore.NewAskStore(ds, datastore.NewKey("retrieval-ask"))
	require.NoError(t, err)

	// A new store returns the default ask
	ask := store.GetAsk()
	require.NotNil(t, ask)

	require.Equal(t, retrievalmarket.DefaultUnsealPrice, ask.UnsealPrice)
	require.Equal(t, retrievalmarket.DefaultPricePerByte, ask.PricePerByte)
	require.Equal(t, retrievalmarket.DefaultPaymentInterval, ask.PaymentInterval)
	require.Equal(t, retrievalmarket.DefaultPaymentIntervalIncrease, ask.PaymentIntervalIncrease)

	// Store a new ask
	newAsk := &retrievalmarket.Ask{
		PricePerByte:            abi.NewTokenAmount(123),
		UnsealPrice:             abi.NewTokenAmount(456),
		PaymentInterval:         789,
		PaymentIntervalIncrease: 789,
	}
	err = store.SetAsk(newAsk)
	require.NoError(t, err)

	// Fetch new ask
	stored := store.GetAsk()
	require.Equal(t, newAsk, stored)

	// Construct a new AskStore and make sure it returns the previously-stored ask
	newStore, err := askstore.NewAskStore(ds, datastore.NewKey("retrieval-ask"))
	require.NoError(t, err)
	stored = newStore.GetAsk()
	require.Equal(t, newAsk, stored)
}
