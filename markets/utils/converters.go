package utils

import (
	"bytes"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/filecoin-project/go-address"
	sharedamount "github.com/filecoin-project/go-fil-markets/shared/tokenamount"
	sharedtypes "github.com/filecoin-project/go-fil-markets/shared/types"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func FromSharedTokenAmount(in sharedamount.TokenAmount) types.BigInt {
	return types.BigInt{Int: in.Int}
}

func ToSharedTokenAmount(in abi.TokenAmount) sharedamount.TokenAmount {
	return sharedamount.TokenAmount{Int: in.Int}
}

func ToSharedSignedVoucher(in *types.SignedVoucher) (*sharedtypes.SignedVoucher, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out sharedtypes.SignedVoucher
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func FromSharedSignedVoucher(in *sharedtypes.SignedVoucher) (*types.SignedVoucher, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out types.SignedVoucher
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func ToSharedSignature(in *types.Signature) (*sharedtypes.Signature, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out sharedtypes.Signature
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func FromSharedSignature(in *sharedtypes.Signature) (*types.Signature, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out types.Signature
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func ToSharedStorageAsk(in *types.SignedStorageAsk) (*sharedtypes.SignedStorageAsk, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out sharedtypes.SignedStorageAsk
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func FromSignedStorageAsk(in *sharedtypes.SignedStorageAsk) (*types.SignedStorageAsk, error) {
	var encoded bytes.Buffer
	err := in.MarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	var out types.SignedStorageAsk
	err = out.UnmarshalCBOR(&encoded)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func NewStorageProviderInfo(address address.Address, miner address.Address, sectorSize abi.SectorSize, peer peer.ID) storagemarket.StorageProviderInfo {
	return storagemarket.StorageProviderInfo{
		Address:    address,
		Worker:     miner,
		SectorSize: uint64(sectorSize),
		PeerID:     peer,
	}
}

func FromOnChainDeal(proposal market.DealProposal, state market.DealState) storagemarket.StorageDeal {
	return storagemarket.StorageDeal{
		PieceRef:             proposal.PieceCID.Bytes(),
		PieceSize:            uint64(proposal.PieceSize.Unpadded()),
		Client:               proposal.Client,
		Provider:             proposal.Provider,
		StoragePricePerEpoch: ToSharedTokenAmount(proposal.StoragePricePerEpoch),
		StorageCollateral:    ToSharedTokenAmount(proposal.ProviderCollateral),
		ActivationEpoch:      uint64(state.SectorStartEpoch),
	}
}

func ToSharedBalance(escrow, locked abi.TokenAmount) storagemarket.Balance {
	return storagemarket.Balance{
		Locked:    ToSharedTokenAmount(locked),
		Available: ToSharedTokenAmount(big.Sub(escrow, locked)),
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
