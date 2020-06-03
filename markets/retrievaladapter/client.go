package retrievaladapter

import (
	"bytes"
	"context"
	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/specs-actors/actors/abi"
	initactor "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/impl/full"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
	"github.com/filecoin-project/lotus/paychmgr"
)

type retrievalClientNode struct {
	chainapi full.ChainAPI
	pmgr     *paychmgr.Manager
	payapi   payapi.PaychAPI
}

// NewRetrievalClientNode returns a new node adapter for a retrieval client that talks to the
// Lotus Node
func NewRetrievalClientNode(pmgr *paychmgr.Manager, payapi payapi.PaychAPI, chainapi full.ChainAPI) retrievalmarket.RetrievalClientNode {
	return &retrievalClientNode{pmgr: pmgr, payapi: payapi, chainapi: chainapi}
}

// GetOrCreatePaymentChannel sets up a new payment channel if one does not exist
// between a client and a miner and ensures the client has the given amount of
// funds available in the channel.
func (rcn *retrievalClientNode) GetOrCreatePaymentChannel(ctx context.Context, clientAddress address.Address, minerAddress address.Address, clientFundsAvailable abi.TokenAmount, tok shared.TipSetToken) (address.Address, cid.Cid, error) {
	// TODO: respect the provided TipSetToken (a serialized TipSetKey) when
	// querying the chain
	return rcn.pmgr.GetPaych(ctx, clientAddress, minerAddress, clientFundsAvailable)
}

// Allocate late creates a lane within a payment channel so that calls to
// CreatePaymentVoucher will automatically make vouchers only for the difference
// in total
func (rcn *retrievalClientNode) AllocateLane(paymentChannel address.Address) (uint64, error) {
	return rcn.pmgr.AllocateLane(paymentChannel)
}

// CreatePaymentVoucher creates a new payment voucher in the given lane for a
// given payment channel so that all the payment vouchers in the lane add up
// to the given amount (so the payment voucher will be for the difference)
func (rcn *retrievalClientNode) CreatePaymentVoucher(ctx context.Context, paymentChannel address.Address, amount abi.TokenAmount, lane uint64, tok shared.TipSetToken) (*paych.SignedVoucher, error) {
	// TODO: respect the provided TipSetToken (a serialized TipSetKey) when
	// querying the chain
	voucher, err := rcn.payapi.PaychVoucherCreate(ctx, paymentChannel, amount, lane)
	if err != nil {
		return nil, err
	}
	return voucher, nil
}

func (rcn *retrievalClientNode) GetChainHead(ctx context.Context) (shared.TipSetToken, abi.ChainEpoch, error) {
	head, err := rcn.chainapi.ChainHead(ctx)
	if err != nil {
		return nil, 0, err
	}

	return head.Key().Bytes(), head.Height(), nil
}

// WaitForPaymentChannelAddFunds waits messageCID to appear on chain. If it doesn't appear within
// defaultMsgWaitTimeout it returns error
func (rcn *retrievalClientNode) WaitForPaymentChannelAddFunds(messageCID cid.Cid) error {
	_, mr, err := rcn.chainapi.StateManager.WaitForMessage(context.TODO(), messageCID, build.MessageConfidence)

	if err != nil {
		return err
	}
	if mr.ExitCode != exitcode.Ok {
		return xerrors.Errorf("wait for payment channel to add funds failed. exit code: %d", mr.ExitCode)
	}
	return nil
}

func (rcn *retrievalClientNode) WaitForPaymentChannelCreation(messageCID cid.Cid) (address.Address, error) {
	_, mr, err := rcn.chainapi.StateManager.WaitForMessage(context.TODO(), messageCID, build.MessageConfidence)

	if err != nil {
		return address.Undef, err
	}
	if mr.ExitCode != exitcode.Ok {
		return address.Undef, xerrors.Errorf("payment channel creation failed. exit code: %d", mr.ExitCode)
	}
	var retval initactor.ExecReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(mr.Return)); err != nil {
		return address.Undef, err
	}
	return retval.RobustAddress, nil
}
