package drivers

import (
	"encoding/binary"
	"testing"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/oni/tvx/chain/types"
)

type MockSectorBuilder struct {
	t         testing.TB
	sectorSeq uint64

	// PreSeal is intexted by sectorID
	MinerSectors map[address.Address][]*types.PreSeal
}

func NewMockSectorBuilder(t testing.TB) *MockSectorBuilder {
	return &MockSectorBuilder{
		t:            t,
		sectorSeq:    0,
		MinerSectors: make(map[address.Address][]*types.PreSeal),
	}
}

func (msb *MockSectorBuilder) NewPreSealedSector(miner, client address.Address, pt abi.RegisteredProof, ssize abi.SectorSize, start, end abi.ChainEpoch) *types.PreSeal {
	minerSectors := msb.MinerSectors[miner]
	sectorID := len(minerSectors)

	token := make([]byte, 32)
	binary.PutUvarint(token, msb.sectorSeq)
	// the only error we could get is if token isn't 32 long, ignore
	D, _ := commcid.DataCommitmentV1ToCID(token)
	R, _ := commcid.ReplicaCommitmentV1ToCID(token)
	msb.sectorSeq++

	preseal := &types.PreSeal{
		CommR:    R,
		CommD:    D,
		SectorID: abi.SectorNumber(sectorID),
		Deal: market.DealProposal{
			PieceCID:   D,
			PieceSize:  abi.PaddedPieceSize(ssize),
			Client:     client,
			Provider:   miner,
			StartEpoch: start,
			EndEpoch:   end,
			// TODO how do we want to interact with these values?
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		},
		ProofType: pt,
	}

	msb.MinerSectors[miner] = append(msb.MinerSectors[miner], preseal)
	return preseal
}
