package messagepool

import (
	"context"
	"math"
	"math/big"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"

	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	logging "github.com/ipfs/go-log"
)

func makeTestMessage(w *wallet.Wallet, from, to address.Address, nonce uint64, gasLimit int64, gasPrice uint64) *types.SignedMessage {
	msg := &types.Message{
		From:       from,
		To:         to,
		Method:     2,
		Value:      types.FromFil(0),
		Nonce:      nonce,
		GasLimit:   gasLimit,
		GasFeeCap:  types.NewInt(100 + gasPrice),
		GasPremium: types.NewInt(gasPrice),
	}
	sig, err := w.Sign(context.TODO(), from, msg.Cid().Bytes())
	if err != nil {
		panic(err)
	}
	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}
}

func makeTestMpool() (*MessagePool, *testMpoolAPI) {
	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()
	mp, err := New(tma, ds, "test")
	if err != nil {
		panic(err)
	}

	return mp, tma
}

func TestMessageChains(t *testing.T) {
	mp, tma := makeTestMpool()

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

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL

	// test chain aggregations

	// test1: 10 messages from a1 to a2, with increasing gasPerf; it should
	//        make a single chain with 10 messages given enough balance
	mset := make(map[uint64]*types.SignedMessage)
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		mset[uint64(i)] = m
	}
	baseFee := types.NewInt(0)

	chains := mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 1 {
		t.Fatal("expected a single chain")
	}
	if len(chains[0].msgs) != 10 {
		t.Fatalf("expected 10 messages in the chain but got %d", len(chains[0].msgs))
	}
	for i, m := range chains[0].msgs {
		if m.Message.Nonce != uint64(i) {
			t.Fatalf("expected nonce %d but got %d", i, m.Message.Nonce)
		}
	}

	// test2 : 10 messages from a1 to a2, with decreasing gasPerf; it should
	//         make 10 chains with 1 message each
	mset = make(map[uint64]*types.SignedMessage)
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(10-i))
		mset[uint64(i)] = m
	}

	chains = mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 10 {
		t.Fatal("expected 10 chains")
	}
	for i, chain := range chains {
		if len(chain.msgs) != 1 {
			t.Fatalf("expected 1 message in chain %d but got %d", i, len(chain.msgs))
		}
	}
	for i, chain := range chains {
		m := chain.msgs[0]
		if m.Message.Nonce != uint64(i) {
			t.Fatalf("expected nonce %d but got %d", i, m.Message.Nonce)
		}
	}

	// test3a: 10 messages from a1 to a2, with gasPerf increasing in groups of 3; it should
	//         merge them in two chains, one with 9 messages and one with the last message
	mset = make(map[uint64]*types.SignedMessage)
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(1+i%3))
		mset[uint64(i)] = m
	}

	chains = mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 2 {
		t.Fatal("expected 1 chain")
	}

	if len(chains[0].msgs) != 9 {
		t.Fatalf("expected 9 messages in the chain but got %d", len(chains[0].msgs))
	}
	if len(chains[1].msgs) != 1 {
		t.Fatalf("expected 1 messages in the chain but got %d", len(chains[1].msgs))
	}
	nextNonce := 0
	for _, chain := range chains {
		for _, m := range chain.msgs {
			if m.Message.Nonce != uint64(nextNonce) {
				t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
			}
			nextNonce++
		}
	}

	// test3b: 10 messages from a1 to a2, with gasPerf decreasing in groups of 3 with a bias for the
	//        earlier chains; it should make 4 chains, the first 3 with 3 messages and the last with
	//        a single message
	mset = make(map[uint64]*types.SignedMessage)
	for i := 0; i < 10; i++ {
		bias := (12 - i) / 3
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(1+i%3+bias))
		mset[uint64(i)] = m
	}

	chains = mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 4 {
		t.Fatal("expected 4 chains")
	}
	for i, chain := range chains {
		expectedLen := 3
		if i > 2 {
			expectedLen = 1
		}
		if len(chain.msgs) != expectedLen {
			t.Fatalf("expected %d message in chain %d but got %d", expectedLen, i, len(chain.msgs))
		}
	}
	nextNonce = 0
	for _, chain := range chains {
		for _, m := range chain.msgs {
			if m.Message.Nonce != uint64(nextNonce) {
				t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
			}
			nextNonce++
		}
	}

	// test chain breaks

	// test4: 10 messages with non-consecutive nonces; it should make a single chain with just
	//        the first message
	mset = make(map[uint64]*types.SignedMessage)
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i*2), gasLimit, uint64(i+1))
		mset[uint64(i)] = m
	}

	chains = mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 1 {
		t.Fatal("expected a single chain")
	}
	if len(chains[0].msgs) != 1 {
		t.Fatalf("expected 1 message in the chain but got %d", len(chains[0].msgs))
	}
	for i, m := range chains[0].msgs {
		if m.Message.Nonce != uint64(i) {
			t.Fatalf("expected nonce %d but got %d", i, m.Message.Nonce)
		}
	}

	// test5: 10 messages with increasing gasLimit, except for the 6th message which has less than
	//        the epoch gasLimit; it should create a single chain with the first 5 messages
	mset = make(map[uint64]*types.SignedMessage)
	for i := 0; i < 10; i++ {
		var m *types.SignedMessage
		if i != 5 {
			m = makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		} else {
			m = makeTestMessage(w1, a1, a2, uint64(i), 1, uint64(i+1))
		}
		mset[uint64(i)] = m
	}

	chains = mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 1 {
		t.Fatal("expected a single chain")
	}
	if len(chains[0].msgs) != 5 {
		t.Fatalf("expected 5 message in the chain but got %d", len(chains[0].msgs))
	}
	for i, m := range chains[0].msgs {
		if m.Message.Nonce != uint64(i) {
			t.Fatalf("expected nonce %d but got %d", i, m.Message.Nonce)
		}
	}

	// test6: one more message than what can fit in a block according to gas limit, with increasing
	//        gasPerf; it should create a single chain with the max messages
	maxMessages := int(build.BlockGasLimit / gasLimit)
	nMessages := maxMessages + 1

	mset = make(map[uint64]*types.SignedMessage)
	for i := 0; i < nMessages; i++ {
		mset[uint64(i)] = makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
	}

	chains = mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 1 {
		t.Fatal("expected a single chain")
	}
	if len(chains[0].msgs) != maxMessages {
		t.Fatalf("expected %d message in the chain but got %d", maxMessages, len(chains[0].msgs))
	}
	for i, m := range chains[0].msgs {
		if m.Message.Nonce != uint64(i) {
			t.Fatalf("expected nonce %d but got %d", i, m.Message.Nonce)
		}
	}

	// test5: insufficient balance for all messages
	tma.setBalanceRaw(a1, types.NewInt(uint64((300)*gasLimit+1)))

	mset = make(map[uint64]*types.SignedMessage)
	for i := 0; i < 10; i++ {
		mset[uint64(i)] = makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
	}

	chains = mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 1 {
		t.Fatalf("expected a single chain: got %d", len(chains))
	}
	if len(chains[0].msgs) != 2 {
		t.Fatalf("expected %d message in the chain but got %d", 2, len(chains[0].msgs))
	}
	for i, m := range chains[0].msgs {
		if m.Message.Nonce != uint64(i) {
			t.Fatalf("expected nonce %d but got %d", i, m.Message.Nonce)
		}
	}

}

func TestMessageChainSkipping(t *testing.T) {
	// regression test for chain skip bug

	mp, tma := makeTestMpool()

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

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]
	baseFee := types.NewInt(0)

	tma.setBalance(a1, 1) // in FIL
	tma.setStateNonce(a1, 10)

	mset := make(map[uint64]*types.SignedMessage)
	for i := 0; i < 20; i++ {
		bias := (20 - i) / 3
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(1+i%3+bias))
		mset[uint64(i)] = m
	}

	chains := mp.createMessageChains(a1, mset, baseFee, ts)
	if len(chains) != 4 {
		t.Fatalf("expected 4 chains, got %d", len(chains))
	}
	for i, chain := range chains {
		var expectedLen int
		switch {
		case i == 0:
			expectedLen = 2
		case i > 2:
			expectedLen = 2
		default:
			expectedLen = 3
		}
		if len(chain.msgs) != expectedLen {
			t.Fatalf("expected %d message in chain %d but got %d", expectedLen, i, len(chain.msgs))
		}
	}
	nextNonce := 10
	for _, chain := range chains {
		for _, m := range chain.msgs {
			if m.Message.Nonce != uint64(nextNonce) {
				t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
			}
			nextNonce++
		}
	}

}

func TestBasicMessageSelection(t *testing.T) {
	mp, tma := makeTestMpool()

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

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)
	tma.applyBlock(t, block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	// we create 10 messages from each actor to another, with the first actor paying higher
	// gas prices than the second; we expect message selection to order his messages first
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(2*i+1))
		mustAdd(t, mp, m)
	}

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(i+1))
		mustAdd(t, mp, m)
	}

	msgs, err := mp.SelectMessages(ts, 1.0)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 20 {
		t.Fatalf("exptected 20 messages, got %d", len(msgs))
	}

	nextNonce := 0
	for i := 0; i < 10; i++ {
		if msgs[i].Message.From != a1 {
			t.Fatalf("expected message from actor a1")
		}
		if msgs[i].Message.Nonce != uint64(nextNonce) {
			t.Fatalf("expected nonce %d, got %d", msgs[i].Message.Nonce, nextNonce)
		}
		nextNonce++
	}

	nextNonce = 0
	for i := 10; i < 20; i++ {
		if msgs[i].Message.From != a2 {
			t.Fatalf("expected message from actor a2")
		}
		if msgs[i].Message.Nonce != uint64(nextNonce) {
			t.Fatalf("expected nonce %d, got %d", msgs[i].Message.Nonce, nextNonce)
		}
		nextNonce++
	}

	// now we make a block with all the messages and advance the chain
	block2 := mock.MkBlock(ts, 2, 2)
	tma.setBlockMessages(block2, msgs...)
	tma.applyBlock(t, block2)

	// we should have no pending messages in the mpool
	pend, ts2 := mp.Pending()
	if len(pend) != 0 {
		t.Fatalf("expected no pending messages, but got %d", len(pend))
	}

	// create a block and advance the chain without applying to the mpool
	msgs = nil
	for i := 10; i < 20; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(2*i+1))
		msgs = append(msgs, m)
		m = makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(i+1))
		msgs = append(msgs, m)
	}
	block3 := mock.MkBlock(ts2, 3, 3)
	tma.setBlockMessages(block3, msgs...)
	ts3 := mock.TipSet(block3)

	// now create another set of messages and add them to the mpool
	for i := 20; i < 30; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(2*i+1))
		mustAdd(t, mp, m)
		m = makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(i+1))
		mustAdd(t, mp, m)
	}

	// select messages in the last tipset; this should include the missed messages as well as
	// the last messages we added, with the first actor's messages first
	// first we need to update the nonce on the tma
	tma.setStateNonce(a1, 10)
	tma.setStateNonce(a2, 10)

	msgs, err = mp.SelectMessages(ts3, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 40 {
		t.Fatalf("expected 40 messages, got %d", len(msgs))
	}

	nextNonce = 10
	for i := 0; i < 20; i++ {
		if msgs[i].Message.From != a1 {
			t.Fatalf("expected message from actor a1")
		}
		if msgs[i].Message.Nonce != uint64(nextNonce) {
			t.Fatalf("expected nonce %d, got %d", msgs[i].Message.Nonce, nextNonce)
		}
		nextNonce++
	}

	nextNonce = 10
	for i := 20; i < 40; i++ {
		if msgs[i].Message.From != a2 {
			t.Fatalf("expected message from actor a2")
		}
		if msgs[i].Message.Nonce != uint64(nextNonce) {
			t.Fatalf("expected nonce %d, got %d", msgs[i].Message.Nonce, nextNonce)
		}
		nextNonce++
	}
}

func TestMessageSelectionTrimming(t *testing.T) {
	mp, tma := makeTestMpool()

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

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)
	tma.applyBlock(t, block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	// make many small chains for the two actors
	nMessages := int((build.BlockGasLimit / gasLimit) + 1)
	for i := 0; i < nMessages; i++ {
		bias := (nMessages - i) / 3
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(1+i%3+bias))
		mustAdd(t, mp, m)
		m = makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(1+i%3+bias))
		mustAdd(t, mp, m)
	}

	msgs, err := mp.SelectMessages(ts, 1.0)
	if err != nil {
		t.Fatal(err)
	}

	expected := int(build.BlockGasLimit / gasLimit)
	if len(msgs) != expected {
		t.Fatalf("expected %d messages, bug got %d", expected, len(msgs))
	}

	mGasLimit := int64(0)
	for _, m := range msgs {
		mGasLimit += m.Message.GasLimit
	}
	if mGasLimit > build.BlockGasLimit {
		t.Fatal("selected messages gas limit exceeds block gas limit!")
	}

}

func TestPriorityMessageSelection(t *testing.T) {
	mp, tma := makeTestMpool()

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

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)
	tma.applyBlock(t, block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	mp.cfg.PriorityAddrs = []address.Address{a1}

	nMessages := 10
	for i := 0; i < nMessages; i++ {
		bias := (nMessages - i) / 3
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(1+i%3+bias))
		mustAdd(t, mp, m)
		m = makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(1+i%3+bias))
		mustAdd(t, mp, m)
	}

	msgs, err := mp.SelectMessages(ts, 1.0)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 20 {
		t.Fatalf("expected 20 messages but got %d", len(msgs))
	}

	// messages from a1 must be first
	nextNonce := uint64(0)
	for i := 0; i < 10; i++ {
		m := msgs[i]
		if m.Message.From != a1 {
			t.Fatal("expected messages from a1 before messages from a2")
		}
		if m.Message.Nonce != nextNonce {
			t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
		}
		nextNonce++
	}

	nextNonce = 0
	for i := 10; i < 20; i++ {
		m := msgs[i]
		if m.Message.From != a2 {
			t.Fatal("expected messages from a2 after messages from a1")
		}
		if m.Message.Nonce != nextNonce {
			t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
		}
		nextNonce++
	}
}

func TestOptimalMessageSelection1(t *testing.T) {
	// this test uses just a single actor sending messages with a low tq
	// the chain depenent merging algorithm should pick messages from the actor
	// from the start
	mp, tma := makeTestMpool()

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

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)
	tma.applyBlock(t, block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	nMessages := int(10 * build.BlockGasLimit / gasLimit)
	for i := 0; i < nMessages; i++ {
		bias := (nMessages - i) / 3
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(1+i%3+bias))
		mustAdd(t, mp, m)
	}

	msgs, err := mp.SelectMessages(ts, 0.25)
	if err != nil {
		t.Fatal(err)
	}

	expectedMsgs := int(build.BlockGasLimit / gasLimit)
	if len(msgs) != expectedMsgs {
		t.Fatalf("expected %d messages, but got %d", expectedMsgs, len(msgs))
	}

	nextNonce := uint64(0)
	for _, m := range msgs {
		if m.Message.From != a1 {
			t.Fatal("expected message from a1")
		}

		if m.Message.Nonce != nextNonce {
			t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
		}
		nextNonce++
	}
}

func TestOptimalMessageSelection2(t *testing.T) {
	// this test uses two actors sending messages to each other, with the first
	// actor paying (much) higher gas premium than the second.
	// We select with a low ticket quality; the chain depenent merging algorithm should pick
	// messages from the second actor from the start
	mp, tma := makeTestMpool()

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

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)
	tma.applyBlock(t, block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	nMessages := int(5 * build.BlockGasLimit / gasLimit)
	for i := 0; i < nMessages; i++ {
		bias := (nMessages - i) / 3
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(10000+i%3+bias))
		mustAdd(t, mp, m)
		m = makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(1+i%3+bias))
		mustAdd(t, mp, m)
	}

	msgs, err := mp.SelectMessages(ts, 0.1)
	if err != nil {
		t.Fatal(err)
	}

	expectedMsgs := int(build.BlockGasLimit / gasLimit)
	if len(msgs) != expectedMsgs {
		t.Fatalf("expected %d messages, but got %d", expectedMsgs, len(msgs))
	}

	nextNonce := uint64(0)
	for _, m := range msgs {
		if m.Message.From != a2 {
			t.Fatal("expected message from a2")
		}

		if m.Message.Nonce != nextNonce {
			t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
		}
		nextNonce++
	}
}

func TestOptimalMessageSelection3(t *testing.T) {
	// this test uses 10 actors sending a block of messages to each other, with the the first
	// actors paying higher gas premium than the subsequent actors.
	// We select with a low ticket quality; the chain depenent merging algorithm should pick
	// messages from the median actor from the start
	mp, tma := makeTestMpool()

	nActors := 10
	// the actors
	var actors []address.Address
	var wallets []*wallet.Wallet

	for i := 0; i < nActors; i++ {
		w, err := wallet.NewWallet(wallet.NewMemKeyStore())
		if err != nil {
			t.Fatal(err)
		}

		a, err := w.GenerateKey(crypto.SigTypeSecp256k1)
		if err != nil {
			t.Fatal(err)
		}

		actors = append(actors, a)
		wallets = append(wallets, w)
	}

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)
	tma.applyBlock(t, block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	for _, a := range actors {
		tma.setBalance(a, 1) // in FIL
	}

	nMessages := int(build.BlockGasLimit/gasLimit) + 1
	for i := 0; i < nMessages; i++ {
		for j := 0; j < nActors; j++ {
			bias := (nActors-j)*nMessages + (nMessages+2-i)/(3*nActors) + i%3
			m := makeTestMessage(wallets[j], actors[j], actors[j%nActors], uint64(i), gasLimit, uint64(1+bias))
			mustAdd(t, mp, m)
		}
	}

	msgs, err := mp.SelectMessages(ts, 0.1)
	if err != nil {
		t.Fatal(err)
	}

	expectedMsgs := int(build.BlockGasLimit / gasLimit)
	if len(msgs) != expectedMsgs {
		t.Fatalf("expected %d messages, but got %d", expectedMsgs, len(msgs))
	}

	nextNonce := uint64(0)
	a := actors[len(actors)/2-1]
	for _, m := range msgs {
		if m.Message.From != a {
			who := 0
			for i, a := range actors {
				if a == m.Message.From {
					who = i
					break
				}
			}
			t.Fatalf("expected message from last actor, but got from %d instead", who)
		}

		if m.Message.Nonce != nextNonce {
			t.Fatalf("expected nonce %d but got %d", nextNonce, m.Message.Nonce)
		}
		nextNonce++
	}
}

func testCompetitiveMessageSelection(t *testing.T, rng *rand.Rand) {
	// in this test we use 100 actors and send 10 blocks of messages.
	// actors send with an exponentially decreasing premium.
	// a number of miners select with varying ticket quality and we compare the
	// capacity and rewards of greedy selection -vs- optimal selection

	mp, tma := makeTestMpool()

	nActors := 300
	// the actors
	var actors []address.Address
	var wallets []*wallet.Wallet

	for i := 0; i < nActors; i++ {
		w, err := wallet.NewWallet(wallet.NewMemKeyStore())
		if err != nil {
			t.Fatal(err)
		}

		a, err := w.GenerateKey(crypto.SigTypeSecp256k1)
		if err != nil {
			t.Fatal(err)
		}

		actors = append(actors, a)
		wallets = append(wallets, w)
	}

	block := mock.MkBlock(nil, 1, 1)
	ts := mock.TipSet(block)
	tma.applyBlock(t, block)

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]
	baseFee := types.NewInt(0)

	for _, a := range actors {
		tma.setBalance(a, 1) // in FIL
	}

	nMessages := 10 * int(build.BlockGasLimit/gasLimit)
	t.Log("nMessages", nMessages)
	nonces := make([]uint64, nActors)
	for i := 0; i < nMessages; i++ {
		from := rng.Intn(nActors)
		to := rng.Intn(nActors)
		premium := 20000*math.Exp(-3.*rand.Float64()) + 5000
		nonce := nonces[from]
		nonces[from]++
		m := makeTestMessage(wallets[from], actors[from], actors[to], uint64(nonce), gasLimit, uint64(premium))
		mustAdd(t, mp, m)
	}

	// 1. greedy selection
	greedyMsgs, err := mp.selectMessagesGreedy(ts, ts)
	if err != nil {
		t.Fatal(err)
	}

	// 2. optimal selection
	minersRand := rng.Float64()
	winerProba := noWinnersProb()
	i := 0
	for ; i < MaxBlocks && minersRand > 0; i++ {
		minersRand -= winerProba[i]
	}
	nMiners := i
	if nMiners == 0 {
		nMiners = 1
	}

	logging.SetLogLevel("messagepool", "error")
	for i := 0; i < 1; i++ {
		optMsgs := make(map[cid.Cid]*types.SignedMessage)
		for j := 0; j < nMiners; j++ {
			tq := rng.Float64()
			msgs, err := mp.SelectMessages(ts, tq)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range msgs {
				optMsgs[m.Cid()] = m
			}
		}

		t.Logf("nMiners: %d", nMiners)
		t.Logf("greedy capacity %d, optimal capacity %d (x%.1f)", len(greedyMsgs),
			len(optMsgs), float64(len(optMsgs))/float64(len(greedyMsgs)))
		if len(greedyMsgs) > len(optMsgs) {
			t.Fatal("greedy capacity higher than optimal capacity; wtf")
		}

		greedyReward := big.NewInt(0)
		for _, m := range greedyMsgs {
			greedyReward.Add(greedyReward, mp.getGasReward(m, baseFee, ts))
		}

		optReward := big.NewInt(0)
		for _, m := range optMsgs {
			optReward.Add(optReward, mp.getGasReward(m, baseFee, ts))
		}

		nMinersBig := big.NewInt(int64(nMiners))
		greedyAvgReward, _ := new(big.Rat).SetFrac(greedyReward, nMinersBig).Float64()
		optimalAvgReward, _ := new(big.Rat).SetFrac(optReward, nMinersBig).Float64()
		t.Logf("greedy reward: %.0f, optimal reward: %.0f (x%.1f)", greedyAvgReward,
			optimalAvgReward, optimalAvgReward/greedyAvgReward)

		if greedyReward.Cmp(optReward) > 0 {
			t.Fatal("greedy reward raw higher than optimal reward; booh")
		}
	}
	logging.SetLogLevel("messagepool", "info")
}

func TestCompetitiveMessageSelection(t *testing.T) {
	seeds := []int64{1947, 1976, 2020, 2100, 10000}
	for _, seed := range seeds {
		t.Log("running competitve message selection with seed", seed)
		testCompetitiveMessageSelection(t, rand.New(rand.NewSource(seed)))
	}
}
