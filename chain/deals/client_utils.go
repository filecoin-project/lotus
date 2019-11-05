package deals

import (
	"bytes"
	"context"
	"reflect"
	"runtime"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	files "github.com/ipfs/go-ipfs-files"
	unixfile "github.com/ipfs/go-unixfs/file"
	"github.com/ipld/go-ipld-prime"
	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/lib/statestore"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func (c *Client) failDeal(id cid.Cid, cerr error) {
	if cerr == nil {
		_, f, l, _ := runtime.Caller(1)
		cerr = xerrors.Errorf("unknown error (fail called at %s:%d)", f, l)
	}

	s, ok := c.conns[id]
	if ok {
		_ = s.Reset()
		delete(c.conns, id)
	}

	// TODO: store in some sort of audit log
	log.Errorf("deal %s failed: %s", id, cerr)
}

func (c *Client) dataSize(ctx context.Context, data cid.Cid) (int64, error) {
	root, err := c.dag.Get(ctx, data)
	if err != nil {
		log.Errorf("failed to get file root for deal: %s", err)
		return 0, err
	}

	n, err := unixfile.NewUnixfsFile(ctx, c.dag, root)
	if err != nil {
		log.Errorf("cannot open unixfs file: %s", err)
		return 0, err
	}

	uf, ok := n.(files.File)
	if !ok {
		// TODO: we probably got directory, how should we handle this in unixfs mode?
		return 0, xerrors.New("unsupported unixfs type")
	}

	return uf.Size()
}

func (c *Client) readStorageDealResp(deal ClientDeal) (*Response, error) {
	s, ok := c.conns[deal.ProposalCid]
	if !ok {
		// TODO: Try to re-establish the connection using query protocol
		return nil, xerrors.Errorf("no connection to miner")
	}

	var resp SignedResponse
	if err := cborrpc.ReadCborRPC(s, &resp); err != nil {
		log.Errorw("failed to read Response message", "error", err)
		return nil, err
	}

	// TODO: verify signature

	if resp.Response.Proposal != deal.ProposalCid {
		return nil, xerrors.New("miner responded to a wrong proposal")
	}

	return &resp.Response, nil
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
	dealVoucher, ok := voucher.(*StorageDataTransferVoucher)
	if !ok {
		return xerrors.Errorf("voucher type %s: %w", voucher.Identifier(), ErrWrongVoucherType)
	}

	var deal ClientDeal
	err := c.deals.Mutate(dealVoucher.Proposal, func(d *ClientDeal) error {
		deal = *d
		return nil
	})
	if err != nil {
		return xerrors.Errorf("Proposal CID %s: %w", dealVoucher.Proposal.String(), ErrNoDeal)
	}
	if deal.Miner != receiver {
		return xerrors.Errorf("Deal Peer %s, Data Transfer Peer %s: %w", deal.Miner.String(), receiver.String(), ErrWrongPeer)
	}
	if !bytes.Equal(deal.Proposal.PieceRef, baseCid.Bytes()) {
		return xerrors.Errorf("Deal Payload CID %s, Data Transfer CID %s: %w", string(deal.Proposal.PieceRef), baseCid.String(), ErrWrongPiece)
	}
	for _, state := range AcceptableDealStates {
		if deal.State == state {
			return nil
		}
	}
	return xerrors.Errorf("Deal State %s: %w", deal.State, ErrInacceptableDealState)
}
