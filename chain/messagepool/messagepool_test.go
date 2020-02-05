package messagepool

import (
	"fmt"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
)

type testMpoolApi struct {
	cb func(rev, app []*types.TipSet) error

	bmsgs      map[cid.Cid][]*types.SignedMessage
	statenonce map[address.Address]uint64

	tipsets []*types.TipSet
}

func newTestMpoolApi() *testMpoolApi {
	return &testMpoolApi{
		bmsgs:      make(map[cid.Cid][]*types.SignedMessage),
		statenonce: make(map[address.Address]uint64),
	}
}

func (tma *testMpoolApi) applyBlock(t *testing.T, b *types.BlockHeader) {
	t.Helper()
	if err := tma.cb(nil, []*types.TipSet{mock.TipSet(b)}); err != nil {
		t.Fatal(err)
	}
}

func (tma *testMpoolApi) revertBlock(t *testing.T, b *types.BlockHeader) {
	t.Helper()
	if err := tma.cb([]*types.TipSet{mock.TipSet(b)}, nil); err != nil {
		t.Fatal(err)
	}
}

func (tma *testMpoolApi) setStateNonce(addr address.Address, v uint64) {
	tma.statenonce[addr] = v
}

func (tma *testMpoolApi) setBlockMessages(h *types.BlockHeader, msgs ...*types.SignedMessage) {
	tma.bmsgs[h.Cid()] = msgs
	tma.tipsets = append(tma.tipsets, mock.TipSet(h))
}

func (tma *testMpoolApi) SubscribeHeadChanges(cb func(rev, app []*types.TipSet) error) *types.TipSet {
	tma.cb = cb
	return nil
}

func (tma *testMpoolApi) PutMessage(m store.ChainMsg) (cid.Cid, error) {
	return cid.Undef, nil
}

func (tma *testMpoolApi) PubSubPublish(string, []byte) error {
	return nil
}

func (tma *testMpoolApi) StateGetActor(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	return &types.Actor{
		Nonce:   tma.statenonce[addr],
		Balance: types.NewInt(90000000),
	}, nil
}

func (tma *testMpoolApi) MessagesForBlock(h *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	return nil, tma.bmsgs[h.Cid()], nil
}

func (tma *testMpoolApi) MessagesForTipset(ts *types.TipSet) ([]store.ChainMsg, error) {
	if len(ts.Blocks()) != 1 {
		panic("cant deal with multiblock tipsets in this test")
	}

	bm, sm, err := tma.MessagesForBlock(ts.Blocks()[0])
	if err != nil {
		return nil, err
	}

	var out []store.ChainMsg
	for _, m := range bm {
		out = append(out, m)
	}

	for _, m := range sm {
		out = append(out, m)
	}

	return out, nil
}

func (tma *testMpoolApi) LoadTipSet(tsk types.TipSetKey) (*types.TipSet, error) {
	for _, ts := range tma.tipsets {
		if types.CidArrsEqual(tsk.Cids(), ts.Cids()) {
			return ts, nil
		}
	}

	return nil, fmt.Errorf("tipset not found")
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
	tma := newTestMpoolApi()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds)
	if err != nil {
		t.Fatal(err)
	}

	a := mock.MkBlock(nil, 1, 1)

	sender, err := w.GenerateKey(types.KTBLS)
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

func TestRevertMessages(t *testing.T) {
	tma := newTestMpoolApi()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds)
	if err != nil {
		t.Fatal(err)
	}

	a := mock.MkBlock(nil, 1, 1)
	b := mock.MkBlock(mock.TipSet(a), 1, 1)

	sender, err := w.GenerateKey(types.KTBLS)
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
	if len(p) != 3 {
		t.Fatal("expected three messages in mempool")
	}

}
