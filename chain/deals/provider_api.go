package deals

import (
	"context"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storagemarket"

	"golang.org/x/xerrors"
)

func (p *Provider) AddAsk(price types.BigInt, ttlsecs int64) error {
	return p.SetPrice(price, ttlsecs)
}

func (p *Provider) ListAsks(addr address.Address) []*types.SignedStorageAsk {
	ask := p.GetAsk(addr)

	if ask != nil {
		return []*types.SignedStorageAsk{ask}
	}

	return nil
}

func (p *Provider) ListDeals(ctx context.Context) ([]actors.OnChainDeal, error) {
	ts, err := p.full.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	var out []actors.OnChainDeal

	allDeals, err := p.full.StateMarketDeals(ctx, ts)
	for _, deal := range allDeals {
		if deal.Deal.Proposal.Provider == p.actor {
			out = append(out, deal)
		}
	}

	return out, nil
}

func (p *Provider) AddStorageCollateral(ctx context.Context, amount types.BigInt) error {
	return p.AddFunds(ctx, p.actor, amount)
}

func (p *Provider) GetStorageCollateral(ctx context.Context) (storagemarket.Balance, error) {
	ts, err := p.full.ChainHead(ctx)
	if err != nil {
		return storagemarket.Balance{}, err
	}

	balance, err := p.full.StateMarketBalance(ctx, p.actor, ts)

	return balance, err
}

func (p *Provider) ListIncompleteDeals() ([]storagemarket.MinerDeal, error) {
	var out []storagemarket.MinerDeal

	var deals []MinerDeal
	if err := p.deals.List(&deals); err != nil {
		return nil, err
	}

	for _, deal := range deals {
		out = append(out, storagemarket.MinerDeal{
			Client:      deal.Client,
			Proposal:    deal.Proposal,
			ProposalCid: deal.ProposalCid,
			State:       deal.State,
			Ref:         deal.Ref,
			DealID:      deal.DealID,
			SectorID:    deal.SectorID,
		})
	}

	return out, nil
}

// (Provider Node API)
func (p *Provider) AddFunds(ctx context.Context, from address.Address, amount types.BigInt) error {
	smsg, err := p.full.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     from,
		Value:    amount,
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   actors.SMAMethods.AddBalance,
	})
	if err != nil {
		return err
	}

	r, err := p.full.StateWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return err
	}

	if r.Receipt.ExitCode != 0 {
		return xerrors.Errorf("adding funds to storage miner market actor failed: exit %d", r.Receipt.ExitCode)
	}

	return nil
}

var _ storagemarket.StorageProvider = &Provider{}
