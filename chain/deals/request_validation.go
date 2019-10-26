package deals

import (
	"bytes"
	"context"
	"errors"
	"reflect"

	"github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/datatransfer"
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
	ErrWrongPiece = errors.New("PieceRef for deal does not match piece ref for piece")

	// ErrInacceptableDealState means the deal for this transfer is not in a deal state
	// where transfer can be performed
	ErrInacceptableDealState = errors.New("Deal is not a in a state where deals are accepted")

	// AcceptableDealStates are the states in which it would make sense to actually start a data transfer
	AcceptableDealStates = []api.DealState{api.DealAccepted, api.DealUnknown}
)

type StorageDataTransferVoucher struct {
	Proposal cid.Cid
}

func (dv StorageDataTransferVoucher) ToBytes() []byte {
	return dv.Proposal.Bytes()
}

func (dv StorageDataTransferVoucher) FromBytes(raw []byte) (datatransfer.Voucher, error) {
	c, err := cid.Cast(raw)
	if err != nil {
		return nil, err
	}
	return StorageDataTransferVoucher{c}, nil
}

func (dv StorageDataTransferVoucher) Identifier() string {
	return "StorageDataTransferVoucher"
}

var _ datatransfer.RequestValidator = &ClientRequestValidator{}

type ClientRequestValidator struct {
	deals ClientStateStore
}

func RegisterClientValidator(lc fx.Lifecycle, crv *ClientRequestValidator, dtm datatransfer.Manager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dtm.RegisterVoucherType(reflect.TypeOf(StorageDataTransferVoucher{}), crv)
		},
	})
}

func NewClientRequestValidator(ds dtypes.MetadataDS) *ClientRequestValidator {
	crv := &ClientRequestValidator{
		deals: ClientStateStore{StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))}},
	}
	return crv
}

func (c *ClientRequestValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	PieceRef cid.Cid,
	Selector selector.Selector) error {
	return ErrNoPushAccepted
}

func (c *ClientRequestValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	PieceRef cid.Cid,
	Selector selector.Selector) error {
	dealVoucher, ok := voucher.(StorageDataTransferVoucher)
	if !ok {
		return ErrWrongVoucherType
	}

	var deal ClientDeal
	err := c.deals.MutateClient(dealVoucher.Proposal, func(d *ClientDeal) error {
		deal = *d
		return nil
	})
	if err != nil {
		return ErrNoDeal
	}
	if deal.Miner != receiver {
		return ErrWrongPeer
	}
	if !bytes.Equal(deal.Proposal.PieceRef, PieceRef.Bytes()) {
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

type ProviderRequestValidator struct {
	deals MinerStateStore
}

func RegisterProviderValidator(lc fx.Lifecycle, mrv *ProviderRequestValidator, dtm datatransfer.Manager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return dtm.RegisterVoucherType(reflect.TypeOf(StorageDataTransferVoucher{}), mrv)
		},
	})
}

func NewProviderRequestValidator(ds dtypes.MetadataDS) *ProviderRequestValidator {
	return &ProviderRequestValidator{
		deals: MinerStateStore{StateStore{ds: namespace.Wrap(ds, datastore.NewKey("/deals/client"))}},
	}
}

func (m *ProviderRequestValidator) ValidatePush(
	sender peer.ID,
	voucher datatransfer.Voucher,
	PieceRef cid.Cid,
	Selector selector.Selector) error {
	dealVoucher, ok := voucher.(StorageDataTransferVoucher)
	if !ok {
		return ErrWrongVoucherType
	}

	var deal MinerDeal
	err := m.deals.MutateMiner(dealVoucher.Proposal, func(d *MinerDeal) error {
		deal = *d
		return nil
	})
	if err != nil {
		return ErrNoDeal
	}
	if deal.Client != sender {
		return ErrWrongPeer
	}

	if !bytes.Equal(deal.Proposal.PieceRef, PieceRef.Bytes()) {
		return ErrWrongPiece
	}
	for _, state := range AcceptableDealStates {
		if deal.State == state {
			return nil
		}
	}
	return ErrInacceptableDealState
}

func (m *ProviderRequestValidator) ValidatePull(
	receiver peer.ID,
	voucher datatransfer.Voucher,
	PieceRef cid.Cid,
	Selector selector.Selector) error {
	return ErrNoPullAccepted
}
