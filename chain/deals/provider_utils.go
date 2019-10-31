package deals

import (
	"bytes"
	"context"
	"reflect"
	"runtime"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/datatransfer"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/ipld/go-ipld-prime"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborrpc"
	"github.com/filecoin-project/lotus/lib/statestore"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"
)

func (p *Provider) failDeal(id cid.Cid, cerr error) {
	if err := p.deals.End(id); err != nil {
		log.Warnf("deals.End: %s", err)
	}

	if cerr == nil {
		_, f, l, _ := runtime.Caller(1)
		cerr = xerrors.Errorf("unknown error (fail called at %s:%d)", f, l)
	}

	log.Errorf("deal %s failed: %s", id, cerr)

	err := p.sendSignedResponse(&Response{
		State:    api.DealFailed,
		Message:  cerr.Error(),
		Proposal: id,
	})

	s, ok := p.conns[id]
	if ok {
		_ = s.Reset()
		delete(p.conns, id)
	}

	if err != nil {
		log.Warnf("notifying client about deal failure: %s", err)
	}
}

func (p *Provider) readProposal(s inet.Stream) (proposal actors.StorageDealProposal, err error) {
	if err := cborrpc.ReadCborRPC(s, &proposal); err != nil {
		log.Errorw("failed to read proposal message", "error", err)
		return proposal, err
	}

	if err := proposal.Verify(); err != nil {
		return proposal, xerrors.Errorf("verifying StorageDealProposal: %w", err)
	}

	// TODO: Validate proposal maybe
	// (and signature, obviously)

	if proposal.Provider != p.actor {
		log.Errorf("proposal with wrong ProviderAddress: %s", proposal.Provider)
		return proposal, err
	}

	return
}

func (p *Provider) sendSignedResponse(resp *Response) error {
	s, ok := p.conns[resp.Proposal]
	if !ok {
		return xerrors.New("couldn't send response: not connected")
	}

	msg, err := cborrpc.Dump(resp)
	if err != nil {
		return xerrors.Errorf("serializing response: %w", err)
	}

	worker, err := p.getWorker(p.actor)
	if err != nil {
		return err
	}

	sig, err := p.full.WalletSign(context.TODO(), worker, msg)
	if err != nil {
		return xerrors.Errorf("failed to sign response message: %w", err)
	}

	signedResponse := &SignedResponse{
		Response:  *resp,
		Signature: sig,
	}

	err = cborrpc.WriteCborRPC(s, signedResponse)
	if err != nil {
		// Assume client disconnected
		s.Close()
		delete(p.conns, resp.Proposal)
	}
	return err
}

func (p *Provider) getWorker(miner address.Address) (address.Address, error) {
	getworker := &types.Message{
		To:     miner,
		From:   miner,
		Method: actors.MAMethods.GetWorkerAddr,
	}
	r, err := p.full.StateCall(context.TODO(), getworker, nil)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting worker address: %w", err)
	}

	if r.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("getWorker call failed: %d", r.ExitCode)
	}

	return address.NewFromBytes(r.Return)
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
	dealVoucher, ok := voucher.(*StorageDataTransferVoucher)
	if !ok {
		return xerrors.Errorf("voucher type %s: %w", voucher.Identifier(), ErrWrongVoucherType)
	}

	var deal MinerDeal
	err := m.deals.Mutate(dealVoucher.Proposal, func(d *MinerDeal) error {
		deal = *d
		return nil
	})
	if err != nil {
		return xerrors.Errorf("Proposal CID %s: %w", dealVoucher.Proposal.String(), ErrNoDeal)
	}
	if deal.Client != sender {
		return xerrors.Errorf("Deal Peer %s, Data Transfer Peer %s: %w", deal.Client.String(), sender.String(), ErrWrongPeer)
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
