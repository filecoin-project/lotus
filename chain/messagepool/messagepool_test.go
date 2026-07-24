package messagepool

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	big2 "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/messagepool/gasguess"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
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

func (tma *testMpoolAPI) PutMessage(ctx context.Context, m types.ChainMsg) (cid.Cid, error) {
	return cid.Undef, nil
}

func (tma *testMpoolAPI) IsLite() bool {
	return false
}

func (tma *testMpoolAPI) PubSubPublish(string, []byte) error {
	tma.published++
	return nil
}

func (tma *testMpoolAPI) GetActorBefore(addr address.Address, ts *types.TipSet) (*types.Actor, error) {
	balance, ok := tma.balance[addr]
	if !ok {
		balance = types.NewInt(1000e6)
		tma.balance[addr] = balance
	}

	nonce := tma.statenonce[addr]

	return &types.Actor{
		Code:    builtin2.AccountActorCodeID,
		Nonce:   nonce,
		Balance: balance,
	}, nil
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
		Code:    builtin2.AccountActorCodeID,
		Nonce:   nonce,
		Balance: balance,
	}, nil
}

func (tma *testMpoolAPI) StateDeterministicAddressAtFinality(ctx context.Context, addr address.Address, ts *types.TipSet) (address.Address, error) {
	if addr.Protocol() != address.BLS && addr.Protocol() != address.SECP256K1 && addr.Protocol() != address.Delegated {
		return address.Undef, fmt.Errorf("given address was not a key addr")
	}
	return addr, nil
}

func (tma *testMpoolAPI) StateNetworkVersion(ctx context.Context, h abi.ChainEpoch) network.Version {
	return buildconstants.TestNetworkVersion
}

func (tma *testMpoolAPI) MessagesForBlock(ctx context.Context, h *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	return nil, tma.bmsgs[h.Cid()], nil
}

func (tma *testMpoolAPI) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	if len(ts.Blocks()) != 1 {
		panic("cant deal with multiblock tipsets in this test")
	}

	bm, sm, err := tma.MessagesForBlock(ctx, ts.Blocks()[0])
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

func (tma *testMpoolAPI) LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
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
	n, err := mp.GetNonce(context.TODO(), addr, types.EmptyTSK)
	if err != nil {
		t.Fatal(err)
	}

	if n != val {
		t.Fatalf("expected nonce of %d, got %d", val, n)
	}
}

func mustAdd(t *testing.T, mp *MessagePool, msg *types.SignedMessage) {
	t.Helper()
	if err := mp.Add(context.TODO(), msg); err != nil {
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

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
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

func TestCheckMessageBig(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	{
		msg := &types.Message{
			To:         to,
			From:       from,
			Value:      types.NewInt(1),
			Nonce:      0,
			GasLimit:   60000000,
			GasFeeCap:  types.NewInt(100),
			GasPremium: types.NewInt(1),
			Params:     make([]byte, 41<<10), // 41KiB payload
		}

		sig, err := w.WalletSign(context.TODO(), from, msg.Cid().Bytes(), api.MsgMeta{})
		if err != nil {
			panic(err)
		}
		sm := &types.SignedMessage{
			Message:   *msg,
			Signature: *sig,
		}
		mustAdd(t, mp, sm)
	}

	{
		msg := &types.Message{
			To:         to,
			From:       from,
			Value:      types.NewInt(1),
			Nonce:      0,
			GasLimit:   50000000,
			GasFeeCap:  types.NewInt(100),
			GasPremium: types.NewInt(1),
			Params:     make([]byte, 64<<10), // 64KiB payload
		}

		sig, err := w.WalletSign(context.TODO(), from, msg.Cid().Bytes(), api.MsgMeta{})
		if err != nil {
			panic(err)
		}
		sm := &types.SignedMessage{
			Message:   *msg,
			Signature: *sig,
		}
		err = mp.Add(context.TODO(), sm)
		assert.ErrorIs(t, err, ErrMessageTooBig)
	}
}

func TestMessagePoolMessagesInEachBlock(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
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

	_, _ = mp.Pending(context.TODO())

	selm, _ := mp.SelectMessages(context.Background(), tsa, 1)
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

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
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

	p, _ := mp.Pending(context.TODO())
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

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
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
		if err := mp.Add(context.TODO(), smsg); err != nil {
			t.Fatal(err)
		}
	}

	for i := 10; i < 50; i++ {
		smsg := mock.MkMessage(sender, target, uint64(i), w)
		if err := mp.Add(context.TODO(), smsg); err != nil {
			t.Fatal(err)
		}
	}

	mp.cfg.SizeLimitHigh = 40
	mp.cfg.SizeLimitLow = 10

	mp.Prune()

	msgs, _ := mp.Pending(context.TODO())
	if len(msgs) != 5 {
		t.Fatal("expected only 5 messages in pool, got: ", len(msgs))
	}
}

func TestGasRewardNegative(t *testing.T) {
	var mp MessagePool

	msg := types.SignedMessage{
		Message: types.Message{
			GasLimit:   1000,
			GasFeeCap:  big2.NewInt(20000),
			GasPremium: big2.NewInt(15000),
		},
	}
	baseFee := big2.NewInt(30000)
	// Over the GasPremium, but under the BaseFee
	gr1 := mp.getGasReward(&msg, baseFee)

	msg.Message.GasFeeCap = big2.NewInt(15000)
	// Equal to GasPremium, under the BaseFee
	gr2 := mp.getGasReward(&msg, baseFee)

	msg.Message.GasFeeCap = big2.NewInt(10000)
	// Under both GasPremium and BaseFee
	gr3 := mp.getGasReward(&msg, baseFee)

	require.True(t, gr1.Sign() < 0)
	require.True(t, gr2.Sign() < 0)
	require.True(t, gr3.Sign() < 0)

	require.True(t, gr1.Cmp(gr2) > 0)
	require.True(t, gr2.Cmp(gr3) > 0)
}

func TestLoadLocal(t *testing.T) {
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

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL
	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin2.StorageMarketActorCodeID, M: 2}]
	msgs := make(map[cid.Cid]struct{})
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		cid, err := mp.Push(context.TODO(), m, true)
		if err != nil {
			t.Fatal(err)
		}
		msgs[cid] = struct{}{}
	}
	err = mp.Close()
	if err != nil {
		t.Fatal(err)
	}

	mp, err = New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	if err != nil {
		t.Fatal(err)
	}

	pmsgs, _ := mp.Pending(context.TODO())
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
		t.Fatalf("not all messages were loaded; missing %d messages", len(msgs))
	}
}

func TestClearAll(t *testing.T) {
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

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL
	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin2.StorageMarketActorCodeID, M: 2}]
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(context.TODO(), m, true)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(i+1))
		mustAdd(t, mp, m)
	}

	mp.Clear(context.Background(), true)

	pending, _ := mp.Pending(context.TODO())
	if len(pending) > 0 {
		t.Fatalf("cleared the mpool, but got %d pending messages", len(pending))
	}
}

func TestClearNonLocal(t *testing.T) {
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

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin2.StorageMarketActorCodeID, M: 2}]
	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(context.TODO(), m, true)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w2, a2, a1, uint64(i), gasLimit, uint64(i+1))
		mustAdd(t, mp, m)
	}

	mp.Clear(context.Background(), false)

	pending, _ := mp.Pending(context.TODO())
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

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	ch, err := mp.Updates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	gasLimit := gasguess.Costs[gasguess.CostKey{Code: builtin2.StorageMarketActorCodeID, M: 2}]

	tma.setBalance(a1, 1) // in FIL
	tma.setBalance(a2, 1) // in FIL

	for i := 0; i < 10; i++ {
		m := makeTestMessage(w1, a1, a2, uint64(i), gasLimit, uint64(i+1))
		_, err := mp.Push(context.TODO(), m, true)
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

func TestMessageBelowMinGasFee(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	// fee is just below minimum gas fee
	fee := minimumBaseFee.Uint64() - 1
	{
		msg := &types.Message{
			To:         to,
			From:       from,
			Value:      types.NewInt(1),
			Nonce:      0,
			GasLimit:   50000000,
			GasFeeCap:  types.NewInt(fee),
			GasPremium: types.NewInt(1),
			Params:     make([]byte, 32<<10),
		}

		sig, err := w.WalletSign(context.TODO(), from, msg.Cid().Bytes(), api.MsgMeta{})
		if err != nil {
			panic(err)
		}
		sm := &types.SignedMessage{
			Message:   *msg,
			Signature: *sig,
		}
		err = mp.Add(context.TODO(), sm)
		assert.ErrorIs(t, err, ErrGasFeeCapTooLow)
	}
}

func TestMessageValueTooHigh(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	totalFil := types.TotalFilecoinInt
	extra := types.NewInt(1)

	value := types.BigAdd(totalFil, extra)
	{
		msg := &types.Message{
			To:         to,
			From:       from,
			Value:      value,
			Nonce:      0,
			GasLimit:   50000000,
			GasFeeCap:  types.NewInt(minimumBaseFee.Uint64()),
			GasPremium: types.NewInt(1),
			Params:     make([]byte, 32<<10),
		}

		sig, err := w.WalletSign(context.TODO(), from, msg.Cid().Bytes(), api.MsgMeta{})
		if err != nil {
			panic(err)
		}
		sm := &types.SignedMessage{
			Message:   *msg,
			Signature: *sig,
		}
		err = mp.Add(context.TODO(), sm)
		assert.Error(t, err)
	}
}

func TestCheckBalanceInsufficientFunds(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	require.NoError(t, err)

	to := mock.Address(1001)

	// GasFeeCap=100, GasLimit=50_000_000 → RequiredFunds = 5_000_000_000 attoFIL
	const gasLimit = 50_000_000
	const feeCap = 100
	requiredFunds := int64(feeCap * gasLimit)

	// Set balance 1 attoFIL below what the message needs
	tma.setBalanceRaw(from, types.NewInt(uint64(requiredFunds-1)))

	ds := datastore.NewMapDatastore()
	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)

	sm := makeTestMessage(w, from, to, 0, gasLimit, 0)
	err = mp.Add(context.TODO(), sm)
	require.ErrorIs(t, err, ErrNotEnoughFunds)
}

func TestCheckBalanceCumulativePending(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	require.NoError(t, err)

	to := mock.Address(1001)

	// GasFeeCap=100, GasLimit=50_000_000 → RequiredFunds = 5_000_000_000 attoFIL per message
	const gasLimit = 50_000_000
	const feeCap = 100
	requiredFunds := int64(feeCap * gasLimit)

	// Set balance to cover exactly one message but not two
	tma.setBalanceRaw(from, types.NewInt(uint64(2*requiredFunds-1)))

	ds := datastore.NewMapDatastore()
	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)

	// First message fits within balance
	sm0 := makeTestMessage(w, from, to, 0, gasLimit, 0)
	require.NoError(t, mp.Add(context.TODO(), sm0))

	// Second message pushes cumulative cost over balance
	sm1 := makeTestMessage(w, from, to, 1, gasLimit, 0)
	err = mp.Add(context.TODO(), sm1)
	require.ErrorIs(t, err, ErrSoftValidationFailure)
}

func TestMessageSignatureInvalid(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	{
		msg := &types.Message{
			To:         to,
			From:       from,
			Value:      types.NewInt(1),
			Nonce:      0,
			GasLimit:   50000000,
			GasFeeCap:  types.NewInt(minimumBaseFee.Uint64()),
			GasPremium: types.NewInt(1),
			Params:     make([]byte, 32<<10),
		}

		badSig := &crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: make([]byte, 0),
		}
		sm := &types.SignedMessage{
			Message:   *msg,
			Signature: *badSig,
		}
		err = mp.Add(context.TODO(), sm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signature for message bafy2bz")
	}
}

func TestAddMessageTwice(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	{
		msg := &types.Message{
			To:         to,
			From:       from,
			Value:      types.NewInt(1),
			Nonce:      0,
			GasLimit:   50000000,
			GasFeeCap:  types.NewInt(minimumBaseFee.Uint64()),
			GasPremium: types.NewInt(1),
			Params:     make([]byte, 32<<10),
		}

		sig, err := w.WalletSign(context.TODO(), from, msg.Cid().Bytes(), api.MsgMeta{})
		if err != nil {
			panic(err)
		}
		sm := &types.SignedMessage{
			Message:   *msg,
			Signature: *sig,
		}
		mustAdd(t, mp, sm)

		err = mp.Add(context.TODO(), sm)
		assert.Contains(t, err.Error(), "with nonce 0 already in mpool")
	}
}

func TestAddMessageTwiceNonceGap(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	{
		// create message with invalid nonce (1)
		sm := makeTestMessage(w, from, to, 1, 50_000_000, minimumBaseFee.Uint64())
		mustAdd(t, mp, sm)

		// then try to add message again
		err = mp.Add(context.TODO(), sm)
		assert.Contains(t, err.Error(), "unfulfilled nonce gap")
	}
}

func TestAddMessageTwiceCidDiff(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	{
		sm := makeTestMessage(w, from, to, 0, 50_000_000, minimumBaseFee.Uint64())
		mustAdd(t, mp, sm)

		// Create message with different data, so CID is different
		sm2 := makeTestMessage(w, from, to, 0, 50_000_001, minimumBaseFee.Uint64())

		// then try to add message again
		err = mp.Add(context.TODO(), sm2)
		// assert.Contains(t, err.Error(), "replace by fee has too low GasPremium")
		assert.Error(t, err)
	}
}

func TestAddMessageTwiceCidDiffReplaced(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	{
		sm := makeTestMessage(w, from, to, 0, 50_000_000, minimumBaseFee.Uint64())
		mustAdd(t, mp, sm)

		// Create message with different data, so CID is different
		sm2 := makeTestMessage(w, from, to, 0, 50_000_000, minimumBaseFee.Uint64()*2)
		mustAdd(t, mp, sm2)
	}
}

func TestRemoveMessage(t *testing.T) {
	tma := newTestMpoolAPI()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	assert.NoError(t, err)

	from, err := w.WalletNew(context.Background(), types.KTBLS)
	assert.NoError(t, err)

	tma.setBalance(from, 1000e9)

	ds := datastore.NewMapDatastore()

	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	assert.NoError(t, err)

	to := mock.Address(1001)

	{
		sm := makeTestMessage(w, from, to, 0, 50_000_000, minimumBaseFee.Uint64())
		mustAdd(t, mp, sm)

		// remove message for sender
		mp.Remove(context.TODO(), from, sm.Message.Nonce, true)

		// check messages in pool: should be none present
		msgs := mp.pendingFor(context.TODO(), from)
		assert.Len(t, msgs, 0)
	}
}

func TestCapGasFee(t *testing.T) {
	t.Run("use default maxfee", func(t *testing.T) {
		msg := &types.Message{
			GasLimit:   100_000_000,
			GasFeeCap:  abi.NewTokenAmount(100_000_000),
			GasPremium: abi.NewTokenAmount(100_000),
		}
		CapGasFee(func() (abi.TokenAmount, error) {
			return abi.NewTokenAmount(100_000_000_000), nil
		}, msg, nil)
		assert.Equal(t, msg.GasFeeCap.Int64(), int64(1000))
		assert.Equal(t, msg.GasPremium.Int.Int64(), int64(1000))
	})

	t.Run("use spec maxfee", func(t *testing.T) {
		msg := &types.Message{
			GasLimit:   100_000_000,
			GasFeeCap:  abi.NewTokenAmount(100_000_000),
			GasPremium: abi.NewTokenAmount(100_000),
		}
		CapGasFee(nil, msg, &api.MessageSendSpec{MaxFee: abi.NewTokenAmount(100_000_000_000)})
		assert.Equal(t, msg.GasFeeCap.Int64(), int64(1000))
		assert.Equal(t, msg.GasPremium.Int.Int64(), int64(1000))
	})

	t.Run("use smaller feecap value when fee is enough", func(t *testing.T) {
		msg := &types.Message{
			GasLimit:   100_000_000,
			GasFeeCap:  abi.NewTokenAmount(100_000),
			GasPremium: abi.NewTokenAmount(100_000_000),
		}
		CapGasFee(nil, msg, &api.MessageSendSpec{MaxFee: abi.NewTokenAmount(100_000_000_000_000)})
		assert.Equal(t, msg.GasFeeCap.Int64(), int64(100_000))
		assert.Equal(t, msg.GasPremium.Int.Int64(), int64(100_000))
	})
}

// makeMsgSetMsg creates a minimal SignedMessage for testing msgSet directly.
// msgSet.add does not validate the signature, so a zero-filled one is sufficient.
func makeMsgSetMsg(nonce uint64) *types.SignedMessage {
	return &types.SignedMessage{
		Message: types.Message{
			From:       mock.Address(1000),
			To:         mock.Address(1001),
			Nonce:      nonce,
			GasLimit:   50_000_000,
			GasFeeCap:  types.NewInt(100),
			GasPremium: types.NewInt(1),
			Value:      types.NewInt(0),
		},
		Signature: crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: make([]byte, 65),
		},
	}
}

// mustAddToMsgSet adds a message to ms, failing the test on error.
func mustAddToMsgSet(t *testing.T, ms *msgSet, nonce uint64) {
	t.Helper()
	_, err := ms.add(makeMsgSetMsg(nonce), nil, false, false)
	require.NoError(t, err)
}

// requireNoGapsBelowNextNonce checks the invariant: every nonce in [startNonce, ms.nextNonce)
// must be present in ms.msgs.
func requireNoGapsBelowNextNonce(t *testing.T, ms *msgSet, startNonce uint64) {
	t.Helper()
	for n := startNonce; n < ms.nextNonce; n++ {
		if _, ok := ms.msgs[n]; !ok {
			t.Errorf("invariant violated: nonce %d missing below nextNonce %d", n, ms.nextNonce)
		}
	}
}

// TestMsgSetNoGapBelowNextNonceSequentialAdd verifies that adding messages in nonce order
// advances nextNonce one step at a time with no gaps below it.
func TestMsgSetNoGapBelowNextNonceSequentialAdd(t *testing.T) {
	ms := newMsgSet(0)
	for i := uint64(0); i < 5; i++ {
		mustAddToMsgSet(t, ms, i)
		require.Equal(t, i+1, ms.nextNonce, "nextNonce after adding nonce %d", i)
		requireNoGapsBelowNextNonce(t, ms, 0)
	}
}

// TestMsgSetNoGapBelowNextNonceFillSimpleGap verifies that adding the missing message in a
// one-nonce gap advances nextNonce past the already-present message above the gap.
func TestMsgSetNoGapBelowNextNonceFillSimpleGap(t *testing.T) {
	ms := newMsgSet(0)
	mustAddToMsgSet(t, ms, 0)
	mustAddToMsgSet(t, ms, 2) // gap at 1; nextNonce stays at 1
	require.Equal(t, uint64(1), ms.nextNonce)
	requireNoGapsBelowNextNonce(t, ms, 0)

	mustAddToMsgSet(t, ms, 1) // fills gap; 2 was already present, so nextNonce advances to 3
	require.Equal(t, uint64(3), ms.nextNonce)
	requireNoGapsBelowNextNonce(t, ms, 0)
}

// TestMsgSetNoGapBelowNextNonceFillBridgesRun verifies that filling one gap in a multi-message
// run advances nextNonce past all subsequent already-present messages in one step.
func TestMsgSetNoGapBelowNextNonceFillBridgesRun(t *testing.T) {
	ms := newMsgSet(0)
	mustAddToMsgSet(t, ms, 0)
	mustAddToMsgSet(t, ms, 1)
	// Add 3, 4, 5 with a gap at 2; nextNonce stays at 2.
	mustAddToMsgSet(t, ms, 3)
	mustAddToMsgSet(t, ms, 4)
	mustAddToMsgSet(t, ms, 5)
	require.Equal(t, uint64(2), ms.nextNonce)
	requireNoGapsBelowNextNonce(t, ms, 0)

	// Filling nonce 2 bridges the gap and advances nextNonce all the way to 6.
	mustAddToMsgSet(t, ms, 2)
	require.Equal(t, uint64(6), ms.nextNonce)
	requireNoGapsBelowNextNonce(t, ms, 0)
}

// TestMsgSetNoGapBelowNextNoncePruneBelow verifies that pruning a message below nextNonce
// rewinds nextNonce so the removed nonce becomes the new gap boundary (at nextNonce, not below it).
func TestMsgSetNoGapBelowNextNoncePruneBelow(t *testing.T) {
	ms := newMsgSet(0)
	mustAddToMsgSet(t, ms, 0)
	mustAddToMsgSet(t, ms, 1)
	mustAddToMsgSet(t, ms, 2)
	require.Equal(t, uint64(3), ms.nextNonce)

	// Prune nonce 1 (below nextNonce=3); nextNonce must rewind to 1.
	ms.rm(1, false)
	require.Equal(t, uint64(1), ms.nextNonce)
	// The only nonce below nextNonce=1 is 0, which still exists.
	requireNoGapsBelowNextNonce(t, ms, 0)
}

// TestMsgSetNoGapBelowNextNoncePruneAbove verifies that pruning a gapped message at or above
// nextNonce leaves nextNonce unchanged and preserves the invariant.
func TestMsgSetNoGapBelowNextNoncePruneAbove(t *testing.T) {
	ms := newMsgSet(0)
	mustAddToMsgSet(t, ms, 0)
	mustAddToMsgSet(t, ms, 2) // gap at 1; nextNonce=1
	require.Equal(t, uint64(1), ms.nextNonce)

	// Prune the gapped message (nonce 2 >= nextNonce=1); nextNonce must not change.
	ms.rm(2, false)
	require.Equal(t, uint64(1), ms.nextNonce)
	requireNoGapsBelowNextNonce(t, ms, 0)
}

// TestMsgSetNoGapBelowNextNonceAppliedUnknownFillsGap verifies that removing an unknown message
// via applied=true advances nextNonce and fills any immediately-following gaps, so the invariant
// holds with the new chain base as the start.
func TestMsgSetNoGapBelowNextNonceAppliedUnknownFillsGap(t *testing.T) {
	ms := newMsgSet(0)
	// Add nonce 1 as a gapped message without adding nonce 0 first.
	mustAddToMsgSet(t, ms, 1)
	require.Equal(t, uint64(0), ms.nextNonce)

	// Apply nonce 0, which we never saw: nextNonce advances to 1, then skips over the
	// already-present nonce 1 to land at 2.
	ms.rm(0, true)
	require.Equal(t, uint64(2), ms.nextNonce)
	// Base is now 1 (nonce 0 was applied on-chain).
	requireNoGapsBelowNextNonce(t, ms, 1)
}

// TestMsgSetNoGapBelowNextNoncePruneAndRefill verifies that pruning a message and then
// re-adding it restores the full contiguous run and advances nextNonce past all pre-existing
// messages in the gap.
func TestMsgSetNoGapBelowNextNoncePruneAndRefill(t *testing.T) {
	ms := newMsgSet(0)
	mustAddToMsgSet(t, ms, 0)
	mustAddToMsgSet(t, ms, 1)
	mustAddToMsgSet(t, ms, 2)
	mustAddToMsgSet(t, ms, 3)
	require.Equal(t, uint64(4), ms.nextNonce)

	// Prune nonce 1; nextNonce rewinds to 1, nonces 2 and 3 become gaps above nextNonce.
	ms.rm(1, false)
	require.Equal(t, uint64(1), ms.nextNonce)
	requireNoGapsBelowNextNonce(t, ms, 0)

	// Re-add nonce 1; nextNonce must bridge over 2 and 3 to reach 4.
	mustAddToMsgSet(t, ms, 1)
	require.Equal(t, uint64(4), ms.nextNonce)
	requireNoGapsBelowNextNonce(t, ms, 0)
}

// requireMpoolNoGapBelowNextNonce checks the msgSet invariant for addr in mp:
// no nonce is missing in [minPendingNonce, nextNonce). Acquires mp.lk for the check.
func requireMpoolNoGapBelowNextNonce(t *testing.T, mp *MessagePool, addr address.Address) {
	t.Helper()
	mp.lk.RLock()
	defer mp.lk.RUnlock()
	ms, ok := mp.pending[addr]
	if !ok || len(ms.msgs) == 0 {
		return
	}
	minNonce := ms.nextNonce
	for n := range ms.msgs {
		if n < minNonce {
			minNonce = n
		}
	}
	for n := minNonce; n < ms.nextNonce; n++ {
		if _, ok := ms.msgs[n]; !ok {
			t.Errorf("nonce gap below nextNonce: nonce %d missing, nextNonce=%d", n, ms.nextNonce)
		}
	}
}

// TestHeadChangeApplyNoGapBelowNextNonce verifies that applying a block with sequential
// messages removes those messages from the mpool without leaving any gaps below nextNonce.
func TestHeadChangeApplyNoGapBelowNextNonce(t *testing.T) {
	tma := newTestMpoolAPI()
	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)
	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)
	target := mock.Address(1001)

	ds := datastore.NewMapDatastore()
	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)

	tma.setBalance(sender, 1)

	var msgs []*types.SignedMessage
	for i := 0; i < 5; i++ {
		m := mock.MkMessage(sender, target, uint64(i), w)
		msgs = append(msgs, m)
		mustAdd(t, mp, m)
	}
	requireMpoolNoGapBelowNextNonce(t, mp, sender)

	block := tma.nextBlock()
	tma.setBlockMessages(block, msgs[:3]...)
	tma.setStateNonce(sender, 3)
	tma.applyBlock(t, block)

	requireMpoolNoGapBelowNextNonce(t, mp, sender)
}

// TestHeadChangeApplyUnknownFillsGap verifies that applying a block containing a message
// not in the mpool (unknown to us) fills the gap and advances nextNonce correctly.
func TestHeadChangeApplyUnknownFillsGap(t *testing.T) {
	tma := newTestMpoolAPI()
	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)
	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)
	target := mock.Address(1001)

	ds := datastore.NewMapDatastore()
	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)

	tma.setBalance(sender, 1)

	// Add nonces 1, 2, 3 without nonce 0: gap at 0.
	nonce0 := mock.MkMessage(sender, target, 0, w)
	for i := 1; i <= 3; i++ {
		mustAdd(t, mp, mock.MkMessage(sender, target, uint64(i), w))
	}
	requireMpoolNoGapBelowNextNonce(t, mp, sender)

	// Apply a block containing nonce 0 (which was never in the mpool).
	block := tma.nextBlock()
	tma.setBlockMessages(block, nonce0)
	tma.setStateNonce(sender, 1)
	tma.applyBlock(t, block)

	// After applying nonce 0, the gap closes and nextNonce must advance past 1, 2, 3.
	requireMpoolNoGapBelowNextNonce(t, mp, sender)
	mp.lk.RLock()
	ms := mp.pending[sender]
	mp.lk.RUnlock()
	require.Equal(t, uint64(4), ms.nextNonce)
}

// TestHeadChangeRevertNoGapBelowNextNonce verifies that reverting a block re-adds its messages
// to the mpool without leaving gaps below nextNonce.
func TestHeadChangeRevertNoGapBelowNextNonce(t *testing.T) {
	tma := newTestMpoolAPI()
	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)
	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)
	target := mock.Address(1001)

	ds := datastore.NewMapDatastore()
	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)

	tma.setBalance(sender, 1)

	var msgs []*types.SignedMessage
	for i := 0; i < 5; i++ {
		m := mock.MkMessage(sender, target, uint64(i), w)
		msgs = append(msgs, m)
		mustAdd(t, mp, m)
	}

	// Apply block with nonces 0-2.
	block := tma.nextBlock()
	tma.setBlockMessages(block, msgs[:3]...)
	tma.setStateNonce(sender, 3)
	tma.applyBlock(t, block)
	requireMpoolNoGapBelowNextNonce(t, mp, sender)

	// Revert the block: nonces 0-2 come back into the mpool.
	tma.setStateNonce(sender, 0)
	tma.revertBlock(t, block)

	requireMpoolNoGapBelowNextNonce(t, mp, sender)
}

// TestHeadChangeReorgIncreaseNoGapBelowNextNonce verifies the invariant after a reorg that
// increases the chain nonce (the replacing fork includes more messages than the reverted one).
func TestHeadChangeReorgIncreaseNoGapBelowNextNonce(t *testing.T) {
	tma := newTestMpoolAPI()
	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)
	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)
	target := mock.Address(1001)

	ds := datastore.NewMapDatastore()
	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)

	tma.setBalance(sender, 1)

	var allMsgs []*types.SignedMessage
	for i := 0; i < 10; i++ {
		m := mock.MkMessage(sender, target, uint64(i), w)
		allMsgs = append(allMsgs, m)
		mustAdd(t, mp, m)
	}

	// Apply fork A (nonces 0-4) so the mpool removes them.
	parentTs := tma.tipsets[len(tma.tipsets)-1]
	blockA := tma.nextBlock()
	tma.setBlockMessages(blockA, allMsgs[:5]...)
	tma.setStateNonce(sender, 5)
	tma.applyBlock(t, blockA)
	requireMpoolNoGapBelowNextNonce(t, mp, sender)

	// Fork B (same parent, nonces 0-6) replaces A in a reorg.
	blockB := mock.MkBlock(parentTs, 1, 2)
	tma.tipsets = append(tma.tipsets, mock.TipSet(blockB))
	tma.setBlockMessages(blockB, allMsgs[:7]...)
	tma.setStateNonce(sender, 7)

	err = tma.cb([]*types.TipSet{mock.TipSet(blockA)}, []*types.TipSet{mock.TipSet(blockB)})
	require.NoError(t, err)

	// After the reorg, nonces 7-9 remain pending with no gaps.
	requireMpoolNoGapBelowNextNonce(t, mp, sender)
}

// TestHeadChangeReorgDecreaseNoGapBelowNextNonce verifies the invariant after a reorg that
// decreases the chain nonce (the replacing fork includes fewer messages than the reverted one).
func TestHeadChangeReorgDecreaseNoGapBelowNextNonce(t *testing.T) {
	tma := newTestMpoolAPI()
	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	require.NoError(t, err)
	sender, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)
	target := mock.Address(1001)

	ds := datastore.NewMapDatastore()
	mp, err := New(context.Background(), tma, ds, filcns.DefaultUpgradeSchedule(), "mptest", nil)
	require.NoError(t, err)

	tma.setBalance(sender, 1)

	var allMsgs []*types.SignedMessage
	for i := 0; i < 10; i++ {
		m := mock.MkMessage(sender, target, uint64(i), w)
		allMsgs = append(allMsgs, m)
		mustAdd(t, mp, m)
	}

	// Apply fork A (nonces 0-6) so the mpool removes them.
	parentTs := tma.tipsets[len(tma.tipsets)-1]
	blockA := tma.nextBlock()
	tma.setBlockMessages(blockA, allMsgs[:7]...)
	tma.setStateNonce(sender, 7)
	tma.applyBlock(t, blockA)
	requireMpoolNoGapBelowNextNonce(t, mp, sender)

	// Fork B (same parent, nonces 0-2) replaces A in a reorg.
	// Nonces 3-6 are reverted from A but not included in B, so they rejoin the mpool.
	blockB := mock.MkBlock(parentTs, 1, 2)
	tma.tipsets = append(tma.tipsets, mock.TipSet(blockB))
	tma.setBlockMessages(blockB, allMsgs[:3]...)
	tma.setStateNonce(sender, 3)

	err = tma.cb([]*types.TipSet{mock.TipSet(blockA)}, []*types.TipSet{mock.TipSet(blockB)})
	require.NoError(t, err)

	// After the reorg, nonces 3-9 must all be pending with no gaps.
	requireMpoolNoGapBelowNextNonce(t, mp, sender)
}
