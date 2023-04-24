package storageimpl

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ipfs/go-cid"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	carv2 "github.com/ipld/go-car/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-fil-markets/commp"
	"github.com/filecoin-project/go-fil-markets/filestore"
	"github.com/filecoin-project/go-fil-markets/piecestore"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket/impl/providerstates"
	"github.com/filecoin-project/go-fil-markets/storagemarket/network"
	"github.com/filecoin-project/go-fil-markets/stores"
)

// -------
// providerDealEnvironment
// -------

type providerDealEnvironment struct {
	p *Provider
}

func (p *providerDealEnvironment) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool) error {
	return stores.RegisterShardSync(ctx, p.p.dagStore, pieceCid, carPath, eagerInit)
}

// AnnounceIndex informs indexer nodes that a new deal was received,
// so they can download its index
func (p *providerDealEnvironment) AnnounceIndex(ctx context.Context, deal storagemarket.MinerDeal) (advertCid cid.Cid, err error) {
	mt := p.p.metadataForDeal(deal)

	// ensure we have a connection with the full node host so that the index provider gossip sub announcements make their
	// way to the filecoin bootstrapper network
	if err := p.p.meshCreator.Connect(ctx); err != nil {
		return cid.Undef, fmt.Errorf("cannot publish index record as indexer host failed to connect to the full node: %w", err)
	}

	return p.p.indexProvider.NotifyPut(ctx, nil, deal.ProposalCid.Bytes(), mt)
}

func (p *providerDealEnvironment) RemoveIndex(ctx context.Context, proposalCid cid.Cid) error {
	_, err := p.p.indexProvider.NotifyRemove(ctx, peer.ID(""), proposalCid.Bytes())
	return err
}

func (p *providerDealEnvironment) ReadCAR(path string) (*carv2.Reader, error) {
	return carv2.OpenReader(path)
}

func (p *providerDealEnvironment) FinalizeBlockstore(proposalCid cid.Cid) error {
	bs, err := p.p.stores.Get(proposalCid.String())
	if err != nil {
		if xerrors.Is(err, stores.ErrNotFound) {
			// The blockstore has already been cleaned up
			return nil
		}
		return xerrors.Errorf("failed to get read/write blockstore: %w", err)
	}

	if err := bs.Finalize(); err != nil {
		return xerrors.Errorf("failed to finalize read/write blockstore: %w", err)
	}

	return nil
}

func (p *providerDealEnvironment) TerminateBlockstore(proposalCid cid.Cid, path string) error {
	// stop tracking it.
	if err := p.p.stores.Untrack(proposalCid.String()); err != nil {
		log.Warnf("failed to untrack read write blockstore, proposalCid=%s, car_path=%s: %s", proposalCid, path, err)
	}

	// delete the backing CARv2 file as it was a temporary file we created for
	// this storage deal; the piece has now been handed off, or the deal has failed.
	if err := os.Remove(path); err != nil {
		log.Warnf("failed to delete carv2 file on termination, car_path=%s: %s", path, err)
	}

	return nil
}

func (p *providerDealEnvironment) Address() address.Address {
	return p.p.actor
}

func (p *providerDealEnvironment) Node() storagemarket.StorageProviderNode {
	return p.p.spn
}

func (p *providerDealEnvironment) Ask() storagemarket.StorageAsk {
	sask := p.p.storedAsk.GetAsk()
	if sask == nil {
		return storagemarket.StorageAskUndefined
	}
	return *sask.Ask
}

// GeneratePieceCommitment generates the pieceCid for the CARv1 deal payload in
// the CARv2 file that already exists at the given path.
func (p *providerDealEnvironment) GeneratePieceCommitment(proposalCid cid.Cid, carPath string, dealSize abi.PaddedPieceSize) (c cid.Cid, path filestore.Path, finalErr error) {
	rd, err := carv2.OpenReader(carPath)
	if err != nil {
		return cid.Undef, "", xerrors.Errorf("failed to get CARv2 reader, proposalCid=%s, carPath=%s: %w", proposalCid, carPath, err)
	}

	defer func() {
		if err := rd.Close(); err != nil {
			log.Errorf("failed to close CARv2 reader, carPath=%s, err=%s", carPath, err)

			if finalErr == nil {
				c = cid.Undef
				path = ""
				finalErr = xerrors.Errorf("failed to close CARv2 reader, proposalCid=%s, carPath=%s: %w",
					proposalCid, carPath, err)
				return
			}
		}
	}()

	r, err := rd.DataReader()
	if err != nil {
		return cid.Undef, "", fmt.Errorf("failed to get data reader over CAR file, proposalCid=%s, carPath=%s: %w", proposalCid, carPath, err)
	}

	pieceCID, err := commp.GenerateCommp(r, rd.Header.DataSize, uint64(dealSize))
	return pieceCID, "", err
}

func (p *providerDealEnvironment) FileStore() filestore.FileStore {
	return p.p.fs
}

func (p *providerDealEnvironment) PieceStore() piecestore.PieceStore {
	return p.p.pieceStore
}

func (p *providerDealEnvironment) SendSignedResponse(ctx context.Context, resp *network.Response) error {
	s, err := p.p.conns.DealStream(resp.Proposal)
	if err != nil {
		return xerrors.Errorf("couldn't send response: %w", err)
	}

	sig, err := p.p.sign(ctx, resp)
	if err != nil {
		return xerrors.Errorf("failed to sign response message: %w", err)
	}

	signedResponse := network.SignedResponse{
		Response:  *resp,
		Signature: sig,
	}

	err = s.WriteDealResponse(signedResponse, p.p.sign)
	if err != nil {
		// Assume client disconnected
		_ = p.p.conns.Disconnect(resp.Proposal)
	}
	return err
}

func (p *providerDealEnvironment) Disconnect(proposalCid cid.Cid) error {
	return p.p.conns.Disconnect(proposalCid)
}

func (p *providerDealEnvironment) RunCustomDecisionLogic(ctx context.Context, deal storagemarket.MinerDeal) (bool, string, error) {
	if p.p.customDealDeciderFunc == nil {
		return true, "", nil
	}
	return p.p.customDealDeciderFunc(ctx, deal)
}

func (p *providerDealEnvironment) TagPeer(id peer.ID, s string) {
	p.p.net.TagPeer(id, s)
}

func (p *providerDealEnvironment) UntagPeer(id peer.ID, s string) {
	p.p.net.UntagPeer(id, s)
}

func (p *providerDealEnvironment) AwaitRestartTimeout() <-chan time.Time {
	timer := time.NewTimer(p.p.awaitTransferRestartTimeout)
	return timer.C
}

var _ providerstates.ProviderDealEnvironment = &providerDealEnvironment{}

type providerStoreGetter struct {
	p *Provider
}

func (psg *providerStoreGetter) Get(proposalCid cid.Cid) (bstore.Blockstore, error) {
	// Wait for the provider to be ready
	err := awaitProviderReady(psg.p)
	if err != nil {
		return nil, err
	}

	var deal storagemarket.MinerDeal
	err = psg.p.deals.Get(proposalCid).Get(&deal)
	if err != nil {
		return nil, xerrors.Errorf("failed to get deal state: %w", err)
	}

	return psg.p.stores.GetOrOpen(proposalCid.String(), deal.InboundCAR, deal.Ref.Root)
}

type providerPushDeals struct {
	p *Provider
}

func (ppd *providerPushDeals) Get(proposalCid cid.Cid) (storagemarket.MinerDeal, error) {
	// Wait for the provider to be ready
	var deal storagemarket.MinerDeal
	err := awaitProviderReady(ppd.p)
	if err != nil {
		return deal, err
	}

	err = ppd.p.deals.GetSync(context.TODO(), proposalCid, &deal)
	return deal, err
}

// awaitProviderReady waits for the provider to startup
func awaitProviderReady(p *Provider) error {
	err := p.AwaitReady()
	if err != nil {
		return xerrors.Errorf("could not get deal with proposal CID %s: error waiting for provider startup: %w")
	}

	return nil
}
