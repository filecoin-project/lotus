package sbmock

import (
	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
)

func PreSeal(ssize abi.SectorSize, maddr address.Address, sectors int) (*genesis.Miner, *types.KeyInfo, error) {
	k, err := wallet.GenerateKey(crypto.SigTypeBLS)
	if err != nil {
		return nil, nil, err
	}

	genm := &genesis.Miner{
		Owner:         k.Address,
		Worker:        k.Address,
		MarketBalance: big.NewInt(0),
		PowerBalance:  big.NewInt(0),
		SectorSize:    ssize,
		Sectors:       make([]*genesis.PreSeal, sectors),
	}

	for i := range genm.Sectors {
		preseal := &genesis.PreSeal{}
		sdata := randB(uint64(abi.PaddedPieceSize(ssize).Unpadded()))

		preseal.CommD = commD(sdata)
		preseal.CommR = commDR(preseal.CommD[:])
		preseal.SectorID = abi.SectorNumber(i + 1)
		preseal.Deal = market.DealProposal{
			PieceCID:             commcid.PieceCommitmentV1ToCID(preseal.CommD[:]),
			PieceSize:            abi.PaddedPieceSize(ssize),
			Client:               maddr,
			Provider:             maddr,
			StartEpoch:           1,
			EndEpoch:             10000,
			StoragePricePerEpoch: big.Zero(),
			ProviderCollateral:   big.Zero(),
			ClientCollateral:     big.Zero(),
		}

		genm.Sectors[i] = preseal
	}

	return genm, &k.KeyInfo, nil
}
