package actors_test

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
)

const testGasLimit = 10000

type HarnessInit struct {
	NAddrs uint64
	Addrs  map[address.Address]types.BigInt
	Miner  address.Address
}

type HarnessStage int

const (
	HarnessPreInit HarnessStage = iota
	HarnessPostInit
)

type HarnessOpt func(testing.TB, *Harness) error

type Harness struct {
	HI          HarnessInit
	Stage       HarnessStage
	Nonces      map[address.Address]uint64
	GasCharges  map[address.Address]types.BigInt
	Rand        vm.Rand
	BlockHeight uint64

	lastBalanceCheck map[address.Address]types.BigInt

	ctx context.Context
	bs  blockstore.Blockstore
	vm  *vm.VM
	cs  *store.ChainStore
	w   *wallet.Wallet
}

var HarnessMinerFunds = types.NewInt(1000000)

func HarnessAddr(addr *address.Address, value uint64) HarnessOpt {
	return func(t testing.TB, h *Harness) error {
		if h.Stage != HarnessPreInit {
			return nil
		}
		hi := &h.HI
		if addr.Empty() {
			k, err := h.w.GenerateKey(types.KTSecp256k1)
			if err != nil {
				t.Fatal(err)
			}

			*addr = k
		}
		hi.Addrs[*addr] = types.NewInt(value)
		return nil
	}
}

func HarnessMiner(addr *address.Address) HarnessOpt {
	return func(_ testing.TB, h *Harness) error {
		if h.Stage != HarnessPreInit {
			return nil
		}
		hi := &h.HI
		if addr.Empty() {
			*addr = hi.Miner
			return nil
		}
		delete(hi.Addrs, hi.Miner)
		hi.Miner = *addr
		return nil
	}
}

func HarnessActor(actor *address.Address, creator *address.Address, code cid.Cid, params func() cbg.CBORMarshaler) HarnessOpt {
	return func(t testing.TB, h *Harness) error {
		if h.Stage != HarnessPostInit {
			return nil
		}
		if !actor.Empty() {
			return xerrors.New("actor address should be empty")
		}

		ret, _ := h.CreateActor(t, *creator, code, params())
		if ret.ExitCode != 0 {
			return xerrors.Errorf("creating actor: %w", ret.ActorErr)
		}
		var err error
		*actor, err = address.NewFromBytes(ret.Return)
		return err
	}

}

func HarnessCtx(ctx context.Context) HarnessOpt {
	return func(t testing.TB, h *Harness) error {
		h.ctx = ctx
		return nil
	}
}

func NewHarness(t *testing.T, options ...HarnessOpt) *Harness {
	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}
	h := &Harness{
		Stage:  HarnessPreInit,
		Nonces: make(map[address.Address]uint64),
		Rand:   &fakeRand{},
		HI: HarnessInit{
			NAddrs: 1,
			Miner:  blsaddr(0),
			Addrs: map[address.Address]types.BigInt{
				blsaddr(0): HarnessMinerFunds,
			},
		},
		GasCharges: make(map[address.Address]types.BigInt),

		lastBalanceCheck: make(map[address.Address]types.BigInt),
		w:                w,
		ctx:              context.Background(),
		bs:               bstore.NewBlockstore(dstore.NewMapDatastore()),
		BlockHeight:      0,
	}
	for _, opt := range options {
		err := opt(t, h)
		if err != nil {
			t.Fatalf("Applying options: %v", err)
		}
	}

	st, err := gen.MakeInitialStateTree(h.bs, h.HI.Addrs)
	if err != nil {
		t.Fatal(err)
	}

	stateroot, err := st.Flush()
	if err != nil {
		t.Fatal(err)
	}

	stateroot, err = gen.SetupStorageMarketActor(h.bs, stateroot, nil)
	if err != nil {
		t.Fatal(err)
	}

	h.cs = store.NewChainStore(h.bs, nil)
	h.vm, err = vm.NewVM(stateroot, 1, h.Rand, h.HI.Miner, h.cs.Blockstore())
	if err != nil {
		t.Fatal(err)
	}
	h.Stage = HarnessPostInit
	for _, opt := range options {
		err := opt(t, h)
		if err != nil {
			t.Fatalf("Applying options: %v", err)
		}
	}

	return h
}

func (h *Harness) Apply(t testing.TB, msg types.Message) (*vm.ApplyRet, *state.StateTree) {
	t.Helper()
	if msg.Nonce == 0 {
		msg.Nonce, _ = h.Nonces[msg.From]
		h.Nonces[msg.From] = msg.Nonce + 1
	}

	ret, err := h.vm.ApplyMessage(h.ctx, &msg)
	if err != nil {
		t.Fatalf("Applying message: %+v", err)
	}

	if ret != nil {
		if prev, ok := h.GasCharges[msg.From]; ok {
			h.GasCharges[msg.From] = types.BigAdd(prev, ret.GasUsed)
		} else {
			h.GasCharges[msg.From] = ret.GasUsed
		}
	}

	stateroot, err := h.vm.Flush(context.TODO())
	if err != nil {
		t.Fatalf("Flushing VM: %+v", err)
	}
	cst := hamt.CSTFromBstore(h.bs)
	state, err := state.LoadStateTree(cst, stateroot)
	if err != nil {
		t.Fatalf("Loading state tree: %+v", err)
	}
	return ret, state
}

func (h *Harness) CreateActor(t testing.TB, from address.Address,
	code cid.Cid, params cbg.CBORMarshaler) (*vm.ApplyRet, *state.StateTree) {
	t.Helper()

	return h.Apply(t, types.Message{
		To:     actors.InitAddress,
		From:   from,
		Method: actors.IAMethods.Exec,
		Params: DumpObject(t,
			&actors.ExecParams{
				Code:   code,
				Params: DumpObject(t, params),
			}),
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(testGasLimit),
		Value:    types.NewInt(0),
	})
}

func (h *Harness) SendFunds(t testing.TB, from address.Address, to address.Address,
	value types.BigInt) (*vm.ApplyRet, *state.StateTree) {
	t.Helper()
	return h.Apply(t, types.Message{
		To:       to,
		From:     from,
		Method:   0,
		Value:    value,
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(testGasLimit),
	})
}

func (h *Harness) Invoke(t testing.TB, from address.Address, to address.Address,
	method uint64, params cbg.CBORMarshaler) (*vm.ApplyRet, *state.StateTree) {
	t.Helper()
	return h.InvokeWithValue(t, from, to, method, types.NewInt(0), params)
}

func (h *Harness) InvokeWithValue(t testing.TB, from address.Address, to address.Address,
	method uint64, value types.BigInt, params cbg.CBORMarshaler) (*vm.ApplyRet, *state.StateTree) {
	t.Helper()
	h.vm.SetBlockHeight(h.BlockHeight)
	return h.Apply(t, types.Message{
		To:       to,
		From:     from,
		Method:   method,
		Value:    value,
		Params:   DumpObject(t, params),
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(testGasLimit),
	})
}

func (h *Harness) AssertBalance(t testing.TB, addr address.Address, amt uint64) {
	t.Helper()

	b, err := h.vm.ActorBalance(addr)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if types.BigCmp(types.NewInt(amt), b) != 0 {
		t.Errorf("expected %s to have balanced of %d. Instead has %s", addr, amt, b)
	}
}

func (h *Harness) AssertBalanceChange(t testing.TB, addr address.Address, amt int64) {
	t.Helper()
	lastBalance, ok := h.lastBalanceCheck[addr]
	if !ok {
		lastBalance, ok = h.HI.Addrs[addr]
		if !ok {
			lastBalance = types.NewInt(0)
		}
	}

	var expected types.BigInt

	if amt >= 0 {
		expected = types.BigAdd(lastBalance, types.NewInt(uint64(amt)))
	} else {
		expected = types.BigSub(lastBalance, types.NewInt(uint64(-amt)))
	}

	h.lastBalanceCheck[addr] = expected

	if gasUsed, ok := h.GasCharges[addr]; ok {
		expected = types.BigSub(expected, gasUsed)
	}

	b, err := h.vm.ActorBalance(addr)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	if types.BigCmp(expected, b) != 0 {
		t.Errorf("expected %s to have balanced of %d. Instead has %s", addr, amt, b)
	}
}

func DumpObject(t testing.TB, obj cbg.CBORMarshaler) []byte {
	if obj == nil {
		return nil
	}
	t.Helper()
	b := new(bytes.Buffer)
	if err := obj.MarshalCBOR(b); err != nil {
		t.Fatalf("dumping params: %+v", err)
	}
	return b.Bytes()
}

type fakeRand struct{}

func (fr *fakeRand) GetRandomness(ctx context.Context, h int64) ([]byte, error) {
	out := make([]byte, 32)
	rand.New(rand.NewSource(h)).Read(out)
	return out, nil
}
