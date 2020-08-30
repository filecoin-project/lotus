package messagepool

import (
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-datastore"
)

func TestRepubMessages(t *testing.T) {
	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest")
	if err != nil {
		t.Fatal(err)
	}

	// the actors
	w1, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	a1, err := w1.GenerateKey(crypto.SigTypeSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	w2, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	a2, err := w2.GenerateKey(crypto.SigTypeSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(m)
		if err != nil {
			t.Fatal(err)
		}
	}

	if tma.published != 10 {
		t.Fatalf("expected to have published 10 messages, but got %d instead", tma.published)
	}

	mp.repubTrigger <- struct{}{}
	time.Sleep(100 * time.Millisecond)

	if tma.published != 20 {
		t.Fatalf("expected to have published 20 messages, but got %d instead", tma.published)
	}
}
