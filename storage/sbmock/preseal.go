package sbmock

import (
	"math"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
)

func PreSeal(ssize uint64, maddr address.Address, sectors int) (*genesis.GenesisMiner, error) {
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
		sdata := randB(sectorbuilder.UserBytesForSectorSize(ssize))

		preseal.CommD = commD(sdata)
		preseal.CommR = commDR(preseal.CommD[:])
		preseal.SectorID = uint64(i + 1)
		preseal.Deal = actors.StorageDealProposal{
			PieceRef:             preseal.CommD[:],
			PieceSize:            sectorbuilder.UserBytesForSectorSize(ssize),
			Client:               maddr,
			Provider:             maddr,
			ProposalExpiration:   math.MaxUint64,
			Duration:             math.MaxUint64,
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil,
		}

		genm.Sectors[i] = preseal
	}

	return genm, nil
}
