package storageadapter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	market0 "github.com/filecoin-project/specs-actors/actors/builtin/market"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage"
)

type dealPublisherAPI interface {
	ChainHead(context.Context) (*types.TipSet, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error)
	StateMinerInfo(context.Context, address.Address, types.TipSetKey) (miner.MinerInfo, error)

	WalletBalance(context.Context, address.Address) (types.BigInt, error)
	WalletHas(context.Context, address.Address) (bool, error)
	StateAccountKey(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateLookupID(context.Context, address.Address, types.TipSetKey) (address.Address, error)
	StateCall(context.Context, *types.Message, types.TipSetKey) (*api.InvocResult, error)
}

// DealPublisher batches deal publishing so that many deals can be included in
// a single publish message. This saves gas for miners that publish deals
// frequently.
// When a deal is submitted, the DealPublisher waits a configurable amount of
// time for other deals to be submitted before sending the publish message.
// There is a configurable maximum number of deals that can be included in one
// message. When the limit is reached the DealPublisher immediately submits a
// publish message with all deals in the queue.
type DealPublisher struct {
	api dealPublisherAPI
	as  *storage.AddressSelector

	ctx      context.Context
	Shutdown context.CancelFunc

	maxDealsPerPublishMsg uint64
	publishPeriod         time.Duration
	publishSpec           *api.MessageSendSpec

	lk                      sync.Mutex
	pending                 []*pendingDeal
	cancelWaitForMoreDeals  context.CancelFunc
	publishPeriodStart      time.Time
	startEpochSealingBuffer abi.ChainEpoch
}

// A deal that is queued to be published
type pendingDeal struct {
	ctx    context.Context
	deal   market2.ClientDealProposal
	Result chan publishResult
}

// The result of publishing a deal
type publishResult struct {
	msgCid cid.Cid
	err    error
}

func newPendingDeal(ctx context.Context, deal market2.ClientDealProposal) *pendingDeal {
	return &pendingDeal{
		ctx:    ctx,
		deal:   deal,
		Result: make(chan publishResult),
	}
}

type PublishMsgConfig struct {
	// The amount of time to wait for more deals to arrive before
	// publishing
	Period time.Duration
	// The maximum number of deals to include in a single PublishStorageDeals
	// message
	MaxDealsPerMsg uint64
	// Minimum start epoch buffer to give time for sealing of sector with deal
	StartEpochSealingBuffer uint64
}

func NewDealPublisher(
	feeConfig *config.MinerFeeConfig,
	publishMsgCfg PublishMsgConfig,
) func(lc fx.Lifecycle, full api.FullNode, as *storage.AddressSelector) *DealPublisher {
	return func(lc fx.Lifecycle, full api.FullNode, as *storage.AddressSelector) *DealPublisher {
		maxFee := abi.NewTokenAmount(0)
		if feeConfig != nil {
			maxFee = abi.TokenAmount(feeConfig.MaxPublishDealsFee)
		}
		publishSpec := &api.MessageSendSpec{MaxFee: maxFee}
		dp := newDealPublisher(full, as, publishMsgCfg, publishSpec)
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				dp.Shutdown()
				return nil
			},
		})
		return dp
	}
}

func newDealPublisher(
	dpapi dealPublisherAPI,
	as *storage.AddressSelector,
	publishMsgCfg PublishMsgConfig,
	publishSpec *api.MessageSendSpec,
) *DealPublisher {
	ctx, cancel := context.WithCancel(context.Background())
	return &DealPublisher{
		api:                     dpapi,
		as:                      as,
		ctx:                     ctx,
		Shutdown:                cancel,
		maxDealsPerPublishMsg:   publishMsgCfg.MaxDealsPerMsg,
		publishPeriod:           publishMsgCfg.Period,
		startEpochSealingBuffer: abi.ChainEpoch(publishMsgCfg.StartEpochSealingBuffer),
		publishSpec:             publishSpec,
	}
}

// PendingDeals returns the list of deals that are queued up to be published
func (p *DealPublisher) PendingDeals() api.PendingDealInfo {
	p.lk.Lock()
	defer p.lk.Unlock()

	// Filter out deals whose context has been cancelled
	deals := make([]*pendingDeal, 0, len(p.pending))
	for _, dl := range p.pending {
		if dl.ctx.Err() == nil {
			deals = append(deals, dl)
		}
	}

	pending := make([]market2.ClientDealProposal, len(deals))
	for i, deal := range deals {
		pending[i] = deal.deal
	}

	return api.PendingDealInfo{
		Deals:              pending,
		PublishPeriodStart: p.publishPeriodStart,
		PublishPeriod:      p.publishPeriod,
	}
}

// ForcePublishPendingDeals publishes all pending deals without waiting for
// the publish period to elapse
func (p *DealPublisher) ForcePublishPendingDeals() {
	p.lk.Lock()
	defer p.lk.Unlock()

	log.Infof("force publishing deals")
	p.publishAllDeals()
}

func (p *DealPublisher) Publish(ctx context.Context, deal market2.ClientDealProposal) (cid.Cid, error) {
	pdeal := newPendingDeal(ctx, deal)

	// Add the deal to the queue
	p.processNewDeal(pdeal)

	// Wait for the deal to be submitted
	select {
	case <-ctx.Done():
		return cid.Undef, ctx.Err()
	case res := <-pdeal.Result:
		return res.msgCid, res.err
	}
}

func (p *DealPublisher) processNewDeal(pdeal *pendingDeal) {
	p.lk.Lock()
	defer p.lk.Unlock()

	// Filter out any cancelled deals
	p.filterCancelledDeals()

	// If all deals have been cancelled, clear the wait-for-deals timer
	if len(p.pending) == 0 && p.cancelWaitForMoreDeals != nil {
		p.cancelWaitForMoreDeals()
		p.cancelWaitForMoreDeals = nil
	}

	// Make sure the new deal hasn't been cancelled
	if pdeal.ctx.Err() != nil {
		return
	}

	// Add the new deal to the queue
	p.pending = append(p.pending, pdeal)
	log.Infof("add deal with piece CID %s to publish deals queue - %d deals in queue (max queue size %d)",
		pdeal.deal.Proposal.PieceCID, len(p.pending), p.maxDealsPerPublishMsg)

	// If the maximum number of deals per message has been reached or we're not batching, send a
	// publish message
	if uint64(len(p.pending)) >= p.maxDealsPerPublishMsg || p.publishPeriod == 0 {
		log.Infof("publish deals queue has reached max size of %d, publishing deals", p.maxDealsPerPublishMsg)
		p.publishAllDeals()
		return
	}

	// Otherwise wait for more deals to arrive or the timeout to be reached
	p.waitForMoreDeals()
}

func (p *DealPublisher) waitForMoreDeals() {
	// Check if we're already waiting for deals
	if !p.publishPeriodStart.IsZero() {
		elapsed := build.Clock.Since(p.publishPeriodStart)
		log.Infof("%s elapsed of / %s until publish deals queue is published",
			elapsed, p.publishPeriod)
		return
	}

	// Set a timeout to wait for more deals to arrive
	log.Infof("waiting publish deals queue period of %s before publishing", p.publishPeriod)
	ctx, cancel := context.WithCancel(p.ctx)

	// Create the timer _before_ taking the current time so publishPeriod+timeout is always >=
	// the actual timer timeout.
	timer := build.Clock.Timer(p.publishPeriod)

	p.publishPeriodStart = build.Clock.Now()
	p.cancelWaitForMoreDeals = cancel

	go func() {
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
			p.lk.Lock()
			defer p.lk.Unlock()

			// The timeout has expired so publish all pending deals
			log.Infof("publish deals queue period of %s has expired, publishing deals", p.publishPeriod)
			p.publishAllDeals()
		}
	}()
}

func (p *DealPublisher) publishAllDeals() {
	// If the timeout hasn't yet been cancelled, cancel it
	if p.cancelWaitForMoreDeals != nil {
		p.cancelWaitForMoreDeals()
		p.cancelWaitForMoreDeals = nil
		p.publishPeriodStart = time.Time{}
	}

	// Filter out any deals that have been cancelled
	p.filterCancelledDeals()
	deals := p.pending
	p.pending = nil

	// Send the publish message
	go p.publishReady(deals)
}

func (p *DealPublisher) publishReady(ready []*pendingDeal) {
	if len(ready) == 0 {
		return
	}

	// onComplete is called when the publish message has been sent or there
	// was an error
	onComplete := func(pd *pendingDeal, msgCid cid.Cid, err error) {
		// Send the publish result on the pending deal's Result channel
		res := publishResult{
			msgCid: msgCid,
			err:    err,
		}
		select {
		case <-p.ctx.Done():
		case <-pd.ctx.Done():
		case pd.Result <- res:
		}
	}

	// Validate each deal to make sure it can be published
	validated := make([]*pendingDeal, 0, len(ready))
	deals := make([]market2.ClientDealProposal, 0, len(ready))
	for _, pd := range ready {
		// Validate the deal
		if err := p.validateDeal(pd.deal); err != nil {
			// Validation failed, complete immediately with an error
			go onComplete(pd, cid.Undef, xerrors.Errorf("publish validation failed: %w", err))
			continue
		}

		validated = append(validated, pd)
		deals = append(deals, pd.deal)
	}

	// Send the publish message
	msgCid, err := p.publishDealProposals(deals)

	// Signal that each deal has been published
	for _, pd := range validated {
		go onComplete(pd, msgCid, err)
	}
}

// validateDeal checks that the deal proposal start epoch hasn't already
// elapsed
func (p *DealPublisher) validateDeal(deal market2.ClientDealProposal) error {
	start := time.Now()

	pcid, err := deal.Proposal.Cid()
	if err != nil {
		return xerrors.Errorf("computing proposal cid: %w", err)
	}

	head, err := p.api.ChainHead(p.ctx)
	if err != nil {
		return err
	}
	if head.Height()+p.startEpochSealingBuffer > deal.Proposal.StartEpoch {
		return xerrors.Errorf(
			"cannot publish deal with piece CID %s: current epoch %d has passed deal proposal start epoch %d",
			deal.Proposal.PieceCID, head.Height(), deal.Proposal.StartEpoch)
	}

	mi, err := p.api.StateMinerInfo(p.ctx, deal.Proposal.Provider, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting provider info: %w", err)
	}

	params, err := actors.SerializeParams(&market2.PublishStorageDealsParams{
		Deals: []market0.ClientDealProposal{deal},
	})
	if err != nil {
		return xerrors.Errorf("serializing PublishStorageDeals params failed: %w", err)
	}

	addr, _, err := p.as.AddressFor(p.ctx, p.api, mi, api.DealPublishAddr, big.Zero(), big.Zero())
	if err != nil {
		return xerrors.Errorf("selecting address for publishing deals: %w", err)
	}

	res, err := p.api.StateCall(p.ctx, &types.Message{
		To:     market.Address,
		From:   addr,
		Value:  types.NewInt(0),
		Method: market.Methods.PublishStorageDeals,
		Params: params,
	}, head.Key())
	if err != nil {
		return xerrors.Errorf("simulating deal publish message: %w", err)
	}
	if res.MsgRct.ExitCode != exitcode.Ok {
		return xerrors.Errorf("simulating deal publish message: non-zero exitcode %s; message: %s", res.MsgRct.ExitCode, res.Error)
	}

	took := time.Now().Sub(start)
	log.Infow("validating deal", "took", took, "proposal", pcid)

	return nil
}

// Sends the publish message
func (p *DealPublisher) publishDealProposals(deals []market2.ClientDealProposal) (cid.Cid, error) {
	if len(deals) == 0 {
		return cid.Undef, nil
	}

	log.Infof("publishing %d deals in publish deals queue with piece CIDs: %s", len(deals), pieceCids(deals))

	provider := deals[0].Proposal.Provider
	for _, dl := range deals {
		if dl.Proposal.Provider != provider {
			msg := fmt.Sprintf("publishing %d deals failed: ", len(deals)) +
				"not all deals are for same provider: " +
				fmt.Sprintf("deal with piece CID %s is for provider %s ", deals[0].Proposal.PieceCID, deals[0].Proposal.Provider) +
				fmt.Sprintf("but deal with piece CID %s is for provider %s", dl.Proposal.PieceCID, dl.Proposal.Provider)
			return cid.Undef, xerrors.Errorf(msg)
		}
	}

	mi, err := p.api.StateMinerInfo(p.ctx, provider, types.EmptyTSK)
	if err != nil {
		return cid.Undef, err
	}

	params, err := actors.SerializeParams(&market2.PublishStorageDealsParams{
		Deals: deals,
	})

	if err != nil {
		return cid.Undef, xerrors.Errorf("serializing PublishStorageDeals params failed: %w", err)
	}

	addr, _, err := p.as.AddressFor(p.ctx, p.api, mi, api.DealPublishAddr, big.Zero(), big.Zero())
	if err != nil {
		return cid.Undef, xerrors.Errorf("selecting address for publishing deals: %w", err)
	}

	smsg, err := p.api.MpoolPushMessage(p.ctx, &types.Message{
		To:     market.Address,
		From:   addr,
		Value:  types.NewInt(0),
		Method: market.Methods.PublishStorageDeals,
		Params: params,
	}, p.publishSpec)

	if err != nil {
		return cid.Undef, err
	}
	return smsg.Cid(), nil
}

func pieceCids(deals []market2.ClientDealProposal) string {
	cids := make([]string, 0, len(deals))
	for _, dl := range deals {
		cids = append(cids, dl.Proposal.PieceCID.String())
	}
	return strings.Join(cids, ", ")
}

// filter out deals that have been cancelled
func (p *DealPublisher) filterCancelledDeals() {
	filtered := p.pending[:0]
	for _, pd := range p.pending {
		if pd.ctx.Err() != nil {
			continue
		}
		filtered = append(filtered, pd)
	}
	p.pending = filtered
}
