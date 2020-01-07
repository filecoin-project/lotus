package actors_test

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	hamt "github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbg "github.com/whyrusleeping/cbor-gen"
)

func TestMinerCommitSectors(t *testing.T) {
	var worker, client address.Address
	var minerAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&worker, 1000000),
		HarnessAddr(&client, 1000000),
		HarnessActor(&minerAddr, &worker, actors.StorageMinerCodeCid,
			func() cbg.CBORMarshaler {
				return &actors.StorageMinerConstructorParams{
					Owner:      worker,
					Worker:     worker,
					SectorSize: 1024,
					PeerID:     "fakepeerid",
				}
			}),
	}

	h := NewHarness(t, opts...)
	h.vm.Syscalls.ValidatePoRep = func(ctx context.Context, maddr address.Address, ssize uint64, commD, commR, ticket, proof, seed []byte, sectorID uint64) (bool, aerrors.ActorError) {
		// all proofs are valid
		return true, nil
	}

	ret, _ := h.SendFunds(t, worker, minerAddr, types.NewInt(100000))
	ApplyOK(t, ret)

	ret, _ = h.InvokeWithValue(t, client, actors.StorageMarketAddress, actors.SMAMethods.AddBalance, types.NewInt(2000), nil)
	ApplyOK(t, ret)

	addSectorToMiner(h, t, minerAddr, worker, client, 1)

	assertSectorIDs(h, t, minerAddr, []uint64{1})
}

func TestMinerSubmitBadFault(t *testing.T) {
	var worker, client address.Address
	var minerAddr address.Address
	opts := []HarnessOpt{
		HarnessAddr(&worker, 1000000),
		HarnessAddr(&client, 1000000),
		HarnessActor(&minerAddr, &worker, actors.StorageMinerCodeCid,
			func() cbg.CBORMarshaler {
				return &actors.StorageMinerConstructorParams{
					Owner:      worker,
					Worker:     worker,
					SectorSize: 1024,
					PeerID:     "fakepeerid",
				}
			}),
	}

	h := NewHarness(t, opts...)
	h.vm.Syscalls.ValidatePoRep = func(ctx context.Context, maddr address.Address, ssize uint64, commD, commR, ticket, proof, seed []byte, sectorID uint64) (bool, aerrors.ActorError) {
		// all proofs are valid
		return true, nil
	}

	ret, _ := h.SendFunds(t, worker, minerAddr, types.NewInt(100000))
	ApplyOK(t, ret)

	ret, _ = h.InvokeWithValue(t, client, actors.StorageMarketAddress, actors.SMAMethods.AddBalance, types.NewInt(2000), nil)
	ApplyOK(t, ret)

	addSectorToMiner(h, t, minerAddr, worker, client, 1)

	assertSectorIDs(h, t, minerAddr, []uint64{1})

	bf := types.NewBitField()
	bf.Set(6)
	ret, _ = h.Invoke(t, worker, minerAddr, actors.MAMethods.DeclareFaults, &actors.DeclareFaultsParams{bf})
	ApplyOK(t, ret)

	ret, _ = h.Invoke(t, actors.NetworkAddress, minerAddr, actors.MAMethods.SubmitElectionPoSt, nil)
	ApplyOK(t, ret)

	assertSectorIDs(h, t, minerAddr, []uint64{1})

	badnum := uint64(0)
	badnum--
	bf = types.NewBitField()
	bf.Set(badnum)
	ret, _ = h.Invoke(t, worker, minerAddr, actors.MAMethods.DeclareFaults, &actors.DeclareFaultsParams{bf})
	ApplyOK(t, ret)

	ret, _ = h.Invoke(t, actors.NetworkAddress, minerAddr, actors.MAMethods.SubmitElectionPoSt, nil)
	ApplyOK(t, ret)

	bf = types.NewBitField()
	bf.Set(1)
	ret, _ = h.Invoke(t, worker, minerAddr, actors.MAMethods.DeclareFaults, &actors.DeclareFaultsParams{bf})
	ApplyOK(t, ret)

	ret, _ = h.Invoke(t, actors.NetworkAddress, minerAddr, actors.MAMethods.SubmitElectionPoSt, nil)
	ApplyOK(t, ret)

	assertSectorIDs(h, t, minerAddr, []uint64{})

}

func addSectorToMiner(h *Harness, t *testing.T, minerAddr, worker, client address.Address, sid uint64) {
	t.Helper()
	s := sectorbuilder.UserBytesForSectorSize(1024)
	deal := h.makeFakeDeal(t, minerAddr, worker, client, s)
	ret, _ := h.Invoke(t, worker, actors.StorageMarketAddress, actors.SMAMethods.PublishStorageDeals,
		&actors.PublishStorageDealsParams{
			Deals: []actors.StorageDealProposal{*deal},
		})
	ApplyOK(t, ret)
	var dealIds actors.PublishStorageDealResponse
	if err := dealIds.UnmarshalCBOR(bytes.NewReader(ret.Return)); err != nil {
		t.Fatal(err)
	}

	dealid := dealIds.DealIDs[0]

	ret, _ = h.Invoke(t, worker, minerAddr, actors.MAMethods.PreCommitSector,
		&actors.SectorPreCommitInfo{
			SectorNumber: sid,
			CommR:        []byte("cats"),
			SealEpoch:    10,
			DealIDs:      []uint64{dealid},
		})
	ApplyOK(t, ret)

	h.BlockHeight += 100
	ret, _ = h.Invoke(t, worker, minerAddr, actors.MAMethods.ProveCommitSector,
		&actors.SectorProveCommitInfo{
			Proof:    []byte("prooofy"),
			SectorID: sid,
			DealIDs:  []uint64{dealid}, // TODO: weird that i have to pass this again
		})
	ApplyOK(t, ret)
}

func assertSectorIDs(h *Harness, t *testing.T, maddr address.Address, ids []uint64) {
	t.Helper()
	sectors, err := getMinerSectorSet(context.TODO(), h.vm.StateTree(), h.bs, maddr)
	if err != nil {
		t.Fatal(err)
	}

	if len(sectors) != len(ids) {
		t.Fatal("miner has wrong number of sectors in their sector set")
	}

	all := make(map[uint64]bool)
	for _, s := range sectors {
		all[s.SectorID] = true
	}

	for _, id := range ids {
		if !all[id] {
			t.Fatal("expected to find sector ID: ", id)
		}
	}
}

func getMinerSectorSet(ctx context.Context, st types.StateTree, bs blockstore.Blockstore, maddr address.Address) ([]*api.ChainSectorInfo, error) {
	mact, err := st.GetActor(maddr)
	if err != nil {
		return nil, err
	}

	cst := hamt.CSTFromBstore(bs)

	var mstate actors.StorageMinerActorState
	if err := cst.Get(ctx, mact.Head, &mstate); err != nil {
		return nil, err
	}

	return stmgr.LoadSectorsFromSet(ctx, bs, mstate.Sectors)
}

func (h *Harness) makeFakeDeal(t *testing.T, miner, worker, client address.Address, size uint64) *actors.StorageDealProposal {
	data := make([]byte, size)
	rand.Read(data)
	commP, err := sectorbuilder.GeneratePieceCommitment(bytes.NewReader(data), size)
	if err != nil {
		t.Fatal(err)
	}

	prop := actors.StorageDealProposal{
		PieceRef:  commP[:],
		PieceSize: size,
		//PieceSerialization SerializationMode // Needs to be here as it tells how data in the sector maps to PieceRef cid

		Client:   client,
		Provider: miner,

		ProposalExpiration: 10000,
		Duration:           150,

		StoragePricePerEpoch: types.NewInt(1),
		StorageCollateral:    types.NewInt(0),
	}

	if err := api.SignWith(context.TODO(), h.w.Sign, client, &prop); err != nil {
		t.Fatal(err)
	}

	return &prop
}
