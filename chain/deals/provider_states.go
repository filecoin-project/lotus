package deals

import (
	"bytes"
	"context"

	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"

	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/padreader"
	"github.com/filecoin-project/lotus/storage/sectorblocks"
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

	head, err := p.full.ChainHead(ctx)
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
	clientMarketBalance, err := p.full.StateMarketBalance(ctx, deal.Proposal.Client, nil)
	if err != nil {
		return nil, xerrors.Errorf("getting client market balance failed: %w", err)
	}

	// This doesn't guarantee that the client won't withdraw / lock those funds
	// but it's a decent first filter
	if clientMarketBalance.Available.LessThan(deal.Proposal.TotalStoragePrice()) {
		return nil, xerrors.New("clientMarketBalance.Available too small")
	}

	waddr, err := p.full.StateMinerWorker(ctx, deal.Proposal.Provider, nil)
	if err != nil {
		return nil, err
	}

	// TODO: check StorageCollateral (may be too large (or too small))
	if err := p.full.MarketEnsureAvailable(ctx, waddr, deal.Proposal.StorageCollateral); err != nil {
		return nil, err
	}

	log.Info("publishing deal")

	params, err := actors.SerializeParams(&actors.PublishStorageDealsParams{
		Deals: []actors.StorageDealProposal{deal.Proposal},
	})
	if err != nil {
		return nil, xerrors.Errorf("serializing PublishStorageDeals params failed: ", err)
	}

	// TODO: We may want this to happen after fetching data
	smsg, err := p.full.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     waddr,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   actors.SMAMethods.PublishStorageDeals,
		Params:   params,
	})
	if err != nil {
		return nil, err
	}
	r, err := p.full.StateWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return nil, err
	}
	if r.Receipt.ExitCode != 0 {
		return nil, xerrors.Errorf("publishing deal failed: exit %d", r.Receipt.ExitCode)
	}
	var resp actors.PublishStorageDealResponse
	if err := resp.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)); err != nil {
		return nil, err
	}
	if len(resp.DealIDs) != 1 {
		return nil, xerrors.Errorf("got unexpected number of DealIDs from SMA")
	}

	log.Infof("fetching data for a deal %d", resp.DealIDs[0])
	err = p.sendSignedResponse(&Response{
		State: api.DealAccepted,

		Proposal:              deal.ProposalCid,
		StorageDealSubmission: smsg,
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
		&StorageDataTransferVoucher{Proposal: deal.ProposalCid, DealID: resp.DealIDs[0]},
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
	root, err := p.dag.Get(ctx, deal.Ref)
	if err != nil {
		return nil, xerrors.Errorf("failed to get file root for deal: %s", err)
	}

	// TODO: abstract this away into ReadSizeCloser + implement different modes
	n, err := unixfile.NewUnixfsFile(ctx, p.dag, root)
	if err != nil {
		return nil, xerrors.Errorf("cannot open unixfs file: %s", err)
	}

	uf, ok := n.(sectorblocks.UnixfsReader)
	if !ok {
		// we probably got directory, unsupported for now
		return nil, xerrors.Errorf("unsupported unixfs file type")
	}

	// TODO: uf.Size() is user input, not trusted
	// This won't be useful / here after we migrate to putting CARs into sectors
	size, err := uf.Size()
	if err != nil {
		return nil, xerrors.Errorf("getting unixfs file size: %w", err)
	}
	if padreader.PaddedSize(uint64(size)) != deal.Proposal.PieceSize {
		return nil, xerrors.Errorf("deal.Proposal.PieceSize didn't match padded unixfs file size")
	}

	sectorID, err := p.secb.AddUnixfsPiece(ctx, uf, deal.DealID)
	if err != nil {
		return nil, xerrors.Errorf("AddPiece failed: %s", err)
	}
	log.Warnf("New Sector: %d (deal %d)", sectorID, deal.DealID)

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
