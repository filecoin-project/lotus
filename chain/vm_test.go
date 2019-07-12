package chain

import (
	"encoding/binary"
	"testing"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/ipfs/go-cid"
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
	cs := &ChainStore{
		bs: bs,
	}

	from := blsaddr(0)
	maddr := blsaddr(1)

	actors := map[address.Address]types.BigInt{
		from:  types.NewInt(1000000),
		maddr: types.NewInt(0),
	}

	var stateroot cid.Cid
	{
		st, err := MakeInitialStateTree(bs, actors)
		if err != nil {
			t.Fatal(err)
		}

		stateroot, err = st.Flush()
		if err != nil {
			t.Fatal(err)
		}

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

	msg := &types.Message{
		To:       InitActorAddress,
		From:     from,
		Method:   1,
		Params:   enc,
		GasPrice: types.NewInt(1),
		GasLimit: types.NewInt(1),
		Value:    types.NewInt(0),
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

	act, err := vm.cstate.GetActor(outaddr)
	if err != nil {
		t.Fatalf("Could not find actor with id %s, err: %s", outaddr, err)
	}
	if act.Code != execparams.Code {
		t.Fatalf("wrong actor code %s instead of %s", act.Code, execparams.Code)
	}

	if outaddr.String() != "t0102" {
		t.Fatal("hold up")
	}
}
