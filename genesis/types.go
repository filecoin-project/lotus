package genesis

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

type PreSeal struct {
	CommR    [32]byte
	CommD    [32]byte
	SectorID uint64
	Deal     actors.StorageDealProposal
}

type GenesisMiner struct {
	Owner  address.Address
	Worker address.Address

	SectorSize uint64

	Sectors []*PreSeal

	Key types.KeyInfo // TODO: separate file
}
