package actors_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/go-address"
	. "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	dstore "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
)

func blsaddr(n uint64) address.Address {
	buf := make([]byte, 48)
	binary.PutUvarint(buf, n)

	addr, err := address.NewBLSAddress(buf)
	if err != nil {
		panic(err) // ok
	}

	return addr
}

func setupVMTestEnv(t *testing.T) (*vm.VM, []address.Address, bstore.Blockstore) {
	bs := bstore.NewBlockstore(dstore.NewMapDatastore())

	from := blsaddr(0)
	maddr := blsaddr(1)

	actors := map[address.Address]types.BigInt{
		from:  types.NewInt(1000000),
		maddr: types.NewInt(0),
	}
	st, err := gen.MakeInitialStateTree(bs, actors)
	if err != nil {
		t.Fatal(err)
	}

	stateroot, err := st.Flush()
	if err != nil {
		t.Fatal(err)
	}

	cs := store.NewChainStore(bs, nil)

	// TODO: should probabaly mock out the randomness bit, nil works for now
	vm, err := vm.NewVM(stateroot, 1, nil, maddr, cs.Blockstore())
	if err != nil {
		t.Fatal(err)
	}
	return vm, []address.Address{from, maddr}, bs
}

func TestVMInvokeMethod(t *testing.T) {
	vm, addrs, _ := setupVMTestEnv(t)
	from := addrs[0]

	var err error
	cenc, err := SerializeParams(&StorageMinerConstructorParams{Owner: from, Worker: from})
	if err != nil {
		t.Fatal(err)
	}

	execparams := &ExecParams{
		Code:   StorageMinerCodeCid,
		Params: cenc,
	}
	enc, err := SerializeParams(execparams)
	if err != nil {
		t.Fatal(err)
	}

	msg := &types.Message{
		To:       InitAddress,
		From:     from,
		Method:   IAMethods.Exec,
		Params:   enc,
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(10000),
		Value:    types.NewInt(0),
	}

	ret, err := vm.ApplyMessage(context.TODO(), msg)
	if err != nil {
		t.Fatal(err)
	}

	if ret.ExitCode != 0 {
		t.Fatal("invocation failed")
	}

	outaddr, err := address.NewFromBytes(ret.Return)
	if err != nil {
		t.Fatal(err)
	}

	if outaddr.String() != "t0102" {
		t.Fatal("hold up")
	}
}

func TestStorageMarketActorCreateMiner(t *testing.T) {
	vm, addrs, bs := setupVMTestEnv(t)
	from := addrs[0]
	maddr := addrs[1]

	cheatStorageMarketTotal(t, vm, bs)

	params := &StorageMinerConstructorParams{
		Owner:      maddr,
		Worker:     maddr,
		SectorSize: build.SectorSizes[0],
		PeerID:     "fakepeerid",
	}
	var err error
	enc, err := SerializeParams(params)
	if err != nil {
		t.Fatal(err)
	}

	msg := &types.Message{
		To:       StoragePowerAddress,
		From:     from,
		Method:   SPAMethods.CreateStorageMiner,
		Params:   enc,
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(10000),
		Value:    types.NewInt(50000),
	}

	ret, err := vm.ApplyMessage(context.TODO(), msg)
	if err != nil {
		t.Fatal(err)
	}

	if ret.ExitCode != 0 {
		fmt.Println(ret.ActorErr)
		t.Fatal("invocation failed: ", ret.ExitCode)
	}

	outaddr, err := address.NewFromBytes(ret.Return)
	if err != nil {
		t.Fatal(err)
	}

	if outaddr.String() != "t0102" {
		t.Fatal("hold up")
	}
}
