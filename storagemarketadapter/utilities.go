package storagemarketadapter

import (
	"bytes"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/lib/sharedutils"
	peer "github.com/libp2p/go-libp2p-peer"
)

func NewStorageProviderInfo(address address.Address, miner address.Address, sectorSize uint64, peer peer.ID) storagemarket.StorageProviderInfo {
	return storagemarket.StorageProviderInfo{
		Address:    address,
		Worker:     miner,
		SectorSize: sectorSize,
		PeerID:     peer,
	}
}

func FromOnChainDeal(deal actors.OnChainDeal) storagemarket.StorageDeal {
	return storagemarket.StorageDeal{
		PieceRef:             deal.PieceRef,
		PieceSize:            deal.PieceSize,
		Client:               deal.Client,
		Provider:             deal.Provider,
		StoragePricePerEpoch: sharedutils.ToSharedTokenAmount(deal.StoragePricePerEpoch),
		StorageCollateral:    sharedutils.ToSharedTokenAmount(deal.StorageCollateral),
		ActivationEpoch:      deal.ActivationEpoch,
	}
}

func ToOnChainDeal(deal storagemarket.StorageDeal) actors.OnChainDeal {
	return actors.OnChainDeal{
		PieceRef:             deal.PieceRef,
		PieceSize:            deal.PieceSize,
		Client:               deal.Client,
		Provider:             deal.Provider,
		StoragePricePerEpoch: sharedutils.FromSharedTokenAmount(deal.StoragePricePerEpoch),
		StorageCollateral:    sharedutils.FromSharedTokenAmount(deal.StorageCollateral),
		ActivationEpoch:      deal.ActivationEpoch,
	}
}

func ToSharedBalance(balance actors.StorageParticipantBalance) storagemarket.Balance {
	return storagemarket.Balance{
		Locked:    sharedutils.ToSharedTokenAmount(balance.Locked),
		Available: sharedutils.ToSharedTokenAmount(balance.Available),
	}
}

func ToSharedStorageDealProposal(proposal *actors.StorageDealProposal) (*storagemarket.StorageDealProposal, error) {
	var encoded bytes.Buffer
	err := proposal.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out storagemarket.StorageDealProposal
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func FromSharedStorageDealProposal(proposal *storagemarket.StorageDealProposal) (*actors.StorageDealProposal, error) {
	var encoded bytes.Buffer
	err := proposal.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out actors.StorageDealProposal
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
