package actors_test

import (
	"testing"

	"github.com/filecoin-project/go-lotus/build"

	. "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/assert"
)

func TestDumpEmpyStruct(t *testing.T) {
	res, err := SerializeParams(struct{}{})
	t.Logf("res: %x, err: %+v", res, err)
}

func TestStorageMarketCreateMiner(t *testing.T) {
	var ownerAddr, workerAddr address.Address

	opts := []HarnessOpt{
		HarnessAddr(&ownerAddr, 100000),
		HarnessAddr(&workerAddr, 100000),
	}

	h := NewHarness(t, opts...)

	var minerAddr address.Address
	{
		ret, _ := h.Invoke(t, ownerAddr, StorageMarketAddress, SMAMethods.CreateStorageMiner,
			CreateStorageMinerParams{
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
			IsMinerParam{Addr: minerAddr})
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
			PowerLookupParams{Miner: minerAddr})
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
}
