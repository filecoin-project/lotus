package actors_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-lotus/build"

	. "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/wallet"

	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
)

func TestStorageMarketCreateAndSlashMiner(t *testing.T) {
	var ownerAddr, workerAddr address.Address

	opts := []HarnessOpt{
		HarnessAddr(&ownerAddr, 100000),
		HarnessAddr(&workerAddr, 100000),
	}

	h := NewHarness(t, opts...)

	var minerAddr address.Address
	{
		ret, _ := h.Invoke(t, ownerAddr, StorageMarketAddress, SMAMethods.CreateStorageMiner,
			&CreateStorageMinerParams{
				Owner:      ownerAddr,
				Worker:     workerAddr,
				SectorSize: types.NewInt(build.SectorSize),
				PeerID:     "fakepeerid",
			})
		ApplyOK(t, ret)
		var err error
		minerAddr, err = address.NewFromBytes(ret.Return)
		assert.NoError(t, err)
	}

	{
		ret, _ := h.Invoke(t, ownerAddr, StorageMarketAddress, SMAMethods.IsMiner,
			&IsMinerParam{Addr: minerAddr})
		ApplyOK(t, ret)

		var output bool
		err := cbor.DecodeInto(ret.Return, &output)
		if err != nil {
			t.Fatalf("error decoding: %+v", err)
		}

		if !output {
			t.Fatalf("%s is miner but IsMiner call returned false", minerAddr)
		}
	}

	{
		ret, _ := h.Invoke(t, ownerAddr, StorageMarketAddress, SMAMethods.PowerLookup,
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

		ret, _ := h.Invoke(t, ownerAddr, StorageMarketAddress, SMAMethods.SlashConsensusFault,
			&SlashConsensusFaultParams{
				Block1: b1,
				Block2: b2,
			})
		ApplyOK(t, ret)
	}
}

func fakeBlock(t *testing.T, minerAddr address.Address, ts uint64) *types.BlockHeader {
	c := fakeCid(t, 1)
	return &types.BlockHeader{Height: 5, Miner: minerAddr, Timestamp: ts, StateRoot: c, Messages: c, MessageReceipts: c, BLSAggregate: types.Signature{Type: types.KTBLS}}
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

	blk.BlockSig = *sig
}
