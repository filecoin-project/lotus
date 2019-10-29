package deals

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-sectorbuilder/sealing_state"
	"github.com/ipfs/go-cid"
	ipldfree "github.com/ipld/go-ipld-prime/impl/free"
	"github.com/ipld/go-ipld-prime/traversal/selector"
	"github.com/ipld/go-ipld-prime/traversal/selector/builder"

	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
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

func (p *Provider) addMarketFunds(ctx context.Context, worker address.Address, deal MinerDeal) error {
	log.Info("Adding market funds for storage collateral")
	smsg, err := p.full.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     worker,
		Value:    deal.Proposal.StorageCollateral,
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

func (p *Provider) accept(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	switch deal.Proposal.PieceSerialization {
	//case SerializationRaw:
	//case SerializationIPLD:
	case actors.SerializationUnixFSv0:
	default:
		return nil, xerrors.Errorf("deal proposal with unsupported serialization: %s", deal.Proposal.PieceSerialization)
	}

	head, err := p.full.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	if head.Height() >= deal.Proposal.ProposalExpiration {
		return nil, xerrors.Errorf("deal proposal already expired")
	}

	// TODO: check StorageCollateral

	// TODO:
	minPrice := types.BigMul(p.ask.Ask.Price, types.BigMul(types.NewInt(deal.Proposal.Duration), types.NewInt(deal.Proposal.PieceSize)))
	if deal.Proposal.StoragePrice.LessThan(minPrice) {
		return nil, xerrors.Errorf("storage price less than asking price: %s < %s", deal.Proposal.StoragePrice, minPrice)
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
	if clientMarketBalance.Available.LessThan(deal.Proposal.StoragePrice) {
		return nil, xerrors.New("clientMarketBalance.Available too small")
	}

	waddr, err := p.full.StateMinerWorker(ctx, deal.Proposal.Provider, nil)
	if err != nil {
		return nil, err
	}

	providerMarketBalance, err := p.full.StateMarketBalance(ctx, waddr, nil)
	if err != nil {
		return nil, xerrors.Errorf("getting provider market balance failed: %w", err)
	}

	// TODO: this needs to be atomic
	if providerMarketBalance.Available.LessThan(deal.Proposal.StorageCollateral) {
		if err := p.addMarketFunds(ctx, waddr, deal); err != nil {
			return nil, err
		}
	}

	log.Info("publishing deal")

	storageDeal := actors.StorageDeal{
		Proposal: deal.Proposal,
	}
	if err := api.SignWith(ctx, p.full.WalletSign, waddr, &storageDeal); err != nil {
		return nil, xerrors.Errorf("signing storage deal failed: ", err)
	}

	params, err := actors.SerializeParams(&actors.PublishStorageDealsParams{
		Deals: []actors.StorageDeal{storageDeal},
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
		return nil, xerrors.Errorf("got unexpected number of DealIDs from")
	}

	log.Info("fetching data for a deal")
	mcid := smsg.Cid()
	err = p.sendSignedResponse(&Response{
		State:          api.DealAccepted,
		Message:        "",
		Proposal:       deal.ProposalCid,
		PublishMessage: &mcid,
	})
	if err != nil {
		return nil, err
	}

	ssb := builder.NewSelectorSpecBuilder(ipldfree.NodeBuilder())

	// this is the selector for "get the whole DAG"
	// TODO: support storage deals with custom payload selectors
	allSelector := ssb.ExploreRecursive(selector.RecursionLimitNone(),
		ssb.ExploreAll(ssb.ExploreRecursiveEdge())).Node()

	// initiate a pull data transfer. This will complete asynchronously and the
	// completion of the data transfer will trigger a change in deal state
	// (see onDataTransferEvent)
	_, err = p.dataTransfer.OpenPullDataChannel(deal.Client,
		StorageDataTransferVoucher{Proposal: deal.ProposalCid},
		deal.Ref,
		allSelector,
	)

	return func(deal *MinerDeal) {
		deal.DealID = resp.DealIDs[0]
	}, nil
}

// STAGED

func (p *Provider) staged(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	err := p.sendSignedResponse(&Response{
		State:    api.DealStaged,
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

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
	if uint64(size) != deal.Proposal.PieceSize {
		return nil, xerrors.Errorf("deal.Proposal.PieceSize didn't match unixfs file size")
	}

	pcid, err := cid.Cast(deal.Proposal.PieceRef)
	if err != nil {
		return nil, err
	}

	sectorID, err := p.secst.AddUnixfsPiece(pcid, uf, deal.DealID)
	if err != nil {
		return nil, xerrors.Errorf("AddPiece failed: %s", err)
	}

	log.Warnf("New Sector: %d", sectorID)
	return func(deal *MinerDeal) {
		deal.SectorID = sectorID
	}, nil
}

// SEALING

func (p *Provider) waitSealed(ctx context.Context, deal MinerDeal) (sectorbuilder.SectorSealingStatus, error) {
	status, err := p.secst.WaitSeal(ctx, deal.SectorID)
	if err != nil {
		return sectorbuilder.SectorSealingStatus{}, err
	}

	switch status.State {
	case sealing_state.Sealed:
	case sealing_state.Failed:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("sealing sector %d for deal %s (ref=%s) failed: %s", deal.SectorID, deal.ProposalCid, deal.Ref, status.SealErrorMsg)
	case sealing_state.Pending:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("sector status was 'pending' after call to WaitSeal (for sector %d)", deal.SectorID)
	case sealing_state.Sealing:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("sector status was 'wait' after call to WaitSeal (for sector %d)", deal.SectorID)
	default:
		return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("unknown SealStatusCode: %d", status.SectorID)
	}

	return status, nil
}

func (p *Provider) sealing(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	err := p.sendSignedResponse(&Response{
		State:    api.DealSealing,
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

	if err := p.secst.SealSector(ctx, deal.SectorID); err != nil {
		return nil, xerrors.Errorf("sealing sector failed: %w", err)
	}

	_, err = p.waitSealed(ctx, deal)
	if err != nil {
		return nil, err
	}
	// TODO: Spec doesn't say anything about inclusion proofs anywhere
	//  Not sure what mechanisms prevents miner from storing data that isn't
	//  clients' data

	return nil, nil
}

func (p *Provider) complete(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	// TODO: Add dealID to commtracker (probably before sealing)
	mcid, err := p.commt.WaitCommit(ctx, deal.Proposal.Provider, deal.SectorID)
	if err != nil {
		log.Warnf("Waiting for sector commitment message: %s", err)
	}

	err = p.sendSignedResponse(&Response{
		State:    api.DealComplete,
		Proposal: deal.ProposalCid,

		CommitMessage: &mcid,
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

	return nil, nil
}
