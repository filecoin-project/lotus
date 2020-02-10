package sbmock

import (
	"math"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
)

func PreSeal(ssize abi.SectorSize, maddr address.Address, sectors int) (*genesis.GenesisMiner, error) {
	k, err := wallet.GenerateKey(types.KTBLS)
	if err != nil {
		return nil, err
	}

	genm := &genesis.GenesisMiner{
		Owner:      k.Address,
		Worker:     k.Address,
		SectorSize: ssize,
		Sectors:    make([]*genesis.PreSeal, sectors),
		Key:        k.KeyInfo,
	}

	for i := range genm.Sectors {
		preseal := &genesis.PreSeal{}
		sdata := randB(uint64(abi.PaddedPieceSize(ssize).Unpadded()))

		preseal.CommD = commD(sdata)
		preseal.CommR = commDR(preseal.CommD[:])
		preseal.SectorID = abi.SectorNumber(i + 1)
		preseal.Deal = actors.StorageDealProposal{
			PieceCID:             commcid.PieceCommitmentV1ToCID(preseal.CommD[:]),
			PieceSize:            abi.PaddedPieceSize(ssize),
			Client:               maddr,
			Provider:             maddr,
			StartEpoch:           1,
			EndEpoch:             math.MaxInt64,
			StoragePricePerEpoch: types.NewInt(0),
			ProviderCollateral:   types.NewInt(0),
		}

		genm.Sectors[i] = preseal
	}

	return genm, nil
}
