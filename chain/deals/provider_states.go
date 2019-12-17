package deals

import (
	"context"

	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storagemarket"
)

type providerHandlerFunc func(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error)

func (p *Provider) handle(ctx context.Context, deal MinerDeal, cb providerHandlerFunc, next api.DealState) {
	go func() {
		mut, err := cb(ctx, deal)

		if err == nil && next == api.DealNoUpdate {
			return
		}

		select {
		case p.updated <- minerDealUpdate{
			newState: next,
			id:       deal.ProposalCid,
			err:      err,
			mut:      mut,
		}:
		case <-p.stop:
		}
	}()
}

// ACCEPTED
func (p *Provider) accept(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {

	head, err := p.spn.MostRecentStateId(ctx)
	if err != nil {
		return nil, err
	}
	if head.Height() >= deal.Proposal.ProposalExpiration {
		return nil, xerrors.Errorf("deal proposal already expired")
	}

	// TODO: check StorageCollateral

	minPrice := types.BigDiv(types.BigMul(p.ask.Ask.Price, types.NewInt(deal.Proposal.PieceSize)), types.NewInt(1<<30))
	if deal.Proposal.StoragePricePerEpoch.LessThan(minPrice) {
		return nil, xerrors.Errorf("storage price per epoch less than asking price: %s < %s", deal.Proposal.StoragePricePerEpoch, minPrice)
	}

	if deal.Proposal.PieceSize < p.ask.Ask.MinPieceSize {
		return nil, xerrors.Errorf("piece size less than minimum required size: %d < %d", deal.Proposal.PieceSize, p.ask.Ask.MinPieceSize)
	}

	// check market funds
	clientMarketBalance, err := p.spn.GetBalance(ctx, deal.Proposal.Client)
	if err != nil {
		return nil, xerrors.Errorf("getting client market balance failed: %w", err)
	}

	// This doesn't guarantee that the client won't withdraw / lock those funds
	// but it's a decent first filter
	if clientMarketBalance.Available.LessThan(deal.Proposal.TotalStoragePrice()) {
		return nil, xerrors.New("clientMarketBalance.Available too small")
	}

	waddr, err := p.spn.GetMinerWorker(ctx, deal.Proposal.Provider)
	if err != nil {
		return nil, err
	}

	// TODO: check StorageCollateral (may be too large (or too small))
	if err := p.spn.EnsureFunds(ctx, waddr, storagemarket.TokenAmount(deal.Proposal.StorageCollateral)); err != nil {
		return nil, err
	}

	smDeal := storagemarket.MinerDeal{
		Client:      deal.Client,
		Proposal:    deal.Proposal,
		ProposalCid: deal.ProposalCid,
		State:       deal.State,
		Ref:         deal.Ref,
		SectorID:    deal.SectorID,
	}

	dealId, mcid, err := p.spn.PublishDeals(ctx, smDeal)
	if err != nil {
		return nil, err
	}

	log.Infof("fetching data for a deal %d", dealId)
	err = p.sendSignedResponse(ctx, &Response{
		State: api.DealAccepted,

		Proposal:       deal.ProposalCid,
		PublishMessage: &mcid,
	})
	if err != nil {
		return nil, err
	}

	if err := p.disconnect(deal); err != nil {
		log.Warnf("closing client connection: %+v", err)
	}

	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	// this is the selector for "get the whole DAG"
	// TODO: support storage deals with custom payload selectors
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	// initiate a pull data transfer. This will complete asynchronously and the
	// completion of the data transfer will trigger a change in deal state
	// (see onDataTransferEvent)
	_, err = p.dataTransfer.OpenPullDataChannel(ctx,
		deal.Client,
		&StorageDataTransferVoucher{Proposal: deal.ProposalCid, DealID: uint64(dealId)},
		deal.Ref,
		allSelector,
	)
	if err != nil {
		return nil, xerrors.Errorf("failed to open pull data channel: %w", err)
	}

	return nil, nil
}

// STAGED

func (p *Provider) staged(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	sectorID, err := p.spn.OnDealComplete(
		ctx,
		storagemarket.MinerDeal{
			Client:      deal.Client,
			Proposal:    deal.Proposal,
			ProposalCid: deal.ProposalCid,
			State:       deal.State,
			Ref:         deal.Ref,
			DealID:      deal.DealID,
		},
		"",
	)

	if err != nil {
		return nil, err
	}

	return func(deal *MinerDeal) {
		deal.SectorID = sectorID
	}, nil
}

// SEALING

func (p *Provider) sealing(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	// TODO: consider waiting for seal to happen

	return nil, nil
}

func (p *Provider) complete(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	// TODO: observe sector lifecycle, status, expiration..

	return nil, nil
}
