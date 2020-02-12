package utils

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

func NewStorageProviderInfo(address address.Address, miner address.Address, sectorSize abi.SectorSize, peer peer.ID) storagemarket.StorageProviderInfo {
	return storagemarket.StorageProviderInfo{
		Address:    address,
		Worker:     miner,
		SectorSize: uint64(sectorSize),
		PeerID:     peer,
	}
}
