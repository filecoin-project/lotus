package actors_test

import (
	"testing"

	. "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func TestDumpEmpyStruct(t *testing.T) {
	res, err := SerializeParams(struct{}{})
	t.Logf("res: %x, err: %+v", res, err)
}

func TestStorageMarketCreateMiner(t *testing.T) {
	h := NewHarness(t)
	var sminer address.Address
	h.Steps = []Step{
		{
			M: types.Message{
				To:       StorageMarketAddress,
				From:     h.From,
				Method:   SMAMethods.CreateStorageMiner,
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
				sminer, err = address.NewFromBytes(ret.Return)
				if err != nil {
					t.Fatal(err)
				}

				if sminer.String() != "t0103" {
					t.Fatalf("hold up got: %s", sminer)
				}
				h.Steps[1].M.Params = h.DumpObject(&IsMinerParam{Addr: sminer})
				h.Steps[2].M.Params = h.DumpObject(&PowerLookupParams{Miner: sminer})
			},
		},
		{
			M: types.Message{
				To:       StorageMarketAddress,
				From:     h.From,
				Method:   SMAMethods.IsMiner,
				GasPrice: types.NewInt(1),
				GasLimit: types.NewInt(1),
				Value:    types.NewInt(0),
				Nonce:    1,
				// Params is sent in previous set
			},
			Ret: func(t *testing.T, ret *types.MessageReceipt) {
				if ret.ExitCode != 0 {
					t.Fatal("invokation failed: ", ret.ExitCode)
				}
				var output bool
				err := cbor.DecodeInto(ret.Return, &output)
				if err != nil {
					t.Fatalf("error decoding: %+v", err)
				}

				if !output {
					t.Fatalf("%s is miner but IsMiner call returned false", sminer)
				}
			},
		},
		{
			M: types.Message{
				To:       StorageMarketAddress,
				From:     h.From,
				Method:   SMAMethods.PowerLookup,
				GasPrice: types.NewInt(1),
				GasLimit: types.NewInt(1),
				Value:    types.NewInt(0),
				Nonce:    2,
				// Params is sent in previous set
			},
			Ret: func(t *testing.T, ret *types.MessageReceipt) {
				if ret.ExitCode != 0 {
					t.Fatal("invokation failed: ", ret.ExitCode)
				}
				power := types.BigFromBytes(ret.Return)

				if types.BigCmp(power, types.NewInt(0)) != 0 {
					t.Fatalf("power should be zero, is: %s", power)
				}
			},
		},
	}
	state := h.Execute()
	act, err := state.GetActor(sminer)
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
	err = cbor.DecodeInto(hblock.RawData(), smas)
	if err != nil {
		t.Fatal(err)
	}

	if smas.Owner != h.From {
		t.Fatalf("Owner should be %s, but is %s", h.From, smas.Owner)
	}
}
