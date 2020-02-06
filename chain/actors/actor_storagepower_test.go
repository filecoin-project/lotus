package actors_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/lotus/build"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	. "github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"

	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
)

func TestStorageMarketCreateAndSlashMiner(t *testing.T) {
	var ownerAddr, workerAddr address.Address

	opts := []HarnessOpt{
		HarnessAddr(&ownerAddr, 1000000),
		HarnessAddr(&workerAddr, 100000),
	}

	h := NewHarness(t, opts...)

	var minerAddr address.Address
	{
		// cheating the bootstrapping problem
		cheatStorageMarketTotal(t, h.vm, h.cs.Blockstore())

		ret, _ := h.InvokeWithValue(t, ownerAddr, StoragePowerAddress, SPAMethods.CreateStorageMiner,
			types.NewInt(500000),
			&CreateStorageMinerParams{
				Owner:      ownerAddr,
				Worker:     workerAddr,
				SectorSize: build.SectorSizes[0],
				PeerID:     "fakepeerid",
			})
		ApplyOK(t, ret)
		var err error
		minerAddr, err = address.NewFromBytes(ret.Return)
		assert.NoError(t, err)
	}

	{
		ret, _ := h.Invoke(t, ownerAddr, StoragePowerAddress, SPAMethods.IsValidMiner,
			&IsValidMinerParam{Addr: minerAddr})
		ApplyOK(t, ret)

		var output bool
		err := cbor.DecodeInto(ret.Return, &output)
		if err != nil {
			t.Fatalf("error decoding: %+v", err)
		}

		if !output {
			t.Fatalf("%s is miner but IsValidMiner call returned false", minerAddr)
		}
	}

	{
		ret, _ := h.Invoke(t, ownerAddr, StoragePowerAddress, SPAMethods.PowerLookup,
			&PowerLookupParams{Miner: minerAddr})
		ApplyOK(t, ret)
		power := types.BigFromBytes(ret.Return)

		if types.BigCmp(power, types.NewInt(0)) != 0 {
			t.Fatalf("power should be zero, is: %s", power)
		}
	}

	{
		ret, _ := h.Invoke(t, ownerAddr, minerAddr, MAMethods.GetOwner, nil)
		ApplyOK(t, ret)
		oA, err := address.NewFromBytes(ret.Return)
		assert.NoError(t, err)
		assert.Equal(t, ownerAddr, oA, "return from GetOwner should be equal to the owner")
	}

	{
		b1 := fakeBlock(t, minerAddr, 100)
		b2 := fakeBlock(t, minerAddr, 101)

		signBlock(t, h.w, workerAddr, b1)
		signBlock(t, h.w, workerAddr, b2)

		h.BlockHeight = build.ForkBlizzardHeight + 1
		ret, _ := h.Invoke(t, ownerAddr, StoragePowerAddress, SPAMethods.ArbitrateConsensusFault,
			&ArbitrateConsensusFaultParams{
				Block1: b1,
				Block2: b1,
			})
		assert.Equal(t, uint8(3), ret.ExitCode, "should have failed with exit 3")

		ret, _ = h.Invoke(t, ownerAddr, StoragePowerAddress, SPAMethods.ArbitrateConsensusFault,
			&ArbitrateConsensusFaultParams{
				Block1: b1,
				Block2: b2,
			})
		ApplyOK(t, ret)
	}

	{
		ret, _ := h.Invoke(t, ownerAddr, StoragePowerAddress, SPAMethods.PowerLookup,
			&PowerLookupParams{Miner: minerAddr})
		assert.Equal(t, ret.ExitCode, byte(1))
	}

	{
		ret, _ := h.Invoke(t, ownerAddr, StoragePowerAddress, SPAMethods.IsValidMiner, &IsValidMinerParam{minerAddr})
		ApplyOK(t, ret)
		assert.Equal(t, ret.Return, cbg.CborBoolFalse)
	}
}

func cheatStorageMarketTotal(t *testing.T, vm *vm.VM, bs bstore.Blockstore) {
	t.Helper()

	sma, err := vm.StateTree().GetActor(StoragePowerAddress)
	if err != nil {
		t.Fatal(err)
	}

	cst := hamt.CSTFromBstore(bs)

	var smastate StoragePowerState
	if err := cst.Get(context.TODO(), sma.Head, &smastate); err != nil {
		t.Fatal(err)
	}

	smastate.TotalStorage = types.NewInt(10000)

	c, err := cst.Put(context.TODO(), &smastate)
	if err != nil {
		t.Fatal(err)
	}

	sma.Head = c

	if err := vm.StateTree().SetActor(StoragePowerAddress, sma); err != nil {
		t.Fatal(err)
	}
}

func fakeBlock(t *testing.T, minerAddr address.Address, ts uint64) *types.BlockHeader {
	c := fakeCid(t, 1)
	return &types.BlockHeader{Height: 8000, Miner: minerAddr, Timestamp: ts, ParentStateRoot: c, Messages: c, ParentMessageReceipts: c, BLSAggregate: types.Signature{Type: types.KTBLS}}
}

func fakeCid(t *testing.T, s int) cid.Cid {
	t.Helper()
	c, err := cid.NewPrefixV1(cid.Raw, mh.IDENTITY).Sum([]byte(fmt.Sprintf("%d", s)))
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func signBlock(t *testing.T, w *wallet.Wallet, worker address.Address, blk *types.BlockHeader) {
	t.Helper()
	sb, err := blk.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}

	sig, err := w.Sign(context.TODO(), worker, sb)
	if err != nil {
		t.Fatal(err)
	}

	blk.BlockSig = sig
}
