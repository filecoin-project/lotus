package genesis

import "github.com/filecoin-project/lotus/chain/address"

type PreSeal struct {
	CommR    [32]byte
	CommD    [32]byte
	SectorID uint64
}

type GenesisMiner struct {
	Sectors []PreSeal
	Owner   address.Address
	Worker  address.Address
}
