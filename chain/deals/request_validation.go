package deals

import (
	"bytes"
	"context"
	"errors"
	"reflect"

	"github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/lib/statestore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var (
	// ErrWrongVoucherType means the voucher was not the correct type can validate against
	ErrWrongVoucherType = errors.New("cannot validate voucher type")

	// ErrNoPushAccepted just means clients do not accept pushes for storage deals
	ErrNoPushAccepted = errors.New("Client should not receive data for a storage deal")

	// ErrNoPullAccepted just means providers do not accept pulls for storage deals
	ErrNoPullAccepted = errors.New("Provider should not send data for a storage deal")

	// ErrNoDeal means no active deal was found for this vouchers proposal cid
	ErrNoDeal = errors.New("No deal found for this proposal")

	// ErrWrongPeer means that the other peer for this data transfer request does not match
	// the other peer for the deal
	ErrWrongPeer = errors.New("Data Transfer peer id and Deal peer id do not match")

	// ErrWrongPiece means that the pieceref for this data transfer request does not match
	// the one specified in the deal
	ErrWrongPiece = errors.New("Base CID for deal does not match CID for piece")

	// ErrInacceptableDealState means the deal for this transfer is not in a deal state
	// where transfer can be performed
	ErrInacceptableDealState = errors.New("Deal is not a in a state where deals are accepted")

	// AcceptableDealStates are the states in which it would make sense to actually start a data transfer
	AcceptableDealStates = []api.DealState{api.DealAccepted, api.DealUnknown}
)

// StorageDataTransferVoucher is the voucher type for data transfers
// used by the storage market
type StorageDataTransferVoucher struct {
	Proposal cid.Cid
}

// ToBytes converts the StorageDataTransferVoucher to raw bytes
func (dv StorageDataTransferVoucher) ToBytes() []byte {
	return dv.Proposal.Bytes()
}

// FromBytes converts the StorageDataTransferVoucher to raw bytes
func (dv StorageDataTransferVoucher) FromBytes(raw []byte) (datatransfer.Voucher, error) {
	c, err := cid.Cast(raw)
	if err != nil {
		return nil, err
	}
	return StorageDataTransferVoucher{c}, nil
}

// Identifier is the unique string identifier for a StorageDataTransferVoucher
func (dv StorageDataTransferVoucher) Identifier() string {
	return "StorageDataTransferVoucher"
}

var _ datatransfer.RequestValidator = &ClientRequestValidator{}

// ClientRequestValidator validates data transfer requests for the client
// in a storage market
type ClientRequestValidator struct {
	deals *statestore.StateStore
}

// RegisterClientValidator is an initialization hook that registers the client
// request validator with the data transfer module as the validator for
// StorageDataTransferVoucher types
func RegisterClientValidator(lc fx.Lifecycle, crv *ClientRequestValidator, dtm datatransfer.ClientDataTransfer) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dtm.RegisterVoucherType(reflect.TypeOf(StorageDataTransferVoucher{}), crv)
		},
	})
}

// NewClientRequestValidator returns a new client request validator for the
// given datastore
func NewClientRequestValidator(ds dtypes.MetadataDS) *ClientRequestValidator {
	crv := &ClientRequestValidator{
		deals: statestore.New(namespace.Wrap(ds, datastore.NewKey("/deals/client"))),
	}
	return crv
}

// ValidatePush validates a push request received from the peer that will send data
// Will always error because clients should not accept push requests from a provider
// in a storage deal (i.e. send data to client).
func (c *ClientRequestValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	return ErrNoPushAccepted
}

// ValidatePull validates a pull request received from the peer that will receive data
// Will succeed only if:
// - voucher has correct type
// - voucher references an active deal
// - referenced deal matches the receiver (miner)
// - referenced deal matches the given base CID
// - referenced deal is in an acceptable state
func (c *ClientRequestValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	dealVoucher, ok := voucher.(StorageDataTransferVoucher)
	if !ok {
		return ErrWrongVoucherType
	}

	var deal ClientDeal
	err := c.deals.Mutate(dealVoucher.Proposal, func(d *ClientDeal) error {
		deal = *d
		return nil
	})
	if err != nil {
		return ErrNoDeal
	}
	if deal.Miner != receiver {
		return ErrWrongPeer
	}
	if !bytes.Equal(deal.Proposal.PieceRef, baseCid.Bytes()) {
		return ErrWrongPiece
	}
	for _, state := range AcceptableDealStates {
		if deal.State == state {
			return nil
		}
	}
	return ErrInacceptableDealState
}

var _ datatransfer.RequestValidator = &ProviderRequestValidator{}

// ProviderRequestValidator validates data transfer requests for the provider
// in a storage market
type ProviderRequestValidator struct {
	deals *statestore.StateStore
}

// RegisterProviderValidator is an initialization hook that registers the provider
// request validator with the data transfer module as the validator for
// StorageDataTransferVoucher types
func RegisterProviderValidator(lc fx.Lifecycle, mrv *ProviderRequestValidator, dtm datatransfer.ProviderDataTransfer) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dtm.RegisterVoucherType(reflect.TypeOf(StorageDataTransferVoucher{}), mrv)
		},
	})
}

// NewProviderRequestValidator returns a new client request validator for the
// given datastore
func NewProviderRequestValidator(ds dtypes.MetadataDS) *ProviderRequestValidator {
	return &ProviderRequestValidator{
		deals: statestore.New(namespace.Wrap(ds, datastore.NewKey("/deals/client"))),
	}
}

// ValidatePush validates a push request received from the peer that will send data
// Will succeed only if:
// - voucher has correct type
// - voucher references an active deal
// - referenced deal matches the client
// - referenced deal matches the given base CID
// - referenced deal is in an acceptable state
func (m *ProviderRequestValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	dealVoucher, ok := voucher.(StorageDataTransferVoucher)
	if !ok {
		return ErrWrongVoucherType
	}

	var deal MinerDeal
	err := m.deals.Mutate(dealVoucher.Proposal, func(d *MinerDeal) error {
		deal = *d
		return nil
	})
	if err != nil {
		return ErrNoDeal
	}
	if deal.Client != sender {
		return ErrWrongPeer
	}

	if !bytes.Equal(deal.Proposal.PieceRef, baseCid.Bytes()) {
		return ErrWrongPiece
	}
	for _, state := range AcceptableDealStates {
		if deal.State == state {
			return nil
		}
	}
	return ErrInacceptableDealState
}

// ValidatePull validates a pull request received from the peer that will receive data.
// Will always error because providers should not accept pull requests from a client
// in a storage deal (i.e. send data to client).
func (m *ProviderRequestValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	baseCid cid.Cid,
	Selector ipld.Node) error {
	return ErrNoPullAccepted
}
