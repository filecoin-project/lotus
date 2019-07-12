package chain

import (
	"encoding/binary"
	"testing"

	"github.com/filecoin-project/go-lotus/chain/address"
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

func TestVMInvokeMethod(t *testing.T) {
	bs := bstore.NewBlockstore(dstore.NewMapDatastore())

	from := blsaddr(0)
	maddr := blsaddr(1)

	actors := map[address.Address]BigInt{
		from:  NewInt(1000000),
		maddr: NewInt(0),
	}
	st, err := MakeInitialStateTree(bs, actors)
	if err != nil {
		t.Fatal(err)
	}

	stateroot, err := st.Flush()
	if err != nil {
		t.Fatal(err)
	}

	cs := &ChainStore{
		bs: bs,
	}

	vm, err := NewVM(stateroot, 1, maddr, cs)
	if err != nil {
		t.Fatal(err)
	}

	execparams := &ExecParams{
		Code:   StorageMinerCodeCid,
		Params: []byte("cats"),
	}
	enc, err := cbor.DumpObject(execparams)
	if err != nil {
		t.Fatal(err)
	}

	msg := &Message{
		To:       InitActorAddress,
		From:     from,
		Method:   1,
		Params:   enc,
		GasPrice: NewInt(1),
		GasLimit: NewInt(1),
		Value:    NewInt(0),
	}

	ret, err := vm.ApplyMessage(msg)
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
