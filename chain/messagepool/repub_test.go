package messagepool

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

func TestRepubMessages(t *testing.T) {
	oldRepublishBatchDelay := RepublishBatchDelay
	RepublishBatchDelay = time.Microsecond
	defer func() {
		RepublishBatchDelay = oldRepublishBatchDelay
	}()

	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	// the actors
	w1, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	a1, err := w1.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	w2, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	a2, err := w2.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin2.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(context.TODO(), m, true)
		if err != nil {
			t.Fatal(err)
		}
	}

	require.Eventually(t, func() bool {
		return tma.publishedCount() == 10
	}, time.Second, time.Millisecond, "expected to have published 10 messages, but got %d instead", tma.publishedCount())

	mp.repubTrigger <- struct{}{}

	require.Eventually(t, func() bool {
		return tma.publishedCount() == 20
	}, time.Second, time.Millisecond, "expected to have published 20 messages, but got %d instead", tma.publishedCount())
}

func TestPushDoesNotWaitForPubSubPublish(t *testing.T) {
	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)
	defer func() {
		_ = mp.Close()
	}()

	w1, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)

	a1, err := w1.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)

	w2, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)

	a2, err := w2.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)

	tma.setBalance(a1, 1)

	publishStarted := make(chan struct{})
	releasePublish := make(chan struct{})
	defer close(releasePublish)
	var closeStarted sync.Once

	tma.publishFn = func(string, []byte) error {
		tma.published.Add(1)
		closeStarted.Do(func() {
			close(publishStarted)
		})
		<-releasePublish
		return nil
	}

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin2.StorageMarketActorCodeID, M: 2}]
	m := makeTestMessage(w1, a1, a2, 0, gasLimit, 1)

	pushReturned := make(chan error, 1)
	go func() {
		_, err := mp.Push(context.TODO(), m, true)
		pushReturned <- err
	}()

	select {
	case err := <-pushReturned:
		require.NoError(t, err)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Push blocked on pubsub publish")
	}

	select {
	case <-publishStarted:
	case <-time.After(time.Second):
		t.Fatal("async pubsub publish did not start")
	}
}
