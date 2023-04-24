package requestvalidation

import (
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
)

// ValidatePush validates a push request received from the peer that will send data
// Will succeed only if:
// - voucher has correct type
// - voucher references an active deal
// - referenced deal matches the given base CID
// - referenced deal is in an acceptable state
func ValidatePush(
	deals PushDeals,
	sender peer.ID,
	voucher datamodel.Node,
	baseCid cid.Cid,
	Selector datamodel.Node) error {

	dealVoucherIface, err := BindnodeRegistry.TypeFromNode(voucher, &StorageDataTransferVoucher{})
	if err != nil {
		return xerrors.Errorf("could not decode StorageDataTransferVoucher: %w", err)
	}
	dealVoucher, _ := dealVoucherIface.(*StorageDataTransferVoucher) // safe to assume type

	deal, err := deals.Get(dealVoucher.Proposal)
	if err != nil {
		return xerrors.Errorf("Proposal CID %s: %w", dealVoucher.Proposal.String(), ErrNoDeal)
	}

	if !deal.Ref.Root.Equals(baseCid) {
		return xerrors.Errorf("Deal Payload CID %s, Data Transfer CID %s: %w", deal.Proposal.PieceCID.String(), baseCid.String(), ErrWrongPiece)
	}
	for _, state := range DataTransferStates {
		if deal.State == state {
			return nil
		}
	}
	return xerrors.Errorf("Deal State %s: %w", storagemarket.DealStates[deal.State], ErrInacceptableDealState)
}

// ValidatePull validates a pull request received from the peer that will receive data
// Will succeed only if:
// - voucher has correct type
// - voucher references an active deal
// - referenced deal matches the given base CID
// - referenced deal is in an acceptable state
func ValidatePull(
	deals PullDeals,
	receiver peer.ID,
	voucher datamodel.Node,
	baseCid cid.Cid,
	Selector datamodel.Node) error {

	dealVoucherIface, err := BindnodeRegistry.TypeFromNode(voucher, &StorageDataTransferVoucher{})
	if err != nil {
		return xerrors.Errorf("could not decode StorageDataTransferVoucher: %w", err)
	}
	dealVoucher, _ := dealVoucherIface.(*StorageDataTransferVoucher) // safe to assume type
	deal, err := deals.Get(dealVoucher.Proposal)
	if err != nil {
		return xerrors.Errorf("Proposal CID %s: %w", dealVoucher.Proposal.String(), ErrNoDeal)
	}

	if !deal.DataRef.Root.Equals(baseCid) {
		return xerrors.Errorf("Deal Payload CID %s, Data Transfer CID %s: %w", deal.Proposal.PieceCID.String(), baseCid.String(), ErrWrongPiece)
	}
	for _, state := range DataTransferStates {
		if deal.State == state {
			return nil
		}
	}
	return xerrors.Errorf("Deal State %s: %w", deal.State, ErrInacceptableDealState)
}
