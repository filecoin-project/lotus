package deals

// this file implements storagemarket.StorageClient

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storagemarket"
)

func (c *Client) ListProviders(ctx context.Context) (<-chan storagemarket.StorageProviderInfo, error) {
	providers, err := c.node.ListStorageProviders(ctx)
	if err != nil {
		return nil, err
	}

	out := make(chan storagemarket.StorageProviderInfo)

	go func() {
		for _, p := range providers {
			select {
			case out <- *p:
			case <-ctx.Done():
				return
			}

		}
	}()

	return out, nil
}

func (c *Client) ListDeals(ctx context.Context) ([]actors.OnChainDeal, error) {
	addr, err := c.node.GetDefaultWalletAddress(ctx)
	if err != nil {
		return nil, err
	}

	return c.node.ListClientDeals(ctx, addr)
}

func (c *Client) ListInProgressDeals(ctx context.Context) ([]storagemarket.ClientDeal, error) {
	deals, err := c.List()
	if err != nil {
		return nil, err
	}

	out := make([]storagemarket.ClientDeal, len(deals))
	for k, v := range deals {
		out[k] = storagemarket.ClientDeal{
			ProposalCid:    v.ProposalCid,
			Proposal:       v.Proposal,
			State:          v.State,
			Miner:          v.Miner,
			MinerWorker:    v.MinerWorker,
			DealID:         v.DealID,
			PublishMessage: v.PublishMessage,
		}
	}

	return out, nil
}

func (c *Client) GetInProgressDeal(ctx context.Context, cid cid.Cid) (storagemarket.ClientDeal, error) {
	deals, err := c.ListInProgressDeals(ctx)
	if err != nil {
		return storagemarket.ClientDeal{}, err
	}

	for _, deal := range deals {
		if deal.ProposalCid == cid {
			return deal, nil
		}
	}

	return storagemarket.ClientDeal{}, xerrors.Errorf("couldn't find client deal")
}

func (c *Client) GetAsk(ctx context.Context, info storagemarket.StorageProviderInfo) (*storagemarket.StorageAsk, error) {
	return c.QueryAsk(ctx, info.PeerID, info.Address)
}

func (c *Client) ProposeStorageDeal(ctx context.Context, info *storagemarket.StorageProviderInfo, payloadCid cid.Cid, proposalExpiration storagemarket.Epoch, duration storagemarket.Epoch, price storagemarket.TokenAmount, collateral storagemarket.TokenAmount) (*storagemarket.ProposeStorageDealResult, error) {
	addr, err := c.node.GetDefaultWalletAddress(ctx)
	if err != nil {
		return nil, err
	}

	proposal := ClientDealProposal{
		Data:               payloadCid,
		PricePerEpoch:      types.BigInt(price),
		ProposalExpiration: uint64(proposalExpiration),
		Duration:           uint64(duration),
		Client:             addr,
		ProviderAddress:    info.Address,
		MinerWorker:        info.Worker,
		MinerID:            info.PeerID,
	}

	proposalCid, err := c.Start(ctx, proposal)

	result := &storagemarket.ProposeStorageDealResult{
		ProposalCid: proposalCid,
	}

	return result, err
}

func (c *Client) GetPaymentEscrow(ctx context.Context) (storagemarket.Balance, error) {
	addr, err := c.node.GetDefaultWalletAddress(ctx)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	balance, err := c.node.GetBalance(ctx, addr)

	return balance, err
}

func (c *Client) AddPaymentEscrow(ctx context.Context, amount storagemarket.TokenAmount) error {
	addr, err := c.node.GetDefaultWalletAddress(ctx)
	if err != nil {
		return err
	}

	return c.node.AddFunds(ctx, addr, amount)
}

var _ storagemarket.StorageClient = &Client{}
