package messagepool

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
	"github.com/filecoin-project/specs-actors/actors/crypto"
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

	tipsets []*types.TipSet
}

func newTestMpoolAPI() *testMpoolAPI {
	return &testMpoolAPI{
		bmsgs:      make(map[cid.Cid][]*types.SignedMessage),
		statenonce: make(map[address.Address]uint64),
	}
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

func (tma *testMpoolAPI) setBlockMessages(h *types.BlockHeader, msgs ...*types.SignedMessage) {
	tma.bmsgs[h.Cid()] = msgs
	tma.tipsets = append(tma.tipsets, mock.TipSet(h))
}

func (tma *testMpoolAPI) SubscribeHeadChanges(cb func(rev, app []*types.TipSet) error) *types.TipSet {
	tma.cb = cb
	return nil
}

func (tma *testMpoolAPI) PutMessage(m types.ChainMsg) (cid.Cid, error) {
	return cid.Undef, nil
}

func (tma *testMpoolAPI) PubSubPublish(string, []byte) error {
	return nil
}

func (tma *testMpoolAPI) StateGetActor(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	return &types.Actor{
		Nonce:   tma.statenonce[addr],
		Balance: types.NewInt(90000000),
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

	mp, err := New(tma, ds, "mptest")
	if err != nil {
		t.Fatal(err)
	}

	a := mock.MkBlock(nil, 1, 1)

	sender, err := w.GenerateKey(crypto.SigTypeBLS)
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
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest")
	if err != nil {
		t.Fatal(err)
	}

	a := mock.MkBlock(nil, 1, 1)
	b := mock.MkBlock(mock.TipSet(a), 1, 1)

	sender, err := w.GenerateKey(crypto.SigTypeBLS)
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

func TestPruningSimple(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(tma, ds, "mptest")
	if err != nil {
		t.Fatal(err)
	}

	a := mock.MkBlock(nil, 1, 1)
	tma.applyBlock(t, a)

	sender, err := w.GenerateKey(crypto.SigTypeBLS)
	if err != nil {
		t.Fatal(err)
	}
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

	mp.maxTxPoolSizeHi = 40
	mp.maxTxPoolSizeLo = 10

	mp.Prune()

	msgs, _ := mp.Pending()
	if len(msgs) != 5 {
		t.Fatal("expected only 5 messages in pool, got: ", len(msgs))
	}
}
