package actors_test

import (
	"context"
	"encoding/binary"
	"testing"

	. "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	dstore "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func blsaddr(n uint64) address.Address {
	buf := make([]byte, 48)
	binary.PutUvarint(buf, n)

	addr, err := address.NewBLSAddress(buf)
	if err != nil {
		panic(err)
	}

	return addr
}

func setupVMTestEnv(t *testing.T) (*vm.VM, []address.Address) {
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

	vm, err := vm.NewVM(stateroot, 1, maddr, cs)
	if err != nil {
		t.Fatal(err)
	}
	return vm, []address.Address{from, maddr}
}

func TestVMInvokeMethod(t *testing.T) {
	vm, addrs := setupVMTestEnv(t)
	from := addrs[0]

	cenc, err := cbor.DumpObject(StorageMinerConstructorParams{})
	if err != nil {
		t.Fatal(err)
	}

	execparams := &ExecParams{
		Code:   StorageMinerCodeCid,
		Params: cenc,
	}
	enc, err := cbor.DumpObject(execparams)
	if err != nil {
		t.Fatal(err)
	}

	msg := &types.Message{
		To:       InitActorAddress,
		From:     from,
		Method:   IAMethods.Exec,
		Params:   enc,
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(1),
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
	vm, addrs := setupVMTestEnv(t)
	from := addrs[0]
	maddr := addrs[1]

	params := &StorageMinerConstructorParams{
		Worker:     maddr,
		SectorSize: types.NewInt(SectorSize),
		PeerID:     "fakepeerid",
	}
	enc, err := cbor.DumpObject(params)
	if err != nil {
		t.Fatal(err)
	}

	msg := &types.Message{
		To:       StorageMarketAddress,
		From:     from,
		Method:   SMAMethods.CreateStorageMiner,
		Params:   enc,
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(1),
		Value:    types.NewInt(0),
	}

	ret, err := vm.ApplyMessage(context.TODO(), msg)
	if err != nil {
		t.Fatal(err)
	}

	if ret.ExitCode != 0 {
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
