package settler

import (
	"context"
	"sync"

	"github.com/filecoin-project/lotus/paychmgr"

	"go.uber.org/fx"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
	payapi "github.com/filecoin-project/lotus/node/impl/paych"
)

var log = logging.Logger("payment-channel-settler")

// API are the dependencies need to run the payment channel settler
type API struct {
	fx.In

	full.ChainAPI
	full.StateAPI
	payapi.PaychAPI
}

type settlerAPI interface {
	PaychList(context.Context) ([]address.Address, error)
	PaychStatus(context.Context, address.Address) (*api.PaychStatus, error)
	PaychVoucherCheckSpendable(context.Context, address.Address, *paych.SignedVoucher, []byte, []byte) (bool, error)
	PaychVoucherList(context.Context, address.Address) ([]*paych.SignedVoucher, error)
	PaychVoucherSubmit(context.Context, address.Address, *paych.SignedVoucher, []byte, []byte) (cid.Cid, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64) (*api.MsgLookup, error)
}

type paymentChannelSettler struct {
	ctx context.Context
	api settlerAPI
}

// SettlePaymentChannels checks the chain for events related to payment channels settling and
// submits any vouchers for inbound channels tracked for this node
func SettlePaymentChannels(lc fx.Lifecycle, api API) error {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			pcs := newPaymentChannelSettler(ctx, &api)
			ev := events.NewEvents(ctx, &api)
			return ev.Called(pcs.check, pcs.messageHandler, pcs.revertHandler, int(build.MessageConfidence+1), events.NoTimeout, pcs.matcher)
		},
	})
	return nil
}

func newPaymentChannelSettler(ctx context.Context, api settlerAPI) *paymentChannelSettler {
	return &paymentChannelSettler{
		ctx: ctx,
		api: api,
	}
}

func (pcs *paymentChannelSettler) check(ts *types.TipSet) (done bool, more bool, err error) {
	return false, true, nil
}

func (pcs *paymentChannelSettler) messageHandler(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (more bool, err error) {
	bestByLane, err := paychmgr.BestSpendableByLane(pcs.ctx, pcs.api, msg.To)
	if err != nil {
		return true, err
	}
	var wg sync.WaitGroup
	wg.Add(len(bestByLane))
	for _, voucher := range bestByLane {
		submitMessageCID, err := pcs.api.PaychVoucherSubmit(pcs.ctx, msg.To, voucher, nil, nil)
		if err != nil {
			return true, err
		}
		go func(voucher *paych.SignedVoucher, submitMessageCID cid.Cid) {
			defer wg.Done()
			msgLookup, err := pcs.api.StateWaitMsg(pcs.ctx, submitMessageCID, build.MessageConfidence)
			if err != nil {
				log.Errorf("submitting voucher: %s", err.Error())
			}
			if msgLookup.Receipt.ExitCode != 0 {
				log.Errorf("failed submitting voucher: %+v", voucher)
			}
		}(voucher, submitMessageCID)
	}
	wg.Wait()
	return true, nil
}

func (pcs *paymentChannelSettler) revertHandler(ctx context.Context, ts *types.TipSet) error {
	return nil
}

func (pcs *paymentChannelSettler) matcher(msg *types.Message) (matchOnce bool, matched bool, err error) {
	// Check if this is a settle payment channel message
	if msg.Method != builtin0.MethodsPaych.Settle {
		return false, false, nil
	}
	// Check if this payment channel is of concern to this node (i.e. tracked in payment channel store),
	// and its inbound (i.e. we're getting vouchers that we may need to redeem)
	trackedAddresses, err := pcs.api.PaychList(pcs.ctx)
	if err != nil {
		return false, false, err
	}
	for _, addr := range trackedAddresses {
		if msg.To == addr {
			status, err := pcs.api.PaychStatus(pcs.ctx, addr)
			if err != nil {
				return false, false, err
			}
			if status.Direction == api.PCHInbound {
				return false, true, nil
			}
		}
	}
	return false, false, nil
}
