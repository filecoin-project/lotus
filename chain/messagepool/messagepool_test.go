package messagepool

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
)

func init() {
	_ = logging.SetLogLevel("*", "INFO")
}

type testMpoolAPI struct {
	cb func(rev, app []*types.TipSet) error

	bmsgs      map[cid.Cid][]*types.SignedMessage
	statenonce map[address.Address]uint64
	balance    map[address.Address]types.BigInt

	tipsets []*types.TipSet

	published int

	baseFee types.BigInt
}

func newTestMpoolAPI() *testMpoolAPI {
	tma := &testMpoolAPI{
		bmsgs:      make(map[cid.Cid][]*types.SignedMessage),
		statenonce: make(map[address.Address]uint64),
		balance:    make(map[address.Address]types.BigInt),
		baseFee:    types.NewInt(100),
	}
	genesis := mock.MkBlock(nil, 1, 1)
	tma.tipsets = append(tma.tipsets, mock.TipSet(genesis))
	return tma
}

func (tma *testMpoolAPI) nextBlock() *types.BlockHeader {
	newBlk := mock.MkBlock(tma.tipsets[len(tma.tipsets)-1], 1, 1)
	tma.tipsets = append(tma.tipsets, mock.TipSet(newBlk))
	return newBlk
}

func (tma *testMpoolAPI) nextBlockWithHeight(height uint64) *types.BlockHeader {
	newBlk := mock.MkBlock(tma.tipsets[len(tma.tipsets)-1], 1, 1)
	newBlk.Height = abi.ChainEpoch(height)
	tma.tipsets = append(tma.tipsets, mock.TipSet(newBlk))
	return newBlk
}

func (tma *testMpoolAPI) applyBlock(t *testing.T, b *types.BlockHeader) {
	t.Helper()
	if err := tma.cb(nil, []*types.TipSet{mock.TipSet(b)}); err != nil {
		t.Fatal(err)
	}
}

func (tma *testMpoolAPI) revertBlock(t *testing.T, b *types.BlockHeader) {
	t.Helper()
	if err := tma.cb([]*types.TipSet{mock.TipSet(b)}, nil); err != nil {
		t.Fatal(err)
	}
}

func (tma *testMpoolAPI) setStateNonce(addr address.Address, v uint64) {
	tma.statenonce[addr] = v
}

func (tma *testMpoolAPI) setBalance(addr address.Address, v uint64) {
	tma.balance[addr] = types.FromFil(v)
}

func (tma *testMpoolAPI) setBalanceRaw(addr address.Address, v types.BigInt) {
	tma.balance[addr] = v
}

func (tma *testMpoolAPI) setBlockMessages(h *types.BlockHeader, msgs ...*types.SignedMessage) {
	tma.bmsgs[h.Cid()] = msgs
}

func (tma *testMpoolAPI) SubscribeHeadChanges(cb func(rev, app []*types.TipSet) error) *types.TipSet {
	tma.cb = cb
	return tma.tipsets[0]
}

func (tma *testMpoolAPI) PutMessage(m types.ChainMsg) (cid.Cid, error) {
	return cid.Undef, nil
}

func (tma *testMpoolAPI) PubSubPublish(string, []byte) error {
	tma.published++
	return nil
}

func (tma *testMpoolAPI) GetActorAfter(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	// regression check for load bug
	if ts == nil {
		panic("GetActorAfter called with nil tipset")
	}

	balance, ok := tma.balance[addr]
	if !ok {
		balance = types.NewInt(1000e6)
		tma.balance[addr] = balance
	}

	msgs := make([]*types.SignedMessage, 0)
	for _, b := range ts.Blocks() {
		for _, m := range tma.bmsgs[b.Cid()] {
			if m.Message.From == addr {
				msgs = append(msgs, m)
			}
		}
	}

	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Message.Nonce < msgs[j].Message.Nonce
	})

	nonce := tma.statenonce[addr]

	for _, m := range msgs {
		if m.Message.Nonce != nonce {
			break
		}
		nonce++
	}

	return &types.Actor{
		Code:    builtin.StorageMarketActorCodeID,
		Nonce:   nonce,
		Balance: balance,
	}, nil
}

func (tma *testMpoolAPI) StateAccountKey(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	if addr.Protocol() != address.BLS && addr.Protocol() != address.SECP256K1 {
		return address.Undef, fmt.Errorf("given address was not a key addr")
	}
	return addr, nil
}

func (tma *testMpoolAPI) MessagesForBlock(h *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	return nil, tma.bmsgs[h.Cid()], nil
}

func (tma *testMpoolAPI) MessagesForTipset(ts *types.TipSet) ([]types.ChainMsg, error) {
	if len(ts.Blocks()) != 1 {
		panic("cant deal with multiblock tipsets in this test")
	}

	bm, sm, err := tma.MessagesForBlock(ts.Blocks()[0])
	if err != nil {
		return nil, err
	}

	var out []types.ChainMsg
	for _, m := range bm {
		out = append(out, m)
	}

	for _, m := range sm {
		out = append(out, m)
	}

	return out, nil
}

func (tma *testMpoolAPI) LoadTipSet(tsk types.TipSetKey) (*types.TipSet, error) {
	for _, ts := range tma.tipsets {
		if types.CidArrsEqual(tsk.Cids(), ts.Cids()) {
			return ts, nil
		}
	}

	return nil, fmt.Errorf("tipset not found")
}

func (tma *testMpoolAPI) ChainComputeBaseFee(ctx context.Context, ts *types.TipSet) (types.BigInt, error) {
	return tma.baseFee, nil
}

func assertNonce(t *testing.T, mp *MessagePool, addr address.Address, val uint64) {
	t.Helper()
	n, err := mp.GetNonce(addr)
	if err != nil {
		t.Fatal(err)
	}

	if n != val {
		t.Fatalf("expected nonce of %d, got %d", val, n)
	}
}

func mustAdd(t *testing.T, mp *MessagePool, msg *types.SignedMessage) {
	t.Helper()
	if err := mp.Add(msg); err != nil {
		t.Fatal(err)
	}
}

func TestMessagePool(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	a := tma.nextBlock()

	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}
	target := mock.Address(1001)

	var msgs []*types.SignedMessage
	for i := 0; i < 5; i++ {
		msgs = append(msgs, mock.MkMessage(sender, target, uint64(i), w))
	}

	tma.setStateNonce(sender, 0)
	assertNonce(t, mp, sender, 0)
	mustAdd(t, mp, msgs[0])
	assertNonce(t, mp, sender, 1)
	mustAdd(t, mp, msgs[1])
	assertNonce(t, mp, sender, 2)

	tma.setBlockMessages(a, msgs[0], msgs[1])
	tma.applyBlock(t, a)

	assertNonce(t, mp, sender, 2)
}

func TestMessagePoolMessagesInEachBlock(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	a := tma.nextBlock()

	sender, err := w.WalletNew(context.Background(), types.KTBLS)
	if err != nil {
		t.Fatal(err)
	}
	target := mock.Address(1001)

	var msgs []*types.SignedMessage
	for i := 0; i < 5; i++ {
		m := mock.MkMessage(sender, target, uint64(i), w)
		msgs = append(msgs, m)
		mustAdd(t, mp, m)
	}

	tma.setStateNonce(sender, 0)

	tma.setBlockMessages(a, msgs[0], msgs[1])
	tma.applyBlock(t, a)
	tsa := mock.TipSet(a)

	_, _ = mp.Pending()

	selm, _ := mp.SelectMessages(tsa, 1)
	if len(selm) == 0 {
		t.Fatal("should have returned the rest of the messages")
	}
}

func TestRevertMessages(t *testing.T) {
	futureDebug = true
	defer func() {
		futureDebug = false
	}()

	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	a := tma.nextBlock()
	b := tma.nextBlock()

	sender, err := w.WalletNew(context.Background(), types.KTBLS)
	if err != nil {
		t.Fatal(err)
	}
	target := mock.Address(1001)

	var msgs []*types.SignedMessage
	for i := 0; i < 5; i++ {
		msgs = append(msgs, mock.MkMessage(sender, target, uint64(i), w))
	}

	tma.setBlockMessages(a, msgs[0])
	tma.setBlockMessages(b, msgs[1], msgs[2], msgs[3])

	mustAdd(t, mp, msgs[0])
	mustAdd(t, mp, msgs[1])
	mustAdd(t, mp, msgs[2])
	mustAdd(t, mp, msgs[3])

	tma.setStateNonce(sender, 0)
	tma.applyBlock(t, a)
	assertNonce(t, mp, sender, 4)

	tma.setStateNonce(sender, 1)
	tma.applyBlock(t, b)
	assertNonce(t, mp, sender, 4)
	tma.setStateNonce(sender, 0)
	tma.revertBlock(t, b)

	assertNonce(t, mp, sender, 4)

	p, _ := mp.Pending()
	fmt.Printf("%+v\n", p)
	if len(p) != 3 {
		t.Fatal("expected three messages in mempool")
	}

}

func TestPruningSimple(t *testing.T) {
	oldMaxNonceGap := MaxNonceGap
	MaxNonceGap = 1000
	defer func() {
		MaxNonceGap = oldMaxNonceGap
	}()

	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	a := tma.nextBlock()
	tma.applyBlock(t, a)

	sender, err := w.WalletNew(context.Background(), types.KTBLS)
	if err != nil {
		t.Fatal(err)
	}
	tma.setBalance(sender, 1) // in FIL
	target := mock.Address(1001)

	for i := 0; i < 5; i++ {
		smsg := mock.MkMessage(sender, target, uint64(i), w)
		if err := mp.Add(smsg); err != nil {
			t.Fatal(err)
		}
	}

	for i := 10; i < 50; i++ {
		smsg := mock.MkMessage(sender, target, uint64(i), w)
		if err := mp.Add(smsg); err != nil {
			t.Fatal(err)
		}
	}

	mp.cfg.SizeLimitHigh = 40
	mp.cfg.SizeLimitLow = 10

	mp.Prune()

	msgs, _ := mp.Pending()
	if len(msgs) != 5 {
		t.Fatal("expected only 5 messages in pool, got: ", len(msgs))
	}
}

func TestLoadLocal(t *testing.T) {
	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
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

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL
	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]
	msgs := make(map[cid.Cid]struct{})
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		cid, err := mp.Push(m)
		if err != nil {
			t.Fatal(err)
		}
		msgs[cid] = struct{}{}
	}
	err = mp.Close()
	if err != nil {
		t.Fatal(err)
	}

	mp, err = New(tma, ds, "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	pmsgs, _ := mp.Pending()
	if len(msgs) != len(pmsgs) {
		t.Fatalf("expected %d messages, but got %d", len(msgs), len(pmsgs))
	}

	for _, m := range pmsgs {
		cid := m.Cid()
		_, ok := msgs[cid]
		if !ok {
			t.Fatal("unknown message")
		}

		delete(msgs, cid)
	}

	if len(msgs) > 0 {
		t.Fatalf("not all messages were laoded; missing %d messages", len(msgs))
	}
}

func TestClearAll(t *testing.T) {
	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
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

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL
	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(m)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(i+1))
		mustAdd(t, mp, m)
	}

	mp.Clear(true)

	pending, _ := mp.Pending()
	if len(pending) > 0 {
		t.Fatalf("cleared the mpool, but got %d pending messages", len(pending))
	}
}

func TestClearNonLocal(t *testing.T) {
	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
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

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(m)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(i+1))
		mustAdd(t, mp, m)
	}

	mp.Clear(false)

	pending, _ := mp.Pending()
	if len(pending) != 10 {
		t.Fatalf("expected 10 pending messages, but got %d instead", len(pending))
	}

	for _, m := range pending {
		if m.Message.From != a1 {
			t.Fatalf("expected message from %s but got one from %s instead", a1, m.Message.From)
		}
	}
}

func TestUpdates(t *testing.T) {
	tma := newTestMpoolAPI()
	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest", nil)
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

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	ch, err := mp.Updates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(m)
		if err != nil {
			t.Fatal(err)
		}

		_, ok := <-ch
		if !ok {
			t.Fatal("expected update, but got a closed channel instead")
		}
	}

	err = mp.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel, but got an update instead")
	}
}
