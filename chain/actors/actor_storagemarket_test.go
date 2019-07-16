package actors_test

import (
	"testing"

	. "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func TestStorageMarketCreateMiner(t *testing.T) {
	h := NewHarness(t)
	var outaddr address.Address
	h.Steps = []Step{
		{
			M: types.Message{
				To:       StorageMarketAddress,
				From:     h.From,
				Method:   1,
				GasPrice: types.NewInt(1),
				GasLimit: types.NewInt(1),
				Value:    types.NewInt(0),
				Params: h.DumpObject(&CreateStorageMinerParams{
					Owner:      h.From,
					Worker:     h.Third,
					SectorSize: types.NewInt(SectorSize),
					PeerID:     "fakepeerid",
				}),
			},
			Ret: func(t *testing.T, ret *types.MessageReceipt) {
				if ret.ExitCode != 0 {
					t.Fatal("invokation failed: ", ret.ExitCode)
				}

				var err error
				outaddr, err = address.NewFromBytes(ret.Return)
				if err != nil {
					t.Fatal(err)
				}

				if outaddr.String() != "t0102" {
					t.Fatal("hold up")
				}
			},
		},
	}
	state := h.Execute()
	act, err := state.GetActor(outaddr)
	if err != nil {
		t.Fatal(err)
	}
	if act.Code != StorageMinerCodeCid {
		t.Fatalf("Expected correct code, got %s, instead of %s", act.Code, StorageMinerCodeCid)
	}
	hblock, err := h.bs.Get(act.Head)
	if err != nil {
		t.Fatal(err)
	}

	smas := &StorageMinerActorState{}
	cbor.DecodeInto(hblock.RawData(), smas)
	if smas.Owner != h.From {
		t.Fatalf("Owner should be %s, but is %s", h.From, smas.Owner)
	}
}
